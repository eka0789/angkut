// Package inventory berisi model & aturan stok (TASK-037/038) yang murni:
// produk, varian SKU, saldo per gudang, movement append-only, transfer antar
// gudang, dan aturan low-stock.
//
// Invariant yang dijaga di sini dan dipaksa lagi di DB:
//   - saldo tidak boleh negatif, reserved ≤ on_hand;
//   - transfer butuh dua gudang berbeda dan qty > 0;
//   - available = on_hand − reserved adalah satu-satunya angka yang boleh
//     dijanjikan ke pengguna (bukan on_hand mentah).
package inventory

import (
	"fmt"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Product adalah katalog produk milik satu organisasi.
type Product struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Category       string    `json:"category,omitempty"`
	WeightKG       *float64  `json:"weight_kg,omitempty"`
	LengthCM       *float64  `json:"length_cm,omitempty"`
	WidthCM        *float64  `json:"width_cm,omitempty"`
	HeightCM       *float64  `json:"height_cm,omitempty"`
	IsActive       bool      `json:"is_active"`
	CreatedBy      string    `json:"created_by,omitempty"`
	Variants       []Variant `json:"variants,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate memeriksa invariant produk sebelum disimpan.
func (p *Product) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(p.Name) == "" {
		fields["name"] = "Nama produk wajib diisi."
	}
	for _, d := range []struct {
		name string
		v    *float64
	}{
		{"weight_kg", p.WeightKG}, {"length_cm", p.LengthCM},
		{"width_cm", p.WidthCM}, {"height_cm", p.HeightCM},
	} {
		if d.v != nil && *d.v < 0 {
			fields[d.name] = "Dimensi/berat tidak boleh negatif."
		}
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Variant adalah SKU yang dikelola stoknya.
type Variant struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organization_id"`
	ProductID         string    `json:"product_id"`
	SKU               string    `json:"sku"`
	Name              string    `json:"name,omitempty"`
	WeightKG          *float64  `json:"weight_kg,omitempty"`
	LengthCM          *float64  `json:"length_cm,omitempty"`
	WidthCM           *float64  `json:"width_cm,omitempty"`
	HeightCM          *float64  `json:"height_cm,omitempty"`
	LowStockThreshold int       `json:"low_stock_threshold"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Validate memeriksa invariant varian.
func (v *Variant) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(v.SKU) == "" {
		fields["sku"] = "SKU wajib diisi."
	} else if len(v.SKU) > 64 {
		fields["sku"] = "SKU maksimal 64 karakter."
	}
	if v.LowStockThreshold < 0 {
		fields["low_stock_threshold"] = "Ambang low-stock tidak boleh negatif."
	}
	for _, d := range []struct {
		name string
		val  *float64
	}{
		{"weight_kg", v.WeightKG}, {"length_cm", v.LengthCM},
		{"width_cm", v.WidthCM}, {"height_cm", v.HeightCM},
	} {
		if d.val != nil && *d.val < 0 {
			fields[d.name] = "Dimensi/berat tidak boleh negatif."
		}
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Stock adalah saldo satu varian di satu gudang.
type Stock struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	VariantID      string `json:"variant_id"`
	WarehouseID    string `json:"warehouse_id"`
	QtyOnHand      int    `json:"qty_on_hand"`
	QtyReserved    int    `json:"qty_reserved"`
	Version        int    `json:"version"`
}

// Available mengembalikan stok yang benar-benar bisa dipakai/dikirim.
func (s Stock) Available() int { return s.QtyOnHand - s.QtyReserved }

// MovementType adalah jenis perubahan stok. Append-only: koreksi = movement
// adjust baru, bukan pengeditan riwayat.
type MovementType string

const (
	MovementAdjust      MovementType = "adjust"
	MovementTransferOut MovementType = "transfer_out"
	MovementTransferIn  MovementType = "transfer_in"
	MovementReserve     MovementType = "reserve"
	MovementRelease     MovementType = "release"
)

// Valid melaporkan apakah jenis movement dikenal.
func (m MovementType) Valid() bool {
	switch m {
	case MovementAdjust, MovementTransferOut, MovementTransferIn, MovementReserve, MovementRelease:
		return true
	}
	return false
}

// Movement adalah satu catatan perubahan stok.
type Movement struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	VariantID      string       `json:"variant_id"`
	WarehouseID    string       `json:"warehouse_id"`
	Type           MovementType `json:"type"`
	// QtyDelta bertanda: negatif mengurangi on_hand (out/adjust turun),
	// positif menambah.
	QtyDelta   int       `json:"qty_delta"`
	QtyAfter   int       `json:"qty_after"`
	Reason     string    `json:"reason,omitempty"`
	TransferID string    `json:"transfer_id,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// StockTransfer adalah transfer stok antar gudang (TASK-038). Status selalu
// completed saat dibuat — transfer yang tidak bisa dilakukan atomik ditolak
// seluruhnya, bukan setengah jalan.
type StockTransfer struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	FromWarehouse  string         `json:"from_warehouse_id"`
	ToWarehouse    string         `json:"to_warehouse_id"`
	Notes          string         `json:"notes,omitempty"`
	CreatedBy      string         `json:"created_by,omitempty"`
	Items          []TransferItem `json:"items"`
	CreatedAt      time.Time      `json:"created_at"`
}

// TransferItem adalah satu varian yang dipindahkan.
type TransferItem struct {
	VariantID string `json:"variant_id"`
	Qty       int    `json:"qty"`
}

// TransferInput adalah permintaan transfer untuk divalidasi.
type TransferInput struct {
	OrganizationID string
	FromWarehouse  string
	ToWarehouse    string
	Notes          string
	Items          []TransferItem
}

// ValidateTransfer memeriksa aturan transfer sebelum menyentuh saldo.
func ValidateTransfer(in TransferInput) error {
	fields := make(map[string]string)
	if strings.TrimSpace(in.FromWarehouse) == "" {
		fields["from"] = "Gudang asal wajib dipilih."
	}
	if strings.TrimSpace(in.ToWarehouse) == "" {
		fields["to"] = "Gudang tujuan wajib dipilih."
	}
	if in.FromWarehouse != "" && in.FromWarehouse == in.ToWarehouse {
		fields["to"] = "Gudang asal dan tujuan harus berbeda."
	}
	if len(in.Items) == 0 {
		fields["items"] = "Minimal satu varian untuk dipindahkan."
	}
	seen := make(map[string]bool, len(in.Items))
	for i, it := range in.Items {
		if strings.TrimSpace(it.VariantID) == "" {
			fields[fmt.Sprintf("items.%d.variant_id", i)] = "Varian wajib dipilih."
		}
		if seen[it.VariantID] {
			fields[fmt.Sprintf("items.%d.variant_id", i)] = "Varian duplikat pada daftar transfer."
		}
		seen[it.VariantID] = true
		if it.Qty <= 0 {
			fields[fmt.Sprintf("items.%d.qty", i)] = "Jumlah harus lebih dari 0."
		}
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// LowStockRow adalah satu baris laporan low-stock.
type LowStockRow struct {
	VariantID         string `json:"variant_id"`
	SKU               string `json:"sku"`
	ProductName       string `json:"product_name"`
	WarehouseID       string `json:"warehouse_id"`
	QtyAvailable      int    `json:"qty_available"`
	LowStockThreshold int    `json:"low_stock_threshold"`
}

// IsLow melaporkan apakah baris ini low-stock (available ≤ threshold).
func (r LowStockRow) IsLow() bool { return r.QtyAvailable <= r.LowStockThreshold }

// Sentinel error modul inventory.
var (
	ErrNotFound = shared.NotFound("INVENTORY_NOT_FOUND", "Data inventory tidak ditemukan.")
	// ErrSKUTaken dilempar saat SKU sudah dipakai varian lain di organisasi
	// ini (unique index per org). Conflict agar pemanggil paham ini bentrok
	// kepemilikan, bukan salah bentuk input.
	ErrSKUTaken = shared.New("PRODUCT_SKU_TAKEN",
		"SKU sudah dipakai varian lain di organisasi ini.", shared.ErrConflict)
)
