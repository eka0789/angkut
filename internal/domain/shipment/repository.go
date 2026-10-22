package shipment

import "context"

// ListFilter menyaring daftar shipment. Semua field opsional.
type ListFilter struct {
	Status      Status
	Courier     string // courier_code
	StoreID     string
	WarehouseID string
	OrderID     string
	Search      string // cocokkan awb (parsial) atau nama penerima
	From        string // RFC3339
	To          string // RFC3339
	Limit       int
	Offset      int
}

// Repository menyimpan shipment tenant-scoped. Semua metode baca WAJIB
// memfilter organization_id — tidak ada jalur yang mengembalikan baris
// organisasi lain.
//
// Satu-satunya pengecualian adalah NextAWBSequence: ia hanya membaca nilai
// maksimum kolom resi (angka), tidak pernah mengembalikan baris pengiriman,
// dan sengaja lintas organisasi agar dua organisasi tidak menerbitkan resi
// identik pada hari yang sama (resi unik global — migrasi 0020).
type Repository interface {
	// Create menyimpan shipment beserta item & paketnya dalam satu transaksi.
	Create(ctx context.Context, s *Shipment) error
	GetByID(ctx context.Context, orgID, id string) (*Shipment, error)
	// GetByIDs mengambil beberapa shipment milik organisasi sekaligus
	// (unduh label massal); id di luar organisasi dilewati — tidak error,
	// agar satu id asing tidak membongkar seluruh permintaan.
	GetByIDs(ctx context.Context, orgID string, ids []string) ([]Shipment, error)
	// GetByAWB mencari lewat resi (pencarian parsial trigram).
	GetByAWB(ctx context.Context, orgID, awb string) (*Shipment, error)
	// GetByIdempotencyKey mengembalikan shipment yang sudah dibuat dengan key
	// tertentu; ok=false berarti belum ada (dipakai jalur idempotent).
	GetByIdempotencyKey(ctx context.Context, orgID, key string) (*Shipment, bool, error)
	List(ctx context.Context, orgID string, f ListFilter) ([]Shipment, int, error)
	// UpdateStatus mengubah status dengan syarat status saat ini masih `from`.
	UpdateStatus(ctx context.Context, orgID, id string, from, to Status, failureReason string) error
	// UpdateLabel menyimpan URL label yang dihasilkan.
	UpdateLabel(ctx context.Context, orgID, id, labelURL string) error
	// NextAWBSequence mengembalikan nomor urut resi berikutnya untuk kode
	// kurir & hari tertentu, dihitung lintas organisasi (lihat catatan pada
	// Repository).
	NextAWBSequence(ctx context.Context, courierCode string, day string) (int64, error)
}
