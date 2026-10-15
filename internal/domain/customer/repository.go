package customer

import (
	"context"
)

// Repository menyimpan pelanggan tenant-scoped. Semua metode wajib memfilter
// organization_id — tidak ada jalur yang mengembalikan baris org lain.
type Repository interface {
	Create(ctx context.Context, c *Customer) error
	GetByID(ctx context.Context, orgID, id string) (*Customer, error)
	GetByPhone(ctx context.Context, orgID, phone string) (*Customer, error)
	Update(ctx context.Context, c *Customer) error
	// Delete soft-delete (set deleted_at); mengembalikan ErrNotFound bila
	// baris tidak ada atau sudah terhapus.
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, segment Segment, query string, limit, offset int) ([]Customer, int, error)
}
