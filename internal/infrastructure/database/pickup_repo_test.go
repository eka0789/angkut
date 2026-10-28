package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/domain/pickup"
	"github.com/angkut/angkut/apps/internal/platform"
)

// seedPickupFixture menyiapkan dua organisasi (masing-masing dengan gudang &
// satu pengiriman) untuk menguji isolasi tenant pada pickup_shipments.
func seedPickupFixture(t *testing.T, pool *pgxpool.Pool) (orgA, userA, whA, shipA, orgB, shipB string) {
	t.Helper()
	ctx := context.Background()

	var provinceID, cityID, districtID string
	if err := pool.QueryRow(ctx, `
		SELECT p.id, c.id, d.id FROM postal_codes pc
		JOIN districts d ON d.id = pc.district_id
		JOIN cities c ON c.id = d.city_id
		JOIN provinces p ON p.id = c.province_id
		LIMIT 1`).Scan(&provinceID, &cityID, &districtID); err != nil {
		t.Fatalf("ambil wilayah seed: %v", err)
	}

	type tenant struct {
		org, user, addr, wh, courier, service, shipment string
	}
	mk := func(label string) tenant {
		var tn tenant
		for _, target := range []*string{&tn.org, &tn.user, &tn.addr, &tn.wh, &tn.courier, &tn.service, &tn.shipment} {
			if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(target); err != nil {
				t.Fatalf("gen id: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, name, password_hash, status)
			VALUES ($1,$2,$3,'x','active')`, tn.user, "pickup-"+tn.user[:8]+"@angkut.test", "Uji Pickup "+label); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		slug := "pickup-" + tn.org[:8]
		if _, err := pool.Exec(ctx, `
			INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
			tn.org, "Org Pickup "+label, slug, tn.user); err != nil {
			t.Fatalf("seed org: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO addresses (id, organization_id, recipient, phone, province_id, city_id,
				district_id, street, status, created_by, created_at, updated_at)
			VALUES ($1,$2,'Gudang','081200000000',$3,$4,$5,'Jl. Gudang','active',$6,now(),now())`,
			tn.addr, tn.org, provinceID, cityID, districtID, tn.user); err != nil {
			t.Fatalf("seed address: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO warehouses (id, organization_id, name, address_id, is_active)
			VALUES ($1,$2,'Gudang Pickup',$3,true)`, tn.wh, tn.org, tn.addr); err != nil {
			t.Fatalf("seed warehouse: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO shipments (id, organization_id, warehouse_id, courier_code, service_code,
				awb, status, actual_weight, volumetric_weight, chargeable_weight, version)
			VALUES ($1,$2,$3,'jne','REG',$4,'pending_pickup',1,0,1,1)`,
			tn.shipment, tn.org, tn.wh, "PKP"+tn.shipment[:10]); err != nil {
			t.Fatalf("seed shipment: %v", err)
		}
		return tn
	}

	a := mk("A")
	b := mk("B")

	t.Cleanup(func() {
		for _, org := range []string{a.org, b.org} {
			_, _ = pool.Exec(ctx, `DELETE FROM pickup_shipments WHERE pickup_id IN (SELECT id FROM pickup_requests WHERE organization_id=$1)`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM pickup_requests WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM shipments WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM warehouses WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM addresses WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, org)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, a.user)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, b.user)
	})
	return a.org, a.user, a.wh, a.shipment, b.org, b.shipment
}

// TestPickupRepoIsolasiTenant: pengiriman milik organisasi lain TIDAK boleh
// bisa dilampirkan ke pickup (celah kebocoran tenant bila query hanya memakai
// FK tanpa filter organization_id).
func TestPickupRepoIsolasiTenant(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgA, userA, whA, shipA, orgB, shipB := seedPickupFixture(t, pool)
	ctx := context.Background()

	pr := &pickup.Request{
		ID: platform.NewUUID(), OrganizationID: orgA, WarehouseID: whA,
		CourierCode: "jne", Status: pickup.StatusRequested,
		CreatedBy: userA, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := stores.Pickups().Create(ctx, pr); err != nil {
		t.Fatalf("Create pickup: %v", err)
	}

	// Pengiriman milik org A: boleh.
	if err := stores.Pickups().AddShipments(ctx, orgA, pr.ID, []string{shipA}); err != nil {
		t.Fatalf("AddShipments milik sendiri: %v", err)
	}
	if ids, err := stores.Pickups().ListShipments(ctx, pr.ID); err != nil || len(ids) != 1 {
		t.Fatalf("ListShipments = %v, %v", ids, err)
	}

	// Pengiriman milik org B: HARUS ditolak.
	if err := stores.Pickups().AddShipments(ctx, orgA, pr.ID, []string{shipB}); err == nil {
		t.Fatal("pengiriman tenant lain tidak boleh terlampir ke pickup org A")
	}
	if ids, _ := stores.Pickups().ListShipments(ctx, pr.ID); len(ids) != 1 {
		t.Errorf("daftar shipment berubah menjadi %v", ids)
	}

	// Sanity: org B memang pemilik sah shipment itu.
	if err := stores.Pickups().AddShipments(ctx, orgB, pr.ID, []string{shipB}); err == nil {
		t.Fatal("pickup org A tidak boleh diubah lewat org B (FK pickup tidak dicek tenant)")
	}
}

// TestPickupRepoScheduleMenyimpanJadwal: scheduled_at harus benar-benar
// tersimpan, bukan hanya status (bug yang ditemukan saat audit).
func TestPickupRepoScheduleMenyimpanJadwal(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgA, userA, whA, _, _, _ := seedPickupFixture(t, pool)
	ctx := context.Background()

	pr := &pickup.Request{
		ID: platform.NewUUID(), OrganizationID: orgA, WarehouseID: whA,
		CourierCode: "jne", Status: pickup.StatusRequested,
		CreatedBy: userA, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := stores.Pickups().Create(ctx, pr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	when := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	if err := stores.Pickups().Schedule(ctx, orgA, pr.ID, when); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	got, err := stores.Pickups().GetByID(ctx, orgA, pr.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != pickup.StatusScheduled {
		t.Errorf("status = %q, ingin scheduled", got.Status)
	}
	if got.ScheduledAt == nil {
		t.Fatal("scheduled_at hilang setelah reload — jadwal tidak tersimpan")
	}
	if !got.ScheduledAt.Equal(when) {
		t.Errorf("scheduled_at = %v, ingin %v", got.ScheduledAt, when)
	}

	// Membatalkan dari scheduled harus berhasil (non-terminal).
	if err := stores.Pickups().CancelFrom(ctx, orgA, pr.ID); err != nil {
		t.Fatalf("CancelFrom scheduled: %v", err)
	}
	// Membatalkan lagi harus Conflict, bukan sukses palsu.
	if err := stores.Pickups().CancelFrom(ctx, orgA, pr.ID); err == nil {
		t.Error("cancel kedua seharusnya ditolak")
	}
}