package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/domain/bulk"
	"github.com/angkut/angkut/apps/internal/platform"
)

// seedBulkFixture menyiapkan org + user + gudang minimal untuk FK bulk_jobs.
func seedBulkFixture(t *testing.T, pool *pgxpool.Pool) (orgID, userID, warehouseID string) {
	t.Helper()
	ctx := context.Background()

	var addrID, provinceID, cityID, districtID string
	for _, target := range []*string{&orgID, &userID, &warehouseID, &addrID} {
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
		t.Fatalf("ambil wilayah seed: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, status)
		VALUES ($1,$2,'Uji Bulk','x','active')`, userID, "bulk-"+userID[:8]+"@angkut.test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		orgID, "Org Uji Bulk", "bulk-"+orgID[:8], userID); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO addresses (id, organization_id, recipient, phone, province_id, city_id,
			district_id, street, status, created_by, created_at, updated_at)
		VALUES ($1,$2,'Gudang','081200000000',$3,$4,$5,'Jl. Gudang','active',$6,now(),now())`,
		addrID, orgID, provinceID, cityID, districtID, userID); err != nil {
		t.Fatalf("seed address: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO warehouses (id, organization_id, name, address_id, is_active)
		VALUES ($1,$2,'Gudang Bulk',$3,true)`, warehouseID, orgID, addrID); err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM bulk_job_rows WHERE job_id IN (SELECT id FROM bulk_jobs WHERE organization_id=$1)`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM bulk_jobs WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM bulk_uploads WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM warehouses WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM addresses WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	return orgID, userID, warehouseID
}

// TestBulkRepoJobRowsIndependen: baris invalid tetap tersimpan sebagai laporan,
// dan progres hanya menghitung baris yang dieksekusi.
func TestBulkRepoJobRowsIndependen(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, userID, whID := seedBulkFixture(t, pool)
	ctx := context.Background()
	repo := stores.Bulk()

	now := time.Now().UTC()
	job := &bulk.Job{
		ID: platform.NewUUID(), OrganizationID: orgID,
		IdempotencyKey: "uji-bulk-1", Status: bulk.JobPending,
		CourierCode: "jne", ServiceCode: "REG", OriginWarehouseID: whID,
		TotalRows: 3, AcceptedRows: 2, RejectedRows: 1,
		CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
	}
	rows := []bulk.RowResult{
		{JobID: job.ID, RowNo: 1, Status: bulk.StatusValid, CreatedAt: now,
			Row: bulk.Row{RecipientName: "A", RecipientPhone: "081234567890", Street: "Jl. A", PostalCode: "50241", WeightKG: 1}},
		{JobID: job.ID, RowNo: 2, Status: bulk.StatusValid, CreatedAt: now,
			Row: bulk.Row{RecipientName: "B", RecipientPhone: "081234567891", Street: "Jl. B", PostalCode: "50241", WeightKG: 2}},
		// Baris ditolak: berat 0 TIDAK boleh ditolak constraint DB — bukti
		// laporan harus bisa disimpan apa adanya.
		{JobID: job.ID, RowNo: 3, Status: bulk.StatusInvalid, CreatedAt: now,
			Issues: []bulk.Issue{{Field: "weight_kg", Message: "Berat harus lebih dari 0."}},
			Row:    bulk.Row{RecipientName: "C", RecipientPhone: "081234567892", Street: "Jl. C", PostalCode: "50241"}},
	}
	if err := repo.CreateJob(ctx, job, rows); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := repo.ListJobRows(ctx, orgID, job.ID)
	if err != nil {
		t.Fatalf("ListJobRows: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("baris = %d, ingin 3", len(got))
	}
	if got[2].Status != bulk.StatusInvalid || len(got[2].Issues) != 1 {
		t.Errorf("baris 3 = %+v, ingin invalid dengan 1 issue", got[2])
	}

	valid, err := repo.ListValidRows(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListValidRows: %v", err)
	}
	if len(valid) != 2 {
		t.Errorf("baris valid = %d, ingin 2", len(valid))
	}

	// Idempotency: key sama untuk org sama ditolak index unik parsial.
	dup := *job
	dup.ID = platform.NewUUID()
	if err := repo.CreateJob(ctx, &dup, nil); err == nil {
		t.Error("job dengan idempotency_key sama seharusnya ditolak")
	}

	// Progres & penyelesaian.
	if err := repo.MarkProcessing(ctx, job.ID, now); err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}
	if err := repo.MarkRowCreating(ctx, job.ID, 1); err != nil {
		t.Fatalf("MarkRowCreating: %v", err)
	}
	shID := platform.NewUUID()
	if err := repo.MarkRowCreated(ctx, job.ID, 1, shID, "TST260101000001"); err != nil {
		t.Fatalf("MarkRowCreated: %v", err)
	}
	if err := repo.MarkRowFailed(ctx, job.ID, 2, "gagal uji"); err != nil {
		t.Fatalf("MarkRowFailed: %v", err)
	}
	if err := repo.UpdateProgress(ctx, job.ID, 2, 1, 1); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if err := repo.FinishJob(ctx, job.ID, bulk.JobCompleted, "1 baris gagal", now); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}

	final, err := repo.GetJob(ctx, orgID, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if final.Status != bulk.JobCompleted || final.ProcessedRows != 2 || final.CreatedRows != 1 || final.FailedRows != 1 {
		t.Errorf("job akhir = %+v", final)
	}
	created, failed, pending, err := repo.CountRowsByStatuses(ctx, job.ID)
	if err != nil {
		t.Fatalf("CountRowsByStatuses: %v", err)
	}
	if created != 1 || failed != 2 || pending != 0 {
		t.Errorf("hitung status: created=%d failed=%d pending=%d", created, failed, pending)
	}

	// Guard tenant: org lain tidak bisa melihat job ini.
	var otherOrg string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&otherOrg); err != nil {
		t.Fatalf("gen org: %v", err)
	}
	if _, err := repo.GetJob(ctx, otherOrg, job.ID); err == nil {
		t.Error("GetJob dengan org lain seharusnya NotFound")
	}
}

// TestBulkRepoUploadRoundTrip: baris unggahan tersimpan & terbaca utuh.
func TestBulkRepoUploadRoundTrip(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, userID, _ := seedBulkFixture(t, pool)
	ctx := context.Background()
	repo := stores.Bulk()

	up := &bulk.Upload{
		ID: platform.NewUUID(), OrganizationID: orgID, Filename: "uji.csv",
		RowCount: 1, CreatedBy: userID, CreatedAt: time.Now().UTC(),
		Rows: []bulk.Row{{
			RecipientName: "Budi", RecipientPhone: "081234567890",
			Street: "Jl. Melati", PostalCode: "50241", WeightKG: 1.5,
			CODAmount: 10000, DeclaredValue: 50000, Insurance: true,
		}},
	}
	if err := repo.CreateUpload(ctx, up); err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	got, err := repo.GetUpload(ctx, orgID, up.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].RecipientName != "Budi" || !got.Rows[0].Insurance {
		t.Errorf("rows = %+v", got.Rows)
	}
	if got.ConsumedAt != nil {
		t.Error("unggahan baru belum boleh ditandai terkonsumsi")
	}
	if err := repo.MarkUploadConsumed(ctx, orgID, up.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkUploadConsumed: %v", err)
	}
	got2, err := repo.GetUpload(ctx, orgID, up.ID)
	if err != nil {
		t.Fatalf("GetUpload kedua: %v", err)
	}
	if got2.ConsumedAt == nil {
		t.Error("ConsumedAt seharusnya terisi setelah dikonsumsi")
	}
}
