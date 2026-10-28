package pickup

import (
	"context"
	"time"
)

// ListFilter menyaring daftar pickup request.
type ListFilter struct {
	Status      Status
	WarehouseID string
	Limit       int
	Offset      int
}

// Repository menyimpan pickup request tenant-scoped.
type Repository interface {
	Create(ctx context.Context, r *Request) error
	GetByID(ctx context.Context, orgID, id string) (*Request, error)
	List(ctx context.Context, orgID string, f ListFilter) ([]Request, int, error)
	// UpdateStatus memindahkan status dengan syarat status saat ini masih `from`.
	UpdateStatus(ctx context.Context, orgID, id string, from, to Status) error
	// Schedule menyimpan jadwal (scheduled_at) sekaligus memindahkan status
	// requested → scheduled dalam satu UPDATE.
	Schedule(ctx context.Context, orgID, id string, at time.Time) error
	// CancelFrom membatalkan pickup dari status non-terminal mana pun.
	// Mengembalikan Conflict bila statusnya sudah terminal — bukan NotFound,
	// karena barisnya jelas ada.
	CancelFrom(ctx context.Context, orgID, id string) error
	// AddShipments melampirkan pengiriman MILIK ORGANISASI yang sama. ID yang
	// tidak ada atau milik tenant lain ditolak, tidak diabaikan diam-diam.
	AddShipments(ctx context.Context, orgID, pickupID string, shipmentIDs []string) error
	ListShipments(ctx context.Context, pickupID string) ([]string, error)
}
