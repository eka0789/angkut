package inventory

import "context"

// ListFilter menyaring daftar.
type ProductFilter struct {
	Search   string // cocokkan nama produk / SKU varian
	Category string
	// LowStock=true hanya produk yang punya varian low-stock.
	LowStock bool
	Limit    int
	Offset   int
}

type StockFilter struct {
	WarehouseID string
	VariantID   string
	Search      string // nama produk / SKU
	Limit       int
	Offset      int
}

type MovementFilter struct {
	VariantID   string
	WarehouseID string
	Type        MovementType
	Limit       int
	Offset      int
}

// StockRow adalah saldo plus keterangan varian/produk untuk tampilan.
type StockRow struct {
	Stock
	SKU         string `json:"sku"`
	ProductName string `json:"product_name"`
	VariantName string `json:"variant_name,omitempty"`
	Threshold   int    `json:"low_stock_threshold"`
}

// Repository menyimpan katalog produk & varian, tenant-scoped. Implementasi
// konkret ada di infrastructure/database; metode yang menyentuh saldo WAJIB
// dipanggil lewat Stores.WithTx agar ikut transaksi pemanggil.
type Repository interface {
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, orgID, id string) (*Product, error)
	List(ctx context.Context, orgID string, f ProductFilter) ([]Product, int, error)
	Update(ctx context.Context, p *Product) error
	SetActive(ctx context.Context, orgID, id string, active bool) error
	// AddVariant menambah SKU; bentrok SKU unik → ErrSKUTaken.
	AddVariant(ctx context.Context, v *Variant) error
	// UpdateVariant mengubah data varian (bukan saldo).
	UpdateVariant(ctx context.Context, v *Variant) error
	// VariantExists memeriksa varian milik organisasi (validasi input transfer).
	VariantExists(ctx context.Context, orgID, variantID string) (bool, error)
}
