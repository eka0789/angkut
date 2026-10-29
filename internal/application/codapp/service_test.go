package codapp_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/application/codapp"
	"github.com/angkut/angkut/apps/internal/application/returnsapp"
	"github.com/angkut/angkut/apps/internal/domain/cod"
	"github.com/angkut/angkut/apps/internal/domain/ledger"
	"github.com/angkut/angkut/apps/internal/domain/returns"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// Uji COD + settlement + ledger + retur terhadap PostgreSQL nyata. Dilewati
// bila DATABASE_URL tidak tersedia — CI selalu menyetelnya.

func moneyTestDB(t *testing.T) (*pgxpool.Pool, *database.Stores, *database.TxManager) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL kosong — lewati uji money")
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

// moneyWorld: org + user + shipment COD + shipment non-COD.
type moneyWorld struct {
	org, user, codShipment, plainShipment, wh string
}

func seedMoneyWorld(t *testing.T, pool *pgxpool.Pool) *moneyWorld {
	t.Helper()
	ctx := context.Background()
	w := &moneyWorld{}
	var addr, provinceID, cityID, districtID string
	for _, target := range []*string{&w.org, &w.user, &addr, &w.wh, &w.codShipment, &w.plainShipment,
		&provinceID, &cityID, &districtID} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(target); err != nil {
			t.Fatalf("gen id: %v", err)
		}
	}
	if err := pool.QueryRow(ctx, `
		SELECT p.id, c.id, d.id FROM postal_codes pc
		JOIN districts d ON d.id = pc.district_id
		JOIN cities c ON c.id = d.city_id
		JOIN provinces p ON p.id = c.province_id LIMIT 1`).
		Scan(&provinceID, &cityID, &districtID); err != nil {
		t.Fatalf("ambil wilayah: %v", err)
	}
	must := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	must(`INSERT INTO users (id, email, name, password_hash, status) VALUES ($1,$2,'Uji Money','x','active')`,
		w.user, "money-"+w.user[:8]+"@angkut.test")
	must(`INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		w.org, "Org Money", "money-"+w.org[:8], w.user)
	must(`INSERT INTO addresses (id, organization_id, recipient, phone, province_id, city_id,
		district_id, street, status, created_by, created_at, updated_at)
		VALUES ($1,$2,'Gudang','081200000000',$3,$4,$5,'Jl. Uji','active',$6,now(),now())`,
		addr, w.org, provinceID, cityID, districtID, w.user)
	must(`INSERT INTO warehouses (id, organization_id, name, address_id, is_active) VALUES ($1,$2,'Gudang Money',$3,true)`,
		w.wh, w.org, addr)
	for _, sh := range []struct{ id, awb string }{{w.codShipment, "MNY" + w.codShipment[:7]}, {w.plainShipment, "MNY" + w.plainShipment[:7]}} {
		must(`INSERT INTO shipments (id, organization_id, warehouse_id, courier_code, service_code,
			awb, status, actual_weight, chargeable_weight, is_cod, cod_amount, cod_fee, version)
			VALUES ($1,$2,$3,'jne','REG',$4,'pending_pickup',1,1,$5,$6,$7,1)`,
			sh.id, w.org, w.wh, sh.awb, sh.id == w.codShipment,
			map[bool]int64{true: 500000, false: 0}[sh.id == w.codShipment],
			map[bool]int64{true: 2500, false: 0}[sh.id == w.codShipment])
	}
	t.Cleanup(func() {
		for _, org := range []string{w.org} {
			_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM cod_settlement_items WHERE settlement_id IN (SELECT id FROM cod_settlements WHERE organization_id=$1)`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM cod_transactions WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM cod_settlements WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM return_items WHERE return_id IN (SELECT id FROM returns WHERE organization_id=$1)`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM returns WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM shipments WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM warehouses WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM addresses WHERE organization_id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, org)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, w.user)
		}
	})
	return w
}

// TestCODCycle: issue → collect saat delivered → settlement → mark-paid
// (ledger tercatat sekali) → replay mark-paid 409.
func TestCODCycle(t *testing.T) {
	pool, stores, tx := moneyTestDB(t)
	w := seedMoneyWorld(t, pool)
	svc := codapp.New(codapp.Deps{Tx: tx, Stores: stores})
	ctx := context.Background()

	// Issue dalam tx (simulasi shipmentapp).
	if err := tx.InTx(ctx, func(q database.Querier) error {
		return svc.IssueTransaction(ctx, q, w.org, w.codShipment, "jne", 500000, 2500)
	}); err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Issue kedua untuk shipment sama → 409 (unique per shipment).
	if err := tx.InTx(ctx, func(q database.Querier) error {
		return svc.IssueTransaction(ctx, q, w.org, w.codShipment, "jne", 500000, 2500)
	}); err == nil || !shared.Is(err, shared.ErrConflict) {
		t.Fatalf("issue duplikat harus 409; err=%v", err)
	}

	// Collect saat delivered.
	if err := tx.InTx(ctx, func(q database.Querier) error {
		return svc.CollectShipment(ctx, q, w.org, w.codShipment, time.Now().UTC())
	}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	// Collect pengiriman non-COD = bukan kegagalan.
	if err := tx.InTx(ctx, func(q database.Querier) error {
		return svc.CollectShipment(ctx, q, w.org, w.plainShipment, time.Now().UTC())
	}); err != nil {
		t.Fatalf("collect non-COD harus diam; err=%v", err)
	}

	sum, err := svc.Summary(ctx, w.org)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Collected != 500000 || sum.ExpectedSettlement != 500000 {
		t.Fatalf("summary = %+v", sum)
	}

	// Settlement: transaksi bukan milik/kurir salah → 422.
	if _, err := svc.Submit(ctx, w.org, "r1", codapp.SubmitInput{
		CourierCode: "sicepat", PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31",
		TransactionIDs: []string{w.codShipment}, Actor: w.user,
	}); err == nil {
		t.Fatal("settlement kurir berbeda harus ditolak")
	}

	txn, terr := stores.COD().GetByShipment(ctx, w.org, w.codShipment)
	if terr != nil {
		t.Fatalf("ambil transaksi COD: %v", terr)
	}
	st, err := svc.Submit(ctx, w.org, "r2", codapp.SubmitInput{
		CourierCode: "jne", PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31",
		TransactionIDs: []string{txn.ID}, Actor: w.user,
	})
	if err != nil {
		t.Fatalf("submit settlement: %v", err)
	}
	if st.TotalAmount != 500000 || st.TotalFee != 2500 {
		t.Fatalf("settlement = %+v", st)
	}
	// Transaksi pindah ke settling.
	txn, _ = stores.COD().GetByShipment(ctx, w.org, w.codShipment)
	if txn.Status != cod.StatusSettling {
		t.Fatalf("status transaksi = %s, ingin settling", txn.Status)
	}

	// Mark-paid → ledger settlement_clearing tercatat.
	if _, err := svc.MarkPaid(ctx, w.org, st.ID, "r3", codapp.MarkPaidInput{
		Reference: "TRF-123", Actor: w.user,
	}); err != nil {
		t.Fatalf("mark-paid: %v", err)
	}
	exists, _ := stores.Ledger().ExistsForRef(ctx, w.org, ledger.AccountSettlementClear, "cod_settlement", st.ID)
	if !exists {
		t.Fatal("entri ledger settlement_clearing tidak ditemukan")
	}
	// Replay mark-paid → 409.
	if _, err := svc.MarkPaid(ctx, w.org, st.ID, "r4", codapp.MarkPaidInput{
		Reference: "TRF-123", Actor: w.user,
	}); err == nil || !shared.Is(err, shared.ErrConflict) {
		t.Fatalf("mark-paid kedua harus 409; err=%v", err)
	}
	// Transaksi akhir = settled.
	txn, _ = stores.COD().GetByShipment(ctx, w.org, w.codShipment)
	if txn.Status != cod.StatusSettled {
		t.Fatalf("status akhir = %s, ingin settled", txn.Status)
	}
}

// TestReturnsFlow: create → window ditolak → approve → receive → inspect →
// refund idempoten (ledger unik per return).
func TestReturnsFlow(t *testing.T) {
	pool, stores, tx := moneyTestDB(t)
	w := seedMoneyWorld(t, pool)
	svc := returnsapp.New(returnsapp.Deps{Tx: tx, Stores: stores})
	ctx := context.Background()

	// Delivered 30 hari lalu → window 7 hari ditolak.
	delivered := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE shipments SET status='delivered', delivered_at=$3 WHERE id=$1::uuid AND organization_id=$2`,
		w.codShipment, w.org, delivered); err != nil {
		t.Fatalf("set delivered: %v", err)
	}
	in := returnsapp.CreateInput{
		ShipmentID: w.codShipment, Type: "refund", Reason: "barang rusak", Actor: w.user,
		RefundAmount: 100000,
		Items: []struct {
			OrderItemID string
			Name        string
			Qty         int
		}{{Name: "Kaos", Qty: 1}},
	}
	if _, err := svc.Create(ctx, w.org, "r1", in); err == nil {
		t.Fatal("retur melewati window harus ditolak")
	}

	// Kebijakan diperluas ke 60 hari → auto-approve tidak aktif (limit 0).
	if _, err := svc.UpdatePolicy(ctx, w.org, "r2", returnsapp.UpdatePolicyInput{
		WindowDays: 60, AutoApproveLimit: 0, Actor: w.user,
	}); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	ret, err := svc.Create(ctx, w.org, "r3", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ret.Status != returns.StatusRequested {
		t.Fatalf("status = %s, ingin requested (limit 0 = manual)", ret.Status)
	}

	// Auto-approve dengan limit ≥ refund.
	if _, err := svc.UpdatePolicy(ctx, w.org, "r4", returnsapp.UpdatePolicyInput{
		WindowDays: 60, AutoApproveLimit: 500000, Actor: w.user,
	}); err != nil {
		t.Fatalf("policy 2: %v", err)
	}
	ret2, err := svc.Create(ctx, w.org, "r5", in)
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if ret2.Status != returns.StatusApproved {
		t.Fatalf("auto-approve gagal: %s", ret2.Status)
	}

	// Flow manual: approve → receive → inspect → refund.
	if _, err := svc.Decide(ctx, w.org, ret.ID, "r6", w.user, true, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := svc.Receive(ctx, w.org, ret.ID, w.user); err != nil {
		t.Fatalf("receive: %v", err)
	}
	items := ret.Items
	in2 := returnsapp.InspectInput{Restock: false, Actor: w.user}
	for _, it := range items {
		in2.Items = append(in2.Items, struct {
			ItemID    string
			Condition string
		}{it.ID, "damaged"})
	}
	if _, err := svc.Inspect(ctx, w.org, ret.ID, "r7", in2); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if _, err := svc.Refund(ctx, w.org, ret.ID, "r8", returnsapp.RefundInput{
		Amount: 100000, Method: "transfer", Actor: w.user,
	}); err != nil {
		t.Fatalf("refund: %v", err)
	}
	// Refund kedua → 409 (ledger unik).
	if _, err := svc.Refund(ctx, w.org, ret.ID, "r9", returnsapp.RefundInput{
		Amount: 100000, Method: "transfer", Actor: w.user,
	}); err == nil || !shared.Is(err, shared.ErrConflict) {
		t.Fatalf("refund ganda harus 409; err=%v", err)
	}
	exists, _ := stores.Ledger().ExistsForRef(ctx, w.org, ledger.AccountRefund, "return", ret.ID)
	if !exists {
		t.Fatal("entri ledger refund tidak ditemukan")
	}

	// Lompatan status ditolak: requested → refunded langsung.
	ret3, err := svc.Create(ctx, w.org, "r10", in)
	if err != nil {
		t.Fatalf("create 3: %v", err)
	}
	if _, err := svc.Refund(ctx, w.org, ret3.ID, "r11", returnsapp.RefundInput{
		Amount: 1, Method: "transfer", Actor: w.user,
	}); err == nil {
		t.Fatal("lompatan requested→refunded harus ditolak")
	}
}
