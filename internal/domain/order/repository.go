package order

import "context"

// ListFilter menyaring daftar order. Semua field opsional; nol berarti "tanpa filter".
type ListFilter struct {
	Status     Status
	StoreID    string
	CustomerID string
	Search     string // cocokkan order_number atau external_order_id
	From       string // RFC3339, opsional
	To         string // RFC3339, opsional
	Limit      int
	Offset     int
}

// Repository menyimpan order tenant-scoped. Semua metode WAJIB memfilter
// organization_id — tidak ada jalur yang mengembalikan baris organisasi lain.
type Repository interface {
	// Create menyimpan order beserta seluruh itemnya. Implementasi harus
	// menjalankannya dalam satu transaksi: order tanpa item adalah data rusak.
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, orgID, id string) (*Order, error)
	List(ctx context.Context, orgID string, f ListFilter) ([]Order, int, error)
	// UpdateStatus mengubah status dengan pemeriksaan optimistic: hanya
	// berpengaruh bila status saat ini sama dengan `from`.
	UpdateStatus(ctx context.Context, orgID, id string, from, to Status, reason string) error
	// NextOrderNumber mengembalikan nomor urut berikutnya untuk periode berjalan.
	NextOrderNumber(ctx context.Context, orgID string, year int) (int64, error)
}
