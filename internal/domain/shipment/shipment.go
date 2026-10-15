// Package shipment berisi entitas pengiriman tenant-scoped, state machine
// statusnya, dan pembentukan AWB. Shipment adalah eksekusi fisik sebuah order.
package shipment

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Status adalah tahap hidup sebuah pengiriman. Nilai HARUS sama dengan CHECK
// di migrasi 0016.
type Status string

const (
	StatusDraft          Status = "draft"
	StatusPendingPickup  Status = "pending_pickup"
	StatusPickedUp       Status = "picked_up"
	StatusInTransit      Status = "in_transit"
	StatusOutForDelivery Status = "out_for_delivery"
	StatusDelivered      Status = "delivered"
	StatusFailed         Status = "failed"
	StatusReturned       Status = "returned"
	StatusCancelled      Status = "cancelled"
)

// AllStatuses adalah urutan kanonik untuk validasi & filter.
var AllStatuses = []Status{
	StatusDraft, StatusPendingPickup, StatusPickedUp, StatusInTransit,
	StatusOutForDelivery, StatusDelivered, StatusFailed, StatusReturned, StatusCancelled,
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

// IsTerminal melaporkan apakah status mengakhiri siklus hidup shipment.
func (s Status) IsTerminal() bool {
	return s == StatusDelivered || s == StatusReturned || s == StatusCancelled
}

// shipmentTransitions adalah state machine pengiriman. Berbeda dari order yang
// linier, pengiriman nyata punya cabang: gagal antar bisa dicoba ulang, dan
// paket bisa dikembalikan. Tabel eksplisit membuat aturan mudah diaudit.
var shipmentTransitions = map[Status][]Status{
	StatusDraft:          {StatusPendingPickup, StatusCancelled},
	StatusPendingPickup:  {StatusPickedUp, StatusCancelled},
	StatusPickedUp:       {StatusInTransit, StatusFailed, StatusCancelled},
	StatusInTransit:      {StatusOutForDelivery, StatusFailed, StatusReturned},
	StatusOutForDelivery: {StatusDelivered, StatusFailed, StatusReturned},
	// Gagal bisa dicoba ulang (kembali ke antrean) atau dikembalikan ke pengirim.
	StatusFailed:    {StatusPendingPickup, StatusReturned, StatusCancelled},
	StatusDelivered: nil,
	StatusReturned:  nil,
	StatusCancelled: nil,
}

// CanTransitionTo melaporkan apakah perpindahan status diizinkan.
func (s Status) CanTransitionTo(target Status) bool {
	if !target.Valid() {
		return false
	}
	for _, allowed := range shipmentTransitions[s] {
		if allowed == target {
			return true
		}
	}
	return false
}

// AllowedTransitions mengembalikan salinan transisi sah dari s.
func (s Status) AllowedTransitions() []Status {
	src := shipmentTransitions[s]
	out := make([]Status, len(src))
	copy(out, src)
	return out
}

// Package adalah satu paket fisik dalam sebuah shipment. Satu shipment bisa
// berisi beberapa paket (mis. pesanan besar dipecah).
type Package struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	ShipmentID     string   `json:"shipment_id"`
	Sequence       int      `json:"sequence"`
	Weight         float64  `json:"weight"`
	Length         *float64 `json:"length,omitempty"`
	Width          *float64 `json:"width,omitempty"`
	Height         *float64 `json:"height,omitempty"`
	AWB            string   `json:"awb,omitempty"`
	QRPayload      string   `json:"qr_payload"`
	BarcodeValue   string   `json:"barcode_value"`
}

// Item adalah isi paket yang disnapshot saat booking.
type Item struct {
	ID           string `json:"id"`
	OrderItemID  string `json:"order_item_id,omitempty"`
	VariantID    string `json:"variant_id,omitempty"`
	NameSnapshot string `json:"name_snapshot"`
	Qty          int    `json:"qty"`
}

// Shipment adalah satu pengiriman fisik.
type Shipment struct {
	ID                   string `json:"id"`
	OrganizationID       string `json:"organization_id"`
	OrderID              string `json:"order_id,omitempty"`
	StoreID              string `json:"store_id,omitempty"`
	WarehouseID          string `json:"warehouse_id,omitempty"`
	CourierID            string `json:"courier_id,omitempty"`
	CourierServiceID     string `json:"courier_service_id,omitempty"`
	CourierCode          string `json:"courier_code"`
	ServiceCode          string `json:"service_code"`
	AWB                  string `json:"awb"`
	Status               Status `json:"status"`
	OriginAddressID      string `json:"origin_address_id,omitempty"`
	DestinationAddressID string `json:"destination_address_id,omitempty"`

	ActualWeight     float64  `json:"actual_weight"`
	VolumetricWeight float64  `json:"volumetric_weight"`
	ChargeableWeight float64  `json:"chargeable_weight"`
	Length           *float64 `json:"length,omitempty"`
	Width            *float64 `json:"width,omitempty"`
	Height           *float64 `json:"height,omitempty"`
	PackageType      string   `json:"package_type,omitempty"`

	DeclaredValue   int64 `json:"declared_value"`
	InsuranceAmount int64 `json:"insurance_amount"`
	ShippingPrice   int64 `json:"shipping_price"`
	CostPrice       int64 `json:"cost_price"`

	IsCOD     bool  `json:"is_cod"`
	CODAmount int64 `json:"cod_amount"`
	CODFee    int64 `json:"cod_fee"`

	EtaMinDays int `json:"eta_min_days"`
	EtaMaxDays int `json:"eta_max_days"`

	Notes          string     `json:"notes,omitempty"`
	FailureReason  string     `json:"failure_reason,omitempty"`
	BookedAt       *time.Time `json:"booked_at,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	Version        int        `json:"version"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	LabelURL       string     `json:"label_url,omitempty"`

	Items    []Item    `json:"items"`
	Packages []Package `json:"packages"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate memeriksa invariant shipment LENGKAP termasuk nomor resi. Dipakai
// tepat sebelum persist — resi diterbitkan server di dalam transaksi.
func (s *Shipment) Validate() error {
	if err := s.ValidateInput(); err != nil {
		return err
	}
	if strings.TrimSpace(s.AWB) == "" {
		return shared.Validation(map[string]string{
			"awb": "Nomor resi wajib diisi.",
		})
	}
	return nil
}

// ValidateInput memeriksa invariant yang tidak bergantung pada nomor resi —
// dipakai sebelum resi diterbitkan sehingga kegagalan input dilaporkan lebih
// awal dengan pesan yang tepat.
func (s *Shipment) ValidateInput() error {
	fields := make(map[string]string)

	if strings.TrimSpace(s.CourierCode) == "" {
		fields["courier_code"] = "Kurir wajib diisi."
	}
	if strings.TrimSpace(s.ServiceCode) == "" {
		fields["service_code"] = "Layanan kurir wajib diisi."
	}
	if !s.Status.Valid() {
		fields["status"] = "Status pengiriman tidak dikenal."
	}
	if s.ActualWeight <= 0 {
		fields["weight"] = "Berat harus lebih dari 0."
	}
	if s.ChargeableWeight <= 0 {
		fields["chargeable_weight"] = "Berat chargeable harus lebih dari 0."
	}
	if s.ShippingPrice < 0 || s.CostPrice < 0 {
		fields["price"] = "Harga tidak boleh negatif."
	}
	// COD tanpa nilai membuat kurir menagih 0 — hampir pasti salah input.
	if s.IsCOD && s.CODAmount <= 0 {
		fields["cod_amount"] = "Nilai COD wajib diisi bila pengiriman COD."
	}
	if !s.IsCOD && s.CODAmount > 0 {
		fields["cod_amount"] = "Nilai COD harus 0 bila bukan pengiriman COD."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Transition memindahkan status shipment bila sah. Status sama = no-op.
func (s *Shipment) Transition(target Status) error {
	if !target.Valid() {
		return shared.Validation(map[string]string{
			"status": "Status pengiriman tidak dikenal.",
		})
	}
	if s.Status == target {
		return nil
	}
	if !s.Status.CanTransitionTo(target) {
		return shared.Conflict("SHIPMENT_INVALID_TRANSITION", fmt.Sprintf(
			"Pengiriman berstatus %s tidak dapat berpindah ke %s.", s.Status, target))
	}
	s.Status = target
	now := time.Now().UTC()
	s.UpdatedAt = now
	switch target {
	case StatusPendingPickup:
		if s.BookedAt == nil {
			s.BookedAt = &now
		}
	case StatusDelivered:
		s.DeliveredAt = &now
	}
	return nil
}

// ─── AWB & payload label ─────────────────────────────────────────────────

// AWB adalah nomor resi. Format internal: <KODE_KURIR><YYMMDD><URUT 6 digit>,
// mis. JNE260101000042. Adapter kurir sungguhan (Phase 4) dapat menggantinya
// dengan resi asli dari API kurir — kolomnya tetap satu (awb).
type AWB struct {
	CourierCode string
	Year        int
	Month       time.Month
	Day         int
	Seq         int64
}

// String menyusun nomor resi kanonik.
func (a AWB) String() string {
	code := strings.ToUpper(strings.TrimSpace(a.CourierCode))
	return fmt.Sprintf("%s%02d%02d%02d%0*d",
		code, a.Year%100, int(a.Month), a.Day, awbSeqWidth, a.Seq)
}

// awbSeqWidth adalah jumlah digit urutan pada AWB.
const awbSeqWidth = 6

// AWBLookupPrefix mengembalikan awalan resi untuk satu kurir & hari
// ("JNE260101"), dipakai saat mencari nomor urut terbesar yang sudah ada.
func AWBLookupPrefix(courierCode, day string) string {
	return strings.ToUpper(strings.TrimSpace(courierCode)) + day
}

// ParseAWBSeq mengambil nomor urut dari sebuah resi. Ok=false bila bentuknya
// tidak dikenali — pemanggil tidak boleh menebak.
func ParseAWBSeq(prefix, awb string) (int64, bool) {
	if !strings.HasPrefix(awb, prefix) {
		return 0, false
	}
	seq, err := strconv.ParseInt(strings.TrimPrefix(awb, prefix), 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

// qrPrefix menandai payload QR label Angkut.
const qrPrefix = "ANGKUT"

// QRPayload menyusun isi QR code label: ANGKUT|<shipment_id>|<awb>. Bentuk ini
// diurai aplikasi pemindai; perubahan bentuknya adalah perubahan kontrak.
func QRPayload(shipmentID, awb string) string {
	return qrPrefix + "|" + shipmentID + "|" + awb
}

// BarcodeValue menyusun nilai barcode 1D label. Memakai AWB apa adanya karena
// AWB sudah alfanumerik tanpa spasi — cocok untuk Code128.
func BarcodeValue(awb string) string {
	return strings.ToUpper(strings.TrimSpace(awb))
}

// ─── sentinel error ──────────────────────────────────────────────────────

var (
	ErrNotFound          = shared.NotFound("SHIPMENT_NOT_FOUND", "Pengiriman tidak ditemukan.")
	ErrOrderNotFound     = shared.NotFound("SHIPMENT_ORDER_NOT_FOUND", "Order untuk pengiriman ini tidak ditemukan.")
	ErrCourierNotFound   = shared.NotFound("SHIPMENT_COURIER_NOT_FOUND", "Kurir atau layanan tidak ditemukan.")
	ErrAddressNotFound   = shared.NotFound("SHIPMENT_ADDRESS_NOT_FOUND", "Alamat asal atau tujuan tidak ditemukan.")
	ErrWarehouseNotFound = shared.NotFound("SHIPMENT_WAREHOUSE_NOT_FOUND", "Gudang asal tidak ditemukan.")
	ErrAWBTaken          = shared.Conflict("SHIPMENT_AWB_TAKEN", "Nomor resi sudah dipakai.")
	ErrNotCancellable    = shared.Conflict("SHIPMENT_NOT_CANCELLABLE", "Pengiriman pada status ini tidak dapat dibatalkan.")
)
