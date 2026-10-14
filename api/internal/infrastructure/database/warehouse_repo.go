package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/api/internal/domain/store"
)

// WarehouseRepo menyimpan gudang tenant-scoped.
type WarehouseRepo struct{ q Querier }

const warehouseCols = `id, organization_id, name, COALESCE(address_id::text,''),
	capacity, is_default, is_active, created_at, updated_at`

// Create menyimpan gudang baru.
func (r *WarehouseRepo) Create(ctx context.Context, w *store.Warehouse) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO warehouses (
			id, organization_id, name, address_id, capacity, is_default, is_active, created_at, updated_at
		) VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9)`,
		w.ID, w.OrganizationID, w.Name, w.AddressID, w.Capacity, w.IsDefault, w.IsActive,
		w.CreatedAt, w.UpdatedAt,
	)
	if err != nil && isForeignKeyOn(err, "warehouses_address_id_fkey") {
		return store.ErrNotFound
	}
	if err != nil && isUniqueViolation(err) {
		return store.ErrWarehouseDefaultTaken
	}
	return err
}

func (r *WarehouseRepo) GetByID(ctx context.Context, orgID, id string) (*store.Warehouse, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+warehouseCols+` FROM warehouses WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`,
		orgID, id)
	return scanWarehouse(row)
}

// List mengembalikan gudang aktif sebuah organisasi.
func (r *WarehouseRepo) List(ctx context.Context, orgID string, onlyActive bool) ([]store.Warehouse, error) {
	where := `organization_id=$1 AND deleted_at IS NULL`
	if onlyActive {
		where += ` AND is_active`
	}
	rows, err := r.q.Query(ctx,
		`SELECT `+warehouseCols+` FROM warehouses WHERE `+where+` ORDER BY is_default DESC, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.Warehouse, 0)
	for rows.Next() {
		w, err := scanWarehouse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// Default mengembalikan gudang default organisasi, atau ErrNotFound bila belum ada.
func (r *WarehouseRepo) Default(ctx context.Context, orgID string) (*store.Warehouse, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+warehouseCols+` FROM warehouses
		  WHERE organization_id=$1 AND is_default AND deleted_at IS NULL`, orgID)
	return scanWarehouse(row)
}

// Update mengubah data gudang.
func (r *WarehouseRepo) Update(ctx context.Context, w *store.Warehouse) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE warehouses SET name=$3, address_id=NULLIF($4,'')::uuid, capacity=$5,
			is_default=$6, is_active=$7
		WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`,
		w.OrganizationID, w.ID, w.Name, w.AddressID, w.Capacity, w.IsDefault, w.IsActive)
	if err != nil {
		if isUniqueViolation(err) {
			return store.ErrWarehouseDefaultTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrWarehouseNotFound
	}
	return nil
}

func scanWarehouse(row pgx.Row) (*store.Warehouse, error) {
	var (
		w         store.Warehouse
		capacity  *int
		createdAt time.Time
	)
	err := row.Scan(&w.ID, &w.OrganizationID, &w.Name, &w.AddressID,
		&capacity, &w.IsDefault, &w.IsActive, &createdAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrWarehouseNotFound
	}
	if err != nil {
		return nil, err
	}
	w.Capacity = capacity
	w.CreatedAt = createdAt
	return &w, nil
}
