package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/api/internal/domain/order"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
)

// Uji repo order terhadap PostgreSQL nyata. Dilewati bila DATABASE_URL kosong
// (dev tanpa DB) — CI selalu menyetelnya.
func orderTestDB(t *testing.T) (*pgxpool.Pool, *database.Stores) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL kosong — lewati uji repo order")
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
	return pool, database.NewStores(pool)
}

// seedOrderFixtures membuat org, store, customer minimal untuk FK order.
// Mengembalikan (orgID, storeID, customerID) dan membersihkannya di akhir test.
func seedOrderFixtures(t *testing.T, pool *pgxpool.Pool) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	var orgID, storeID, customerID, userID string
	// UUID v7-like: pakai gen_random_uuid() dari DB agar tidak bergantung Go.
	for _, target := range []*string{&orgID, &storeID, &customerID, &userID} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(target); err != nil {
			t.Fatalf("gen id: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, status)
		VALUES ($1,$2,'Pemilik Uji','x','active')`, userID, "uji-"+userID[:8]+"@angkut.test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		orgID, "Org Uji Order", "org-uji-"+orgID[:8], userID); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO stores (id, organization_id, name, channel, adapter_type)
		VALUES ($1,$2,'Toko Uji','manual','mock')`, storeID, orgID); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO customers (id, organization_id, name, phone)
		VALUES ($1::uuid,$2::uuid,'Pelanggan Uji','0812000'||substr($1::text,1,6))`, customerID, orgID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	t.Cleanup(func() {
		// Urutan penting karena FK RESTRICT pada orders.customer_id.
		_, _ = pool.Exec(ctx, `DELETE FROM order_items WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM stores WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	return orgID, storeID, customerID
}

func TestOrderRepoCreateRoundTrip(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, storeID, customerID := seedOrderFixtures(t, pool)
	repo := stores.Orders()
	ctx := context.Background()

	now := time.Now().UTC()
	o := &order.Order{
		ID:             "01920000-0000-7000-8000-0000000a0001",
		OrganizationID: orgID,
		StoreID:        storeID,
		CustomerID:     customerID,
		OrderNumber:    order.FormatOrderNumber(2026, 1),
		Status:         order.StatusDraft,
		PaymentType:    order.PaymentCOD,
		ShippingCost:   21_000,
		CreatedAt:      now,
		UpdatedAt:      now,
		Items: []order.Item{
			{ID: "01920000-0000-7000-8000-0000000b0001", OrganizationID: orgID,
				ProductNameSnapshot: "Sepatu", Qty: 2, UnitPrice: 150_000, CreatedAt: now},
			{ID: "01920000-0000-7000-8000-0000000b0002", OrganizationID: orgID,
				ProductNameSnapshot: "Kaos", Qty: 1, UnitPrice: 75_000, CreatedAt: now},
		},
	}
	o.Recalculate()
	if err := o.Validate(); err != nil {
		t.Fatalf("order valid ditolak: %v", err)
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, o.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.OrderNumber != o.OrderNumber {
		t.Errorf("order_number = %q, ingin %q", got.OrderNumber, o.OrderNumber)
	}
	if len(got.Items) != 2 {
		t.Fatalf("jumlah item = %d, ingin 2", len(got.Items))
	}
	if got.Subtotal != 375_000 || got.TotalAmount != 396_000 {
		t.Errorf("subtotal/total = %d/%d, ingin 375000/396000", got.Subtotal, got.TotalAmount)
	}
	if got.PaymentType != order.PaymentCOD {
		t.Errorf("payment_type = %q, ingin cod", got.PaymentType)
	}
}

func TestOrderRepoTenantIsolation(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, storeID, customerID := seedOrderFixtures(t, pool)
	repo := stores.Orders()
	ctx := context.Background()

	now := time.Now().UTC()
	o := &order.Order{
		ID:             "01920000-0000-7000-8000-0000000a0002",
		OrganizationID: orgID,
		StoreID:        storeID,
		CustomerID:     customerID,
		OrderNumber:    order.FormatOrderNumber(2026, 2),
		Status:         order.StatusDraft,
		PaymentType:    order.PaymentPrepaid,
		CreatedAt:      now,
		UpdatedAt:      now,
		Items: []order.Item{{ID: "01920000-0000-7000-8000-0000000b0010", OrganizationID: orgID,
			ProductNameSnapshot: "Topi", Qty: 1, UnitPrice: 50_000, CreatedAt: now}},
	}
	o.Recalculate()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Organisasi lain tidak boleh melihat order ini — inti isolasi tenant.
	otherOrg := "01920000-0000-7000-8000-00000000ffff"
	if _, err := repo.GetByID(ctx, otherOrg, o.ID); err == nil {
		t.Fatal("order terlihat oleh organisasi lain — isolasi tenant bocor")
	}
}

func TestOrderRepoUniqueNumberRejected(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, storeID, customerID := seedOrderFixtures(t, pool)
	repo := stores.Orders()
	ctx := context.Background()

	now := time.Now().UTC()
	mk := func(id string) *order.Order {
		o := &order.Order{
			ID: id, OrganizationID: orgID, StoreID: storeID, CustomerID: customerID,
			OrderNumber: order.FormatOrderNumber(2027, 7),
			Status:      order.StatusDraft, PaymentType: order.PaymentPrepaid,
			CreatedAt: now, UpdatedAt: now,
			Items: []order.Item{{ID: id[:len(id)-1] + "9", OrganizationID: orgID,
				ProductNameSnapshot: "X", Qty: 1, UnitPrice: 1000, CreatedAt: now}},
		}
		o.Recalculate()
		return o
	}

	if err := repo.Create(ctx, mk("01920000-0000-7000-8000-0000000a0010")); err != nil {
		t.Fatalf("Create pertama: %v", err)
	}
	err := repo.Create(ctx, mk("01920000-0000-7000-8000-0000000a0011"))
	if err == nil {
		t.Fatal("nomor order duplikat harus ditolak")
	}
	if err != order.ErrNumberTaken {
		t.Errorf("error = %v, ingin ErrNumberTaken", err)
	}
}

func TestOrderRepoUpdateStatusIsOptimistic(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, storeID, customerID := seedOrderFixtures(t, pool)
	repo := stores.Orders()
	ctx := context.Background()

	now := time.Now().UTC()
	o := &order.Order{
		ID: "01920000-0000-7000-8000-0000000a0020", OrganizationID: orgID,
		StoreID: storeID, CustomerID: customerID,
		OrderNumber: order.FormatOrderNumber(2028, 1),
		Status:      order.StatusDraft, PaymentType: order.PaymentPrepaid,
		CreatedAt: now, UpdatedAt: now,
		Items: []order.Item{{ID: "01920000-0000-7000-8000-0000000b0020", OrganizationID: orgID,
			ProductNameSnapshot: "X", Qty: 1, UnitPrice: 1000, CreatedAt: now}},
	}
	o.Recalculate()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.UpdateStatus(ctx, orgID, o.ID, order.StatusDraft, order.StatusConfirmed, ""); err != nil {
		t.Fatalf("UpdateStatus pertama: %v", err)
	}
	// Status sudah 'confirmed'; mencoba menerapkan "dari draft" harus gagal
	// (bukti optimistic check mencegah penimpaan).
	if err := repo.UpdateStatus(ctx, orgID, o.ID, order.StatusDraft, order.StatusProcessing, ""); err == nil {
		t.Fatal("update dengan status asal salah harus gagal")
	}
}

func TestOrderRepoNextOrderNumberIncrements(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, storeID, customerID := seedOrderFixtures(t, pool)
	repo := stores.Orders()
	ctx := context.Background()

	seq, err := repo.NextOrderNumber(ctx, orgID, 2030)
	if err != nil {
		t.Fatalf("NextOrderNumber: %v", err)
	}
	if seq != 1 {
		t.Errorf("nomor pertama = %d, ingin 1", seq)
	}

	now := time.Now().UTC()
	o := &order.Order{
		ID: "01920000-0000-7000-8000-0000000a0030", OrganizationID: orgID,
		StoreID: storeID, CustomerID: customerID,
		OrderNumber: order.FormatOrderNumber(2030, seq),
		Status:      order.StatusDraft, PaymentType: order.PaymentPrepaid,
		CreatedAt: now, UpdatedAt: now,
		Items: []order.Item{{ID: "01920000-0000-7000-8000-0000000b0030", OrganizationID: orgID,
			ProductNameSnapshot: "X", Qty: 1, UnitPrice: 1000, CreatedAt: now}},
	}
	o.Recalculate()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	seq2, err := repo.NextOrderNumber(ctx, orgID, 2030)
	if err != nil {
		t.Fatalf("NextOrderNumber kedua: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("nomor berikutnya = %d, ingin 2", seq2)
	}
}
