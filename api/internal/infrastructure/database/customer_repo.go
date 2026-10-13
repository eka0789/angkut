package database

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/angkut/angkut/apps/api/internal/domain/customer"
)

// CustomerRepo menyimpan pelanggan tenant-scoped.
// Semua query memfilter organization_id dan mengecualikan baris soft-deleted.
type CustomerRepo struct{ q Querier }

const custCols = `id, organization_id, name, phone, COALESCE(email,''), COALESCE(notes,''),
	COALESCE(default_address_id::text,''), segment, segment_source, metrics, created_at, updated_at`

func (r *CustomerRepo) Create(ctx context.Context, c *customer.Customer) error {
	mb, err := json.Marshal(c.Metrics)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO customers (
			id, organization_id, name, phone, email, notes, default_address_id,
			segment, segment_source, metrics, created_at, updated_at
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7::uuid,''),$8,$9,CAST($10 AS jsonb),$11,$12)`,
		c.ID, c.OrganizationID, c.Name, c.Phone, c.Email, c.Notes, c.DefaultAddressID,
		string(c.Segment), c.SegmentSource, string(mb), c.CreatedAt, c.UpdatedAt,
	)
	if isUniqueCustomerPhone(err) {
		return customer.ErrPhoneTaken
	}
	return err
}

func (r *CustomerRepo) GetByID(ctx context.Context, orgID, id string) (*customer.Customer, error) {
	row := r.q.QueryRow(ctx, `SELECT `+custCols+` FROM customers WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`, orgID, id)
	return scanCust(row)
}

func (r *CustomerRepo) GetByPhone(ctx context.Context, orgID, phone string) (*customer.Customer, error) {
	row := r.q.QueryRow(ctx, `SELECT `+custCols+` FROM customers WHERE organization_id=$1 AND phone=$2 AND deleted_at IS NULL`, orgID, phone)
	return scanCust(row)
}

func (r *CustomerRepo) Update(ctx context.Context, c *customer.Customer) error {
	mb, err := json.Marshal(c.Metrics)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE customers SET name=$3, phone=$4, email=NULLIF($5,''), notes=NULLIF($6,''),
			default_address_id=NULLIF($7::uuid,''), segment=$8, segment_source=$9, metrics=CAST($10 AS jsonb)
		WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`,
		c.OrganizationID, c.ID, c.Name, c.Phone, c.Email, c.Notes, c.DefaultAddressID,
		string(c.Segment), c.SegmentSource, string(mb),
	)
	if err != nil {
		if isUniqueCustomerPhone(err) {
			return customer.ErrPhoneTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return customer.ErrNotFound
	}
	return nil
}

func (r *CustomerRepo) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE customers SET deleted_at=now()
		WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return customer.ErrNotFound
	}
	return nil
}

// List mengembalikan halaman pelanggan + total jumlah baris yang cocok filter.
func (r *CustomerRepo) List(ctx context.Context, orgID string, segment customer.Segment, query string, limit, offset int) ([]customer.Customer, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	where := `organization_id=$1 AND deleted_at IS NULL`
	args := []any{orgID}
	if segment != "" {
		args = append(args, string(segment))
		where += ` AND segment=$2`
	}
	if query != "" {
		// Search di nama/telepon/email — trigram index sudah tersedia.
		args = append(args, query)
		where += ` AND (name ILIKE '%'||$` + itoa(len(args)) + `||'%'
			OR phone ILIKE '%'||$` + itoa(len(args)) + `||'%'
			OR (email IS NOT NULL AND email ILIKE '%'||$` + itoa(len(args)) + `||'%'))`
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM customers WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.q.Query(ctx,
		`SELECT `+custCols+` FROM customers WHERE `+where+
			` ORDER BY updated_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]customer.Customer, 0)
	for rows.Next() {
		c, err := scanCust(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

func scanCust(row pgx.Row) (*customer.Customer, error) {
	var (
		c         customer.Customer
		metricsB  []byte
		createdAt time.Time
	)
	err := row.Scan(
		&c.ID, &c.OrganizationID, &c.Name, &c.Phone, &c.Email, &c.Notes,
		&c.DefaultAddressID, &c.Segment, &c.SegmentSource, &metricsB, &createdAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, customer.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(metricsB) > 0 {
		_ = json.Unmarshal(metricsB, &c.Metrics)
	}
	c.CreatedAt = createdAt
	return &c, nil
}

// isUniqueCustomerPhone mendeteksi pelanggaran unique index (org, phone).
func isUniqueCustomerPhone(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "customers_org_phone_idx"
	}
	return false
}

// itoA alias agar pemanggilan inline tetap singkat.
func itoa(n int) string { return strconv.Itoa(n) }
