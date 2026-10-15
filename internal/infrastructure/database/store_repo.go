package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/store"
)

// StoreRepo menyimpan kanal penjualan (store) tenant-scoped.
// Kredensial TIDAK pernah dibaca ke struct domain — hanya keberadaannya yang
// dilaporkan lewat HasCredentials agar tidak ada jalur rahasia bocor ke API.
type StoreRepo struct{ q Querier }

const storeCols = `id, organization_id, name, channel, adapter_type,
	(credentials_enc IS NOT NULL), COALESCE(warehouse_id::text,''), is_active, created_at, updated_at`

// Create menyimpan store baru.
func (r *StoreRepo) Create(ctx context.Context, s *store.Store) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO stores (
			id, organization_id, name, channel, adapter_type, warehouse_id, is_active, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9)`,
		s.ID, s.OrganizationID, s.Name, string(s.Channel), s.AdapterType, s.WarehouseID,
		s.IsActive, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil && isForeignKeyOn(err, "stores_warehouse_id_fkey") {
		return store.ErrWarehouseNotFound
	}
	return err
}

func (r *StoreRepo) GetByID(ctx context.Context, orgID, id string) (*store.Store, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+storeCols+` FROM stores WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`,
		orgID, id)
	return scanStore(row)
}

// List mengembalikan store aktif sebuah organisasi.
func (r *StoreRepo) List(ctx context.Context, orgID string, onlyActive bool) ([]store.Store, error) {
	where := `organization_id=$1 AND deleted_at IS NULL`
	if onlyActive {
		where += ` AND is_active`
	}
	rows, err := r.q.Query(ctx, `SELECT `+storeCols+` FROM stores WHERE `+where+` ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.Store, 0)
	for rows.Next() {
		s, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Update mengubah nama, kanal, gudang default, dan status aktif.
func (r *StoreRepo) Update(ctx context.Context, s *store.Store) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE stores SET name=$3, channel=$4, adapter_type=$5,
			warehouse_id=NULLIF($6,'')::uuid, is_active=$7
		WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`,
		s.OrganizationID, s.ID, s.Name, string(s.Channel), s.AdapterType, s.WarehouseID, s.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanStore(row pgx.Row) (*store.Store, error) {
	var (
		s         store.Store
		channel   string
		createdAt time.Time
	)
	err := row.Scan(&s.ID, &s.OrganizationID, &s.Name, &channel, &s.AdapterType,
		&s.HasCredentials, &s.WarehouseID, &s.IsActive, &createdAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Channel = store.Channel(channel)
	s.CreatedAt = createdAt
	return &s, nil
}
