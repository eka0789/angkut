package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/address"
)

// AddressRepo menyimpan alamat tenant-scoped.
// Semua query memfilter organization_id — tidak ada jalur yang mengembalikan baris org lain.
type AddressRepo struct{ q Querier }

const addrCols = `id, organization_id, recipient, phone, province_id, city_id, district_id,
	COALESCE(village_id,''), COALESCE(postal_code,''), street, COALESCE(notes,''),
	latitude, longitude, COALESCE(label,''), is_default, status, created_by, created_at, updated_at`

func (r *AddressRepo) Create(ctx context.Context, a *address.Address) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO addresses (
			id, organization_id, recipient, phone, province_id, city_id, district_id,
			village_id, postal_code, street, notes, latitude, longitude, label,
			is_default, status, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,NULLIF($11,''),$12,$13,NULLIF($14,''),$15,$16,$17,$18,$19)`,
		a.ID, a.OrganizationID, a.Recipient, a.Phone, a.ProvinceID, a.CityID, a.DistrictID,
		a.VillageID, a.PostalCode, a.Street, a.Notes, a.Latitude, a.Longitude, a.Label,
		a.IsDefault, string(a.Status), a.CreatedBy, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (r *AddressRepo) GetByID(ctx context.Context, orgID, id string) (*address.Address, error) {
	row := r.q.QueryRow(ctx, `SELECT `+addrCols+` FROM addresses WHERE organization_id=$1 AND id=$2`, orgID, id)
	return scanAddr(row)
}

func (r *AddressRepo) Update(ctx context.Context, a *address.Address) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE addresses SET recipient=$3, phone=$4, province_id=$5, city_id=$6, district_id=$7,
			village_id=NULLIF($8,''), postal_code=NULLIF($9,''), street=$10, notes=NULLIF($11,''),
			latitude=$12, longitude=$13, label=NULLIF($14,''), is_default=$15, status=$16
		WHERE organization_id=$1 AND id=$2`,
		a.OrganizationID, a.ID, a.Recipient, a.Phone, a.ProvinceID, a.CityID, a.DistrictID,
		a.VillageID, a.PostalCode, a.Street, a.Notes, a.Latitude, a.Longitude, a.Label,
		a.IsDefault, string(a.Status),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return address.ErrNotFound
	}
	return nil
}

func (r *AddressRepo) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM addresses WHERE organization_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return address.ErrNotFound
	}
	return nil
}

func (r *AddressRepo) List(ctx context.Context, orgID string, status address.Status) ([]address.Address, error) {
	q := `SELECT ` + addrCols + ` FROM addresses WHERE organization_id=$1`
	args := []any{orgID}
	if status != "" {
		q += ` AND status=$2`
		args = append(args, string(status))
	}
	q += ` ORDER BY is_default DESC, created_at DESC`
	rows, err := r.q.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]address.Address, 0)
	for rows.Next() {
		a, err := scanAddr(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *AddressRepo) Search(ctx context.Context, orgID, query string, limit int) ([]address.Address, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.q.Query(ctx, `
		SELECT `+addrCols+` FROM addresses
		WHERE organization_id=$1 AND status='active'
		  AND (recipient ILIKE '%'||$2||'%' OR street ILIKE '%'||$2||'%' OR COALESCE(label,'') ILIKE '%'||$2||'%')
		ORDER BY is_default DESC, created_at DESC LIMIT $3`, orgID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]address.Address, 0)
	for rows.Next() {
		a, err := scanAddr(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *AddressRepo) UnsetDefaultsExcept(ctx context.Context, orgID, exceptID string) error {
	_, err := r.q.Exec(ctx, `UPDATE addresses SET is_default=false WHERE organization_id=$1 AND id<>$2 AND is_default=true`, orgID, exceptID)
	return err
}

func scanAddr(row pgx.Row) (*address.Address, error) {
	var a address.Address
	var status string
	err := row.Scan(
		&a.ID, &a.OrganizationID, &a.Recipient, &a.Phone, &a.ProvinceID, &a.CityID, &a.DistrictID,
		&a.VillageID, &a.PostalCode, &a.Street, &a.Notes,
		&a.Latitude, &a.Longitude, &a.Label, &a.IsDefault, &status, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, address.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Status = address.Status(status)
	return &a, nil
}

// unused import guard
var _ = time.Time{}
