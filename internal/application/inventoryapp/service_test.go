package inventoryapp_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/application/inventoryapp"
	"github.com/angkut/angkut/apps/internal/domain/inventory"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// Uji service inventory terhadap PostgreSQL nyata. Dilewati bila DATABASE_URL
// kosong / tidak terjangkau (dev tanpa DB) — CI selalu menyetelnya.

// assertValidation memastikan err adalah VALIDATION_ERROR (shared.Validation
// tidak membungkus sentinel ErrValidation, jadi cek lewat errors.As + Code).
func assertValidation(t *testing.T, err error, context string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: ingin VALIDATION_ERROR, dapat nil", context)
	}
	var se *shared.Error
	if !errors.As(err, &se) || se.Code != "VALIDATION_ERROR" {
		t.Fatalf("%s: ingin VALIDATION_ERROR, dapat %v", context, err)
	}
}

func invTestDB(t *testing.T) (*pgxpool.Pool, *database.Stores, *database.TxManager) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL kosong — lewati uji inventory")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("membuka pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("Postgres tidak terjangkau (%v) — lewati", err)
	}
	t.Cleanup(pool.Close)
	return pool, database.NewStores(pool), database.NewTxManager(pool)
}

// invWorld adalah organisasi uji: user + 2 gudang + 1 produk + 2 varian.
type invWorld struct {
	org, user, whA, whB, product, variantA, variantB string
	// Tenant kedua untuk uji guard lintas organisasi.
	otherOrg, otherVariant string
}

func seedInvWorld(t *testing.T, pool *pgxpool.Pool) *invWorld {
	t.Helper()
	ctx := context.Background()
	w := &invWorld{}
	var addrA, addrB, provinceID, cityID, districtID, otherOrg, otherUser, otherProduct, otherVariant string
	for _, target := range []*string{&w.org, &w.user, &w.whA, &w.whB, &w.product,
		&w.variantA, &w.variantB, &provinceID, &cityID, &districtID, &addrA, &addrB,
		&otherOrg, &otherUser, &otherProduct, &otherVariant} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(target); err != nil {
			t.Fatalf("gen id: %v", err)
		}
	}
	if err := pool.QueryRow(ctx, `
		SELECT p.id, c.id, d.id FROM postal_codes pc
		JOIN districts d ON d.id = pc.district_id
		JOIN cities c ON c.id = d.city_id
		JOIN provinces p ON p.id = c.province_id
		LIMIT 1`).Scan(&provinceID, &cityID, &districtID); err != nil {
		t.Fatalf("ambil wilayah: %v", err)
	}

	must := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	must(`INSERT INTO users (id, email, name, password_hash, status) VALUES ($1,$2,'Uji Inv','x','active')`,
		w.user, "inv-"+w.user[:8]+"@angkut.test")
	must(`INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		w.org, "Org Inventory", "inv-"+w.org[:8], w.user)
	for i, wh := range []*string{&w.whA, &w.whB} {
		addrID := addrA
		if i == 1 {
			addrID = addrB
		}
		must(`INSERT INTO addresses (id, organization_id, recipient, phone, province_id, city_id,
			district_id, street, status, created_by, created_at, updated_at)
			VALUES ($1,$2,'Gudang','081200000000',$3,$4,$5,'Jl. Gudang','active',$6,now(),now())`,
			addrID, w.org, provinceID, cityID, districtID, w.user)
		must(`INSERT INTO warehouses (id, organization_id, name, address_id, is_active)
			VALUES ($1,$2,$3,$4,true)`, *wh, w.org, "Gudang "+string(rune('A'+i)), addrID)
	}
	must(`INSERT INTO products (id, organization_id, name, category, is_active, created_by)
		VALUES ($1,$2,'Kaos Polos','apparel',true,$3)`, w.product, w.org, w.user)
	must(`INSERT INTO product_variants (id, organization_id, product_id, sku, low_stock_threshold)
		VALUES ($1,$2,$3,'KAOS-M',3)`, w.variantA, w.org, w.product)
	must(`INSERT INTO product_variants (id, organization_id, product_id, sku, low_stock_threshold)
		VALUES ($1,$2,$3,'KAOS-L',3)`, w.variantB, w.org, w.product)

	// Organisasi kedua + variannya (untuk uji tenant guard).
	must(`INSERT INTO users (id, email, name, password_hash, status) VALUES ($1,$2,'Lain','x','active')`,
		otherUser, "inv-lain-"+otherUser[:8]+"@angkut.test")
	must(`INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,'Org Inv Lain','inv-lain',$2)`,
		otherOrg, otherUser)
	must(`INSERT INTO products (id, organization_id, name, is_active) VALUES ($1,$2,'Produk Lain',true)`,
		otherProduct, otherOrg)
	must(`INSERT INTO product_variants (id, organization_id, product_id, sku) VALUES ($1,$2,$3,'LAIN-1')`,
		otherVariant, otherOrg, otherProduct)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM stock_transfer_items WHERE transfer_id IN (SELECT id FROM stock_transfers WHERE organization_id IN ($1,$2))`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM stock_transfers WHERE organization_id IN ($1,$2)`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM inventory_movements WHERE organization_id IN ($1,$2)`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM inventory WHERE organization_id IN ($1,$2)`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM product_variants WHERE organization_id IN ($1,$2)`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM products WHERE organization_id IN ($1,$2)`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM warehouses WHERE organization_id=$1`, w.org)
		_, _ = pool.Exec(ctx, `DELETE FROM addresses WHERE organization_id=$1`, w.org)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id IN ($1,$2)`, w.org, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, w.user, otherUser)
	})
	// Simpan ke struct untuk pemakaian test tenant-guard.
	w.otherOrg = otherOrg
	w.otherVariant = otherVariant
	return w
}

func TestInventoryAdjustDanGuard(t *testing.T) {
	pool, stores, tx := invTestDB(t)
	w := seedInvWorld(t, pool)
	svc := inventoryapp.New(inventoryapp.Deps{Tx: tx, Stores: stores})
	ctx := context.Background()

	// Adjust +10.
	bal, err := svc.Adjust(ctx, w.org, "req-1", inventoryapp.AdjustInput{
		VariantID: w.variantA, WarehouseID: w.whA, QtyDelta: 10, Reason: "stok awal", Actor: w.user,
	})
	if err != nil {
		t.Fatalf("adjust +10: %v", err)
	}
	if bal.QtyOnHand != 10 {
		t.Fatalf("on_hand = %d, ingin 10", bal.QtyOnHand)
	}

	// Adjust −15 → ditolak (saldo negatif).
	_, err = svc.Adjust(ctx, w.org, "req-2", inventoryapp.AdjustInput{
		VariantID: w.variantA, WarehouseID: w.whA, QtyDelta: -15, Actor: w.user,
	})
	assertValidation(t, err, "adjust −15")

	// Reservasi 3 → available 7; koreksi −9 ditolak karena menggerus reserved.
	if err := svc.Reserve(ctx, w.org, w.variantA, w.whA, 3, "pesanan #1", w.user); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := svc.Adjust(ctx, w.org, "req-3", inventoryapp.AdjustInput{
		VariantID: w.variantA, WarehouseID: w.whA, QtyDelta: -9, Actor: w.user,
	}); err == nil {
		t.Fatal("adjust −9 saat reserved 3 harus ditolak")
	}
	// Release 3 lagi.
	if err := svc.Release(ctx, w.org, w.variantA, w.whA, 3, "pesanan batal", w.user); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestInventoryTransferAtomik(t *testing.T) {
	pool, stores, tx := invTestDB(t)
	w := seedInvWorld(t, pool)
	svc := inventoryapp.New(inventoryapp.Deps{Tx: tx, Stores: stores})
	ctx := context.Background()

	if _, err := svc.Adjust(ctx, w.org, "r1", inventoryapp.AdjustInput{
		VariantID: w.variantA, WarehouseID: w.whA, QtyDelta: 10, Actor: w.user,
	}); err != nil {
		t.Fatalf("seed saldo: %v", err)
	}

	// Transfer sah: 4 pcs A dari A→B.
	tr, err := svc.Transfer(ctx, w.org, "r2", inventoryapp.TransferInput{
		FromWarehouse: w.whA, ToWarehouse: w.whB, Notes: "pindah gudang",
		Items: []inventory.TransferItem{{VariantID: w.variantA, Qty: 4}}, Actor: w.user,
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if len(tr.Items) != 1 {
		t.Fatalf("items = %d", len(tr.Items))
	}
	a, err := stores.Inventory().Balance(ctx, w.org, w.variantA, w.whA)
	if err != nil || a.QtyOnHand != 6 {
		t.Fatalf("saldo asal = %d (err=%v), ingin 6", a.QtyOnHand, err)
	}
	b, err := stores.Inventory().Balance(ctx, w.org, w.variantA, w.whB)
	if err != nil || b.QtyOnHand != 4 {
		t.Fatalf("saldo tujuan = %d (err=%v), ingin 4", b.QtyOnHand, err)
	}
	movs, total, err := stores.Inventory().ListMovements(ctx, w.org, inventory.MovementFilter{VariantID: w.variantA, Limit: 10})
	if err != nil || total != 3 {
		// 3 = adjust awal (seed) + transfer_out + transfer_in — semua perubahan
		// saldo wajib berjejak, termasuk saldo awal.
		t.Fatalf("movement = %d (err=%v), ingin 3", total, err)
	}
	kinds := map[inventory.MovementType]int{}
	for _, m := range movs {
		kinds[m.Type]++
	}
	if kinds[inventory.MovementTransferOut] != 1 || kinds[inventory.MovementTransferIn] != 1 {
		t.Errorf("movement transfer salah: %+v", movs)
	}

	// Transfer melebihi saldo → ditolak seluruhnya (atomik).
	_, err = svc.Transfer(ctx, w.org, "r3", inventoryapp.TransferInput{
		FromWarehouse: w.whA, ToWarehouse: w.whB,
		Items: []inventory.TransferItem{{VariantID: w.variantA, Qty: 999}}, Actor: w.user,
	})
	assertValidation(t, err, "transfer 999")
	bAgain, _ := stores.Inventory().Balance(ctx, w.org, w.variantA, w.whB)
	if bAgain.QtyOnHand != 4 {
		t.Fatalf("transfer gagal tidak boleh mengubah saldo tujuan (%d)", bAgain.QtyOnHand)
	}

	// Varian milik org lain → 422 (guard tenant di service).
	_, err = svc.Transfer(ctx, w.org, "r4", inventoryapp.TransferInput{
		FromWarehouse: w.whA, ToWarehouse: w.whB,
		Items: []inventory.TransferItem{{VariantID: w.otherVariant, Qty: 1}}, Actor: w.user,
	})
	assertValidation(t, err, "varian org lain")

	// Gudang asal = tujuan → 422.
	_, err = svc.Transfer(ctx, w.org, "r5", inventoryapp.TransferInput{
		FromWarehouse: w.whA, ToWarehouse: w.whA,
		Items: []inventory.TransferItem{{VariantID: w.variantA, Qty: 1}}, Actor: w.user,
	})
	assertValidation(t, err, "gudang sama")
}

func TestInventoryLowStock(t *testing.T) {
	pool, stores, tx := invTestDB(t)
	w := seedInvWorld(t, pool)
	svc := inventoryapp.New(inventoryapp.Deps{Tx: tx, Stores: stores})
	ctx := context.Background()

	// varian A: on_hand 3, reserved 1 → available 2 ≤ threshold 3.
	if _, err := svc.Adjust(ctx, w.org, "r1", inventoryapp.AdjustInput{
		VariantID: w.variantA, WarehouseID: w.whA, QtyDelta: 3, Actor: w.user,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.Reserve(ctx, w.org, w.variantA, w.whA, 1, "pesanan", w.user); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// varian B: on_hand 10 → tidak low.
	if _, err := svc.Adjust(ctx, w.org, "r2", inventoryapp.AdjustInput{
		VariantID: w.variantB, WarehouseID: w.whA, QtyDelta: 10, Actor: w.user,
	}); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	rows, err := svc.LowStock(ctx, w.org, "")
	if err != nil {
		t.Fatalf("LowStock: %v", err)
	}
	if len(rows) != 1 || rows[0].VariantID != w.variantA || rows[0].QtyAvailable != 2 {
		t.Fatalf("low-stock = %+v, ingin hanya varian A dengan available 2", rows)
	}

	// Filter stok juga memperlihatkan available.
	stocks, total, err := svc.ListStock(ctx, w.org, inventoryapp.StockFilter{Limit: 10})
	if err != nil || total != 2 {
		t.Fatalf("ListStock total = %d (err=%v), ingin 2", total, err)
	}
	for _, sr := range stocks {
		if sr.VariantID == w.variantA && sr.QtyOnHand-sr.QtyReserved != 2 {
			t.Errorf("available varian A = %d, ingin 2", sr.QtyOnHand-sr.QtyReserved)
		}
	}
}
