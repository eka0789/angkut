// Package order berisi entitas order tenant-scoped dan state machine-nya.
// Order adalah niat jual; shipment (modul shipping) adalah eksekusinya.
// Isolasi tenant (ADR-008) ditegakkan di setiap query repository.
package order

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Status adalah tahap hidup sebuah order. Nilai HARUS sama dengan CHECK di
// migrasi 0015 — perubahan status wajib lewat CanTransitionTo, bukan set langsung.
type Status string

const (
	StatusDraft      Status = "draft"
	StatusConfirmed  Status = "confirmed"
	StatusProcessing Status = "processing"
	StatusShipped    Status = "shipped"
	StatusDelivered  Status = "delivered"
	StatusCancelled  Status = "cancelled"
	StatusReturned   Status = "returned"
)

// AllStatuses adalah urutan kanonik untuk iterasi & validasi (mis. filter list).
var AllStatuses = []Status{
	StatusDraft, StatusConfirmed, StatusProcessing,
	StatusShipped, StatusDelivered, StatusCancelled, StatusReturned,
}

// Valid melaporkan apakah status dikenal.
func (s Status) Valid() bool {
	for _, v := range AllStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// terminal adalah status yang tidak punya transisi keluar.
func (s Status) terminal() bool {
	return s == StatusCancelled || s == StatusReturned
}

// transitions memetakan status → status tujuan yang sah. Tabel eksplisit
// membuat aturan terlihat sekaligus mudah diuji; TIDAK ada lompatan status
// (mis. draft → delivered) yang diam-diam diperbolehkan.
var transitions = map[Status][]Status{
	StatusDraft:      {StatusConfirmed, StatusCancelled},
	StatusConfirmed:  {StatusProcessing, StatusCancelled},
	StatusProcessing: {StatusShipped, StatusCancelled},
	StatusShipped:    {StatusDelivered, StatusReturned},
	StatusDelivered:  {StatusReturned},
	StatusCancelled:  nil,
	StatusReturned:   nil,
}

// CanTransitionTo melaporkan apakah perpindahan dari s ke target diizinkan.
// Status sama dengan status saat ini BUKAN transisi (idempotent no-op ditangani
// pemanggil, bukan dianggap sah di sini).
func (s Status) CanTransitionTo(target Status) bool {
	if !target.Valid() {
		return false
	}
	for _, allowed := range transitions[s] {
		if allowed == target {
			return true
		}
	}
	return false
}

// AllowedTransitions mengembalikan salinan transisi yang sah dari s.
func (s Status) AllowedTransitions() []Status {
	src := transitions[s]
	out := make([]Status, len(src))
	copy(out, src)
	return out
}

// PaymentType menentukan apakah order dibayar di muka atau saat terima (COD).
type PaymentType string

const (
	PaymentPrepaid PaymentType = "prepaid"
	PaymentCOD     PaymentType = "cod"
)

// Valid melaporkan apakah tipe pembayaran dikenal.
func (p PaymentType) Valid() bool {
	return p == PaymentPrepaid || p == PaymentCOD
}

// Item adalah satu baris order. Nama produk DISNAPSHOT saat order dibuat agar
// histori tidak berubah ketika katalog (Phase 4) berubah.
type Item struct {
	ID                  string    `json:"id"`
	OrganizationID      string    `json:"organization_id"`
	OrderID             string    `json:"order_id"`
	VariantID           string    `json:"variant_id,omitempty"`
	ProductNameSnapshot string    `json:"product_name_snapshot"`
	VariantNameSnapshot string    `json:"variant_name_snapshot,omitempty"`
	Qty                 int       `json:"qty"`
	UnitPrice           int64     `json:"unit_price"`
	TotalPrice          int64     `json:"total_price"`
	CreatedAt           time.Time `json:"created_at"`
}

// Validate memeriksa invariant satu item.
func (i *Item) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(i.ProductNameSnapshot) == "" {
		fields["product_name_snapshot"] = "Nama produk wajib diisi."
	}
	if i.Qty <= 0 {
		fields["qty"] = "Jumlah harus lebih dari 0."
	}
	if i.UnitPrice < 0 {
		fields["unit_price"] = "Harga satuan tidak boleh negatif."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Order adalah order pelanggan.
type Order struct {
	ID                string      `json:"id"`
	OrganizationID    string      `json:"organization_id"`
	StoreID           string      `json:"store_id,omitempty"`
	CustomerID        string      `json:"customer_id"`
	OrderNumber       string      `json:"order_number"`
	ExternalOrderID   string      `json:"external_order_id,omitempty"`
	Status            Status      `json:"status"`
	PaymentType       PaymentType `json:"payment_type"`
	Subtotal          int64       `json:"subtotal"`
	ShippingCost      int64       `json:"shipping_cost"`
	Discount          int64       `json:"discount"`
	TotalAmount       int64       `json:"total_amount"`
	ShippingAddressID string      `json:"shipping_address_id,omitempty"`
	Notes             string      `json:"notes,omitempty"`
	CancelledReason   string      `json:"cancelled_reason,omitempty"`
	Items             []Item      `json:"items"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

// Validate memeriksa invariant order LENGKAP (termasuk nomor order). Dipakai
// sebelum persist final — nomor order diterbitkan server di dalam transaksi,
// jadi pemanggil yang belum punya nomor memakai ValidateInput.
func (o *Order) Validate() error {
	if err := o.ValidateInput(); err != nil {
		return err
	}
	if strings.TrimSpace(o.OrderNumber) == "" {
		return shared.Validation(map[string]string{
			"order_number": "Nomor order wajib diisi.",
		})
	}
	return nil
}

// ValidateInput memeriksa invariant yang HARUS dipenuhi klien — tanpa nomor
// order (diterbitkan server) dan tanpa menyentuh field turunan.
func (o *Order) ValidateInput() error {
	fields := make(map[string]string)

	if strings.TrimSpace(o.CustomerID) == "" {
		fields["customer_id"] = "Pelanggan wajib diisi."
	}
	if !o.Status.Valid() {
		fields["status"] = "Status order tidak dikenal."
	}
	if !o.PaymentType.Valid() {
		fields["payment_type"] = "Tipe pembayaran tidak dikenal."
	}
	if len(o.Items) == 0 {
		fields["items"] = "Order harus punya minimal satu item."
	}
	// Total dihitung di aplikasi dari item — pemanggil tidak boleh mengarang.
	if o.Subtotal < 0 || o.Discount < 0 || o.ShippingCost < 0 {
		fields["amounts"] = "Nilai uang tidak boleh negatif."
	}
	if o.Discount > o.Subtotal+o.ShippingCost {
		fields["discount"] = "Diskon tidak boleh melebihi subtotal + ongkir."
	}

	for idx := range o.Items {
		if err := o.Items[idx].Validate(); err != nil {
			fields[fmt.Sprintf("items[%d]", idx)] = err.Error()
		}
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Recalculate menghitung ulang subtotal, total, dan total tiap item dari
// harga satuan × qty. Satu sumber kebenaran: klien mengirim harga, server
// yang menjumlahkan.
func (o *Order) Recalculate() {
	var subtotal int64
	for idx := range o.Items {
		it := &o.Items[idx]
		it.TotalPrice = it.UnitPrice * int64(it.Qty)
		subtotal += it.TotalPrice
	}
	o.Subtotal = subtotal
	total := subtotal - o.Discount + o.ShippingCost
	if total < 0 {
		total = 0
	}
	o.TotalAmount = total
}

// Transition memindahkan status order ke target bila sah. Mengembalikan
// shared.Conflict bila transisi tidak diizinkan sehingga aturan state machine
// tidak bisa dilewati dengan menulis status langsung.
func (o *Order) Transition(target Status) error {
	if !target.Valid() {
		return shared.Validation(map[string]string{
			"status": "Status order tidak dikenal.",
		})
	}
	if o.Status == target {
		// Idempotent: meminta status yang sama bukan kesalahan.
		return nil
	}
	if !o.Status.CanTransitionTo(target) {
		return shared.Conflict("ORDER_INVALID_TRANSITION", fmt.Sprintf(
			"Order berstatus %s tidak dapat berpindah ke %s.", o.Status, target))
	}
	o.Status = target
	o.UpdatedAt = time.Now().UTC()
	return nil
}

// ─── format nomor order ──────────────────────────────────────────────────

// orderNumberPrefix menandai nomor order internal Angkut.
const orderNumberPrefix = "ORD"

// orderNumberSeqWidth adalah jumlah digit urutan (nol di depan).
const orderNumberSeqWidth = 6

// FormatOrderNumber menyusun nomor order internal: ORD-<tahun>-<urut 6 digit>.
// Format ini terlihat pelanggan, sehingga perubahan bentuknya adalah perubahan
// kontrak — dipakai bersama FE dan dokumen.
func FormatOrderNumber(year int, seq int64) string {
	return fmt.Sprintf("%s-%d-%0*d", orderNumberPrefix, year, orderNumberSeqWidth, seq)
}

// OrderNumberPrefix mengembalikan awalan nomor order satu tahun ("ORD-2026-"),
// dipakai saat mencari nomor urut terbesar yang sudah ada.
func OrderNumberPrefix(year int) string {
	return fmt.Sprintf("%s-%d-", orderNumberPrefix, year)
}

// ParseOrderNumberSeq mengambil nomor urut dari sebuah nomor order. Ok=false
// bila bentuknya tidak dikenali — pemanggil tidak boleh menebak.
func ParseOrderNumberSeq(year int, orderNumber string) (int64, bool) {
	prefix := OrderNumberPrefix(year)
	if !strings.HasPrefix(orderNumber, prefix) {
		return 0, false
	}
	seq, err := strconv.ParseInt(strings.TrimPrefix(orderNumber, prefix), 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

// ─── sentinel error ──────────────────────────────────────────────────────

var (
	ErrNotFound         = shared.NotFound("ORDER_NOT_FOUND", "Order tidak ditemukan.")
	ErrCustomerNotFound = shared.NotFound("ORDER_CUSTOMER_NOT_FOUND", "Pelanggan untuk order ini tidak ditemukan.")
	ErrStoreNotFound    = shared.NotFound("ORDER_STORE_NOT_FOUND", "Toko untuk order ini tidak ditemukan.")
	ErrAddressNotFound  = shared.NotFound("ORDER_ADDRESS_NOT_FOUND", "Alamat pengiriman tidak ditemukan.")
	ErrNumberTaken      = shared.Conflict("ORDER_NUMBER_TAKEN", "Nomor order sudah dipakai.")
	ErrNotCancellable   = shared.Conflict("ORDER_NOT_CANCELLABLE", "Order pada status ini tidak dapat dibatalkan.")
	ErrNotShippable     = shared.Conflict("ORDER_NOT_SHIPPABLE", "Order hanya dapat dikirim setelah dikonfirmasi.")
)
