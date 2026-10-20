package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// seedShipmentFixture menyiapkan org + courier + service minimal lalu
// membersihkannya. Mengembalikan (orgID, courierCode, serviceCode).
func seedShipmentFixture(t *testing.T, pool *pgxpool.Pool) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	var orgID, userID, courierID, serviceID string
	for _, target := range []*string{&orgID, &userID, &courierID, &serviceID} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(target); err != nil {
			t.Fatalf("gen id: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, status)
		VALUES ($1,$2,'Uji Ship','x','active')`, userID, "ship-"+userID[:8]+"@angkut.test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		orgID, "Org Uji Ship", "ship-"+orgID[:8], userID); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	// Kode kurir unik agar tidak bentrok dengan seed 0013.
	code := "tst" + courierID[:4]
	if _, err := pool.Exec(ctx, `
		INSERT INTO couriers (id, code, name, adapter_type, is_active)
		VALUES ($1,$2,'Kurir Uji','mock',true)`, courierID, code); err != nil {
		t.Fatalf("seed courier: %v", err)
	}
	sCode := "REG"
	if _, err := pool.Exec(ctx, `
		INSERT INTO courier_services (id, courier_id, code, name, eta_min_days, eta_max_days, reliability_score)
		VALUES ($1,$2,$3,'Reguler',2,3,90)`, serviceID, courierID, sCode); err != nil {
		t.Fatalf("seed service: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM shipment_packages WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM shipment_items WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM shipments WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM courier_services WHERE id=$1`, serviceID)
		_, _ = pool.Exec(ctx, `DELETE FROM couriers WHERE id=$1`, courierID)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	return orgID, code, sCode
}

func newShipment(orgID, courierCode, serviceCode, awb, idemKey string) *shipment.Shipment {
	now := time.Now().UTC()
	sh := &shipment.Shipment{
		ID:               uuid.NewString(),
		OrganizationID:   orgID,
		CourierCode:      courierCode,
		ServiceCode:      serviceCode,
		AWB:              awb,
		Status:           shipment.StatusPendingPickup,
		ActualWeight:     1.5,
		ChargeableWeight: 1.5,
		ShippingPrice:    21_000,
		CostPrice:        18_500,
		IdempotencyKey:   idemKey,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
		Items: []shipment.Item{
			{NameSnapshot: "Sepatu", Qty: 1},
		},
	}
	sh.Packages = []shipment.Package{{
		Sequence: 1, Weight: 1.5,
		QRPayload:    shipment.QRPayload("placeholder", awb),
		BarcodeValue: shipment.BarcodeValue(awb),
	}}
	return sh
}

func TestShipmentRepoCreateRoundTrip(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	sh := newShipment(orgID, code, svc, "TST260101000001", "")
	if err := repo.Create(ctx, sh); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, sh.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AWB != sh.AWB {
		t.Errorf("awb = %q, ingin %q", got.AWB, sh.AWB)
	}
	if got.Status != shipment.StatusPendingPickup {
		t.Errorf("status = %q", got.Status)
	}
	if len(got.Items) != 1 || len(got.Packages) != 1 {
		t.Fatalf("item/paket = %d/%d, ingin 1/1", len(got.Items), len(got.Packages))
	}
	if got.Packages[0].QRPayload == "" || got.Packages[0].BarcodeValue == "" {
		t.Error("payload label tidak tersimpan")
	}
}

func TestShipmentRepoAWBUniquePerOrg(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	if err := repo.Create(ctx, newShipment(orgID, code, svc, "TST260101000002", "")); err != nil {
		t.Fatalf("Create pertama: %v", err)
	}
	err := repo.Create(ctx, newShipment(orgID, code, svc, "TST260101000002", ""))
	if err == nil {
		t.Fatal("AWB duplikat dalam organisasi sama harus ditolak")
	}
	if err != shipment.ErrAWBTaken {
		t.Errorf("error = %v, ingin ErrAWBTaken", err)
	}
}

// TestShipmentRepoAWBUniqueGlobal memastikan resi yang sama DITOLAK walaupun
// datang dari organisasi berbeda — prasyarat pelacakan publik per-resi
// (TASK-035): `GET /api/v1/public/track/{awb}` hanya menerima resi tanpa
// konteks organisasi, jadi satu resi harus menunjuk tepat satu pengiriman.
func TestShipmentRepoAWBUniqueGlobal(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	otherOrgID := seedSecondOrg(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	const awb = "TST260101000077"
	if err := repo.Create(ctx, newShipment(orgID, code, svc, awb, "")); err != nil {
		t.Fatalf("Create org pertama: %v", err)
	}
	err := repo.Create(ctx, newShipment(otherOrgID, code, svc, awb, ""))
	if err == nil {
		t.Fatal("AWB duplikat lintas organisasi harus ditolak")
	}
	if err != shipment.ErrAWBTaken {
		t.Errorf("error = %v, ingin ErrAWBTaken", err)
	}
}

// seedSecondOrg membuat organisasi kedua (dengan pemiliknya) lalu
// membersihkannya. Mengembalikan id organisasi.
func seedSecondOrg(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()

	var orgID, userID string
	for _, target := range []*string{&orgID, &userID} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(target); err != nil {
			t.Fatalf("gen id: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, status)
		VALUES ($1,$2,'Uji Ship 2','x','active')`, userID, "ship2-"+userID[:8]+"@angkut.test"); err != nil {
		t.Fatalf("seed user kedua: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		orgID, "Org Uji Ship 2", "ship2-"+orgID[:8], userID); err != nil {
		t.Fatalf("seed org kedua: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM shipments WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	return orgID
}

func TestShipmentRepoIdempotencyKeyBlocksDuplicate(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	key := "idem-key-001"
	first := newShipment(orgID, code, svc, "TST260101000003", key)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create pertama: %v", err)
	}

	// Lookup dengan key yang sama harus menemukan yang pertama — inilah dasar
	// jaminan "retry tidak menduplikasi" di layer service.
	existing, ok, err := repo.GetByIdempotencyKey(ctx, orgID, key)
	if err != nil {
		t.Fatalf("GetByIdempotencyKey: %v", err)
	}
	if !ok {
		t.Fatal("shipment dengan key idempotency harus ditemukan")
	}
	if existing.ID != first.ID {
		t.Errorf("id = %q, ingin %q", existing.ID, first.ID)
	}

	// Insert kedua dengan key sama ditolak unique index — jaring terakhir bila
	// dua request paralel lolos pemeriksaan awal.
	second := newShipment(orgID, code, svc, "TST260101000004", key)
	if err := repo.Create(ctx, second); err == nil {
		t.Fatal("key idempotency duplikat harus ditolak unique index")
	}

	// Key yang tidak ada → ok=false, bukan error.
	_, ok, err = repo.GetByIdempotencyKey(ctx, orgID, "tidak-ada")
	if err != nil {
		t.Fatalf("lookup key tak ada: %v", err)
	}
	if ok {
		t.Error("key tak ada seharusnya ok=false")
	}
}

func TestShipmentRepoTenantIsolation(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	sh := newShipment(orgID, code, svc, "TST260101000005", "")
	if err := repo.Create(ctx, sh); err != nil {
		t.Fatalf("Create: %v", err)
	}

	otherOrg := "01920000-0000-7000-8000-00000000ffff"
	if _, err := repo.GetByID(ctx, otherOrg, sh.ID); err == nil {
		t.Fatal("shipment terlihat oleh organisasi lain — isolasi tenant bocor")
	}
}

func TestShipmentRepoUpdateStatusOptimistic(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	sh := newShipment(orgID, code, svc, "TST260101000006", "")
	if err := repo.Create(ctx, sh); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.UpdateStatus(ctx, orgID, sh.ID, shipment.StatusPendingPickup, shipment.StatusPickedUp, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	// Status sudah berpindah; update dari status lama harus gagal.
	if err := repo.UpdateStatus(ctx, orgID, sh.ID, shipment.StatusPendingPickup, shipment.StatusInTransit, ""); err == nil {
		t.Fatal("update dengan status asal salah harus gagal")
	}
	// delivered_at terisi saat status → delivered.
	if err := repo.UpdateStatus(ctx, orgID, sh.ID, shipment.StatusPickedUp, shipment.StatusInTransit, ""); err != nil {
		t.Fatalf("UpdateStatus in_transit: %v", err)
	}
	got, err := repo.GetByID(ctx, orgID, sh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != shipment.StatusInTransit {
		t.Errorf("status = %q, ingin in_transit", got.Status)
	}
}

func TestShipmentRepoNextAWBSequence(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	day := "260101"
	seq, err := repo.NextAWBSequence(ctx, code, day)
	if err != nil {
		t.Fatalf("NextAWBSequence: %v", err)
	}
	if seq != 1 {
		t.Errorf("residu pertama = %d, ingin 1", seq)
	}

	// AWB harus dibentuk dengan helper domain agar prefix-nya cocok dengan
	// kode kurir + hari yang di-query (bukan literal sembarang).
	awb := shipment.AWB{CourierCode: code, Year: 2026, Month: 1, Day: 1, Seq: 1}.String()
	sh := newShipment(orgID, code, svc, awb, "")
	if err := repo.Create(ctx, sh); err != nil {
		t.Fatalf("Create: %v", err)
	}
	seq2, err := repo.NextAWBSequence(ctx, code, day)
	if err != nil {
		t.Fatalf("NextAWBSequence kedua: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("residu berikutnya = %d, ingin 2", seq2)
	}
}

func TestShipmentRepoListFiltersByStatus(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, code, svc := seedShipmentFixture(t, pool)
	repo := stores.Shipments()
	ctx := context.Background()

	a := newShipment(orgID, code, svc, "TST260101000008", "")
	b := newShipment(orgID, code, svc, "TST260101000009", "")
	b.Status = shipment.StatusInTransit
	for _, sh := range []*shipment.Shipment{a, b} {
		if err := repo.Create(ctx, sh); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	items, total, err := repo.List(ctx, orgID, shipment.ListFilter{Status: shipment.StatusInTransit, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("filter in_transit = %d/%d, ingin 1/1", total, len(items))
	}
	if items[0].AWB != b.AWB {
		t.Errorf("awb = %q, ingin %q", items[0].AWB, b.AWB)
	}
}

var _ = database.NewStores // menjaga impor database dipakai di helper bersama
