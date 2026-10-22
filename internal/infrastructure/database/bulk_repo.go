package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/bulk"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// BulkRepo menyimpan job pengiriman massal & barisnya, tenant-scoped.
// Setiap baris berdiri sendiri — tidak ada transaksi lintas baris di sini.
type BulkRepo struct{ q Querier }

// ── Unggahan ────────────────────────────────────────────────────────────

// CreateUpload menyimpan hasil parse unggahan CSV (baris sebagai jsonb).
func (r *BulkRepo) CreateUpload(ctx context.Context, up *bulk.Upload) error {
	rows, err := json.Marshal(up.Rows)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO bulk_uploads (id, organization_id, filename, row_count, rows, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7)`,
		up.ID, up.OrganizationID, up.Filename, up.RowCount, rows, up.CreatedBy, up.CreatedAt)
	return err
}

// GetUpload mengambil unggahan milik organisasi.
func (r *BulkRepo) GetUpload(ctx context.Context, orgID, id string) (*bulk.Upload, error) {
	var up bulk.Upload
	var rowsRaw []byte
	err := r.q.QueryRow(ctx, `
		SELECT id, organization_id, filename, row_count, rows, created_by, created_at, consumed_at
		FROM bulk_uploads WHERE organization_id=$1 AND id=$2`, orgID, id).
		Scan(&up.ID, &up.OrganizationID, &up.Filename, &up.RowCount, &rowsRaw, &up.CreatedBy, &up.CreatedAt, &up.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("BULK_UPLOAD_NOT_FOUND", "Unggahan tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rowsRaw, &up.Rows); err != nil {
		return nil, err
	}
	return &up, nil
}

// MarkUploadConsumed menandai unggahan sudah dipakai submit (jejak audit).
func (r *BulkRepo) MarkUploadConsumed(ctx context.Context, orgID, id string, at time.Time) error {
	_, err := r.q.Exec(ctx,
		`UPDATE bulk_uploads SET consumed_at=$3 WHERE organization_id=$1 AND id=$2`,
		orgID, id, at)
	return err
}

// ── Job ─────────────────────────────────────────────────────────────────

// CreateJob membuat job BESERTA seluruh barisnya dalam satu transaksi pemanggil
// (panggil lewat WithTx). Baris invalid ikut disimpan sebagai laporan, bukan
// dibuang — pengirim perlu tahu mana yang ditolak dan mengapa.
func (r *BulkRepo) CreateJob(ctx context.Context, job *bulk.Job, rows []bulk.RowResult) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO bulk_jobs (
			id, organization_id, idempotency_key, status, courier_code, service_code,
			origin_warehouse_id, total_rows, accepted_rows, rejected_rows,
			created_by, created_at, updated_at
		) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		job.ID, job.OrganizationID, job.IdempotencyKey, string(job.Status),
		job.CourierCode, job.ServiceCode, job.OriginWarehouseID,
		job.TotalRows, job.AcceptedRows, job.RejectedRows,
		job.CreatedBy, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return err
	}
	for i := range rows {
		if err := r.insertRow(ctx, &rows[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *BulkRepo) insertRow(ctx context.Context, rr *bulk.RowResult) error {
	issues, err := json.Marshal(rr.Issues)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO bulk_job_rows (
			job_id, row_no, recipient_name, recipient_phone, street, postal_code,
			weight_kg, length_cm, width_cm, height_cm, cod_amount, declared_value,
			insurance, notes, status, issues, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15,$16::jsonb,$17)`,
		rr.JobID, rr.RowNo, rr.Row.RecipientName, rr.Row.RecipientPhone, rr.Row.Street, rr.Row.PostalCode,
		rr.Row.WeightKG, rr.Row.LengthCM, rr.Row.WidthCM, rr.Row.HeightCM,
		rr.Row.CODAmount, rr.Row.DeclaredValue, rr.Row.Insurance, rr.Row.Notes,
		string(rr.Status), issues, rr.CreatedAt)
	return err
}

// GetJob mengambil job milik organisasi.
func (r *BulkRepo) GetJob(ctx context.Context, orgID, id string) (*bulk.Job, error) {
	row := r.q.QueryRow(ctx, jobSelect+` WHERE organization_id=$1 AND id=$2`, orgID, id)
	return scanJob(row)
}

// GetJobByKey mengambil job berdasar idempotency key (replay submit).
func (r *BulkRepo) GetJobByKey(ctx context.Context, orgID, key string) (*bulk.Job, error) {
	row := r.q.QueryRow(ctx, jobSelect+` WHERE organization_id=$1 AND idempotency_key=$2`, orgID, key)
	return scanJob(row)
}

// GetJobByID mengambil job tanpa guard organisasi — DIPAKAI HANYA oleh
// prosesor yang sudah memegang job hasil GetJob (bukan dari input klien).
func (r *BulkRepo) GetJobByID(ctx context.Context, id string) (*bulk.Job, error) {
	row := r.q.QueryRow(ctx, jobSelect+` WHERE id=$1`, id)
	return scanJob(row)
}

const jobSelect = `SELECT id, organization_id, COALESCE(idempotency_key,''), status,
	courier_code, service_code, origin_warehouse_id::text,
	total_rows, accepted_rows, rejected_rows, processed_rows, created_rows, failed_rows,
	COALESCE(error_summary,''), created_by, created_at, started_at, finished_at, updated_at
	FROM bulk_jobs`

func scanJob(row pgx.Row) (*bulk.Job, error) {
	var j bulk.Job
	var status string
	err := row.Scan(&j.ID, &j.OrganizationID, &j.IdempotencyKey, &status,
		&j.CourierCode, &j.ServiceCode, &j.OriginWarehouseID,
		&j.TotalRows, &j.AcceptedRows, &j.RejectedRows, &j.ProcessedRows, &j.CreatedRows, &j.FailedRows,
		&j.ErrorSummary, &j.CreatedBy, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("BULK_JOB_NOT_FOUND", "Job massal tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	j.Status = bulk.JobStatus(status)
	return &j, nil
}

// ListJobRows mengambil seluruh baris job (tenant-scoped lewat join job).
func (r *BulkRepo) ListJobRows(ctx context.Context, orgID, jobID string) ([]bulk.RowResult, error) {
	rows, err := r.q.Query(ctx, `
		SELECT j.job_id, j.row_no, j.recipient_name, j.recipient_phone, j.street, j.postal_code,
			j.weight_kg, j.length_cm, j.width_cm, j.height_cm, j.cod_amount, j.declared_value,
			j.insurance, COALESCE(j.notes,''), j.status, j.issues,
			COALESCE(j.shipment_id::text,''), COALESCE(j.awb,''), COALESCE(j.error,''), j.created_at
		FROM bulk_job_rows j
		JOIN bulk_jobs b ON b.id = j.job_id AND b.organization_id = $1
		WHERE j.job_id = $2
		ORDER BY j.row_no`, orgID, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bulk.RowResult
	for rows.Next() {
		rr, err := scanRowResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rr)
	}
	return out, rows.Err()
}

// ListValidRows mengambil baris berstatus valid yang belum dieksekusi —
// daftar kerja prosesor.
func (r *BulkRepo) ListValidRows(ctx context.Context, jobID string) ([]bulk.RowResult, error) {
	rows, err := r.q.Query(ctx, `
		SELECT job_id, row_no, recipient_name, recipient_phone, street, postal_code,
			weight_kg, length_cm, width_cm, height_cm, cod_amount, declared_value,
			insurance, COALESCE(notes,''), status, issues,
			COALESCE(shipment_id::text,''), COALESCE(awb,''), COALESCE(error,''), created_at
		FROM bulk_job_rows WHERE job_id=$1 AND status='valid' ORDER BY row_no`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bulk.RowResult
	for rows.Next() {
		rr, err := scanRowResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rr)
	}
	return out, rows.Err()
}

func scanRowResult(rows pgx.Rows) (*bulk.RowResult, error) {
	var rr bulk.RowResult
	var status string
	var issuesRaw []byte
	err := rows.Scan(&rr.JobID, &rr.RowNo, &rr.Row.RecipientName, &rr.Row.RecipientPhone,
		&rr.Row.Street, &rr.Row.PostalCode, &rr.Row.WeightKG, &rr.Row.LengthCM, &rr.Row.WidthCM,
		&rr.Row.HeightCM, &rr.Row.CODAmount, &rr.Row.DeclaredValue, &rr.Row.Insurance,
		&rr.Row.Notes, &status, &issuesRaw, &rr.ShipmentID, &rr.AWB, &rr.Error, &rr.CreatedAt)
	if err != nil {
		return nil, err
	}
	rr.Status = bulk.Status(status)
	if len(issuesRaw) > 0 {
		if err := json.Unmarshal(issuesRaw, &rr.Issues); err != nil {
			return nil, err
		}
	}
	return &rr, nil
}

// MarkProcessing menandai job mulai diproses.
func (r *BulkRepo) MarkProcessing(ctx context.Context, jobID string, at time.Time) error {
	_, err := r.q.Exec(ctx, `
		UPDATE bulk_jobs SET status='processing', started_at=COALESCE(started_at,$2), updated_at=$2
		WHERE id=$1 AND status IN ('pending','processing')`, jobID, at)
	return err
}

// MarkRowCreating menandai sebuah baris sedang dieksekusi.
func (r *BulkRepo) MarkRowCreating(ctx context.Context, jobID string, rowNo int) error {
	_, err := r.q.Exec(ctx,
		`UPDATE bulk_job_rows SET status='creating' WHERE job_id=$1 AND row_no=$2 AND status='valid'`,
		jobID, rowNo)
	return err
}

// MarkRowCreated menyimpan hasil baris yang berhasil menjadi shipment.
func (r *BulkRepo) MarkRowCreated(ctx context.Context, jobID string, rowNo int, shipmentID, awb string) error {
	_, err := r.q.Exec(ctx, `
		UPDATE bulk_job_rows SET status='created', shipment_id=$3::uuid, awb=$4, error=NULL
		WHERE job_id=$1 AND row_no=$2`, jobID, rowNo, shipmentID, awb)
	return err
}

// MarkRowFailed menyimpan kegagalan baris — baris lain tidak terpengaruh.
func (r *BulkRepo) MarkRowFailed(ctx context.Context, jobID string, rowNo int, msg string) error {
	_, err := r.q.Exec(ctx, `
		UPDATE bulk_job_rows SET status='failed', error=$3
		WHERE job_id=$1 AND row_no=$2`, jobID, rowNo, msg)
	return err
}

// UpdateProgress memperbarui penghitung progres job.
func (r *BulkRepo) UpdateProgress(ctx context.Context, jobID string, processed, created, failed int) error {
	_, err := r.q.Exec(ctx, `
		UPDATE bulk_jobs SET processed_rows=$2, created_rows=$3, failed_rows=$4, updated_at=$5
		WHERE id=$1`, jobID, processed, created, failed, time.Now().UTC())
	return err
}

// FinishJob menutup job dengan status akhir.
func (r *BulkRepo) FinishJob(ctx context.Context, jobID string, status bulk.JobStatus, summary string, at time.Time) error {
	_, err := r.q.Exec(ctx, `
		UPDATE bulk_jobs SET status=$2, error_summary=NULLIF($3,''), finished_at=$4, updated_at=$4
		WHERE id=$1`, jobID, string(status), summary, at)
	return err
}

// CountRowsByStatuses menghitung baris per status untuk job tertentu — dipakai
// penyelarasan progres setelah pemulihan (mis. API mati di tengah job).
func (r *BulkRepo) CountRowsByStatuses(ctx context.Context, jobID string) (created, failed, pending int, err error) {
	err = r.q.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status='created'),
			COUNT(*) FILTER (WHERE status IN ('failed','invalid')),
			COUNT(*) FILTER (WHERE status IN ('valid','creating'))
		FROM bulk_job_rows WHERE job_id=$1`, jobID).Scan(&created, &failed, &pending)
	return created, failed, pending, err
}
