// Package customer berisi entitas CRM yang terikat tenant (organization_id).
// Prinsip isolasi tenant ADR-008 wajib ditegakkan di setiap query.
package customer

import (
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Customer adalah kontak pelanggan tenant: profil + segmen + metrik agregat.
type Customer struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organization_id"`
	Name             string    `json:"name"`
	Phone            string    `json:"phone"`
	Email            string    `json:"email,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	DefaultAddressID string    `json:"default_address_id,omitempty"`
	Segment          Segment   `json:"segment"`
	SegmentSource    string    `json:"segment_source"` // auto | manual
	Metrics          Metrics   `json:"metrics"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Segment kategori pelanggan. Nilai HARUS sama dengan CHECK di migrasi 0011.
type Segment string

const (
	SegmentNew      Segment = "new"
	SegmentRegular  Segment = "regular"
	SegmentVIP      Segment = "vip"
	SegmentHighRisk Segment = "high_risk"
	SegmentCODRisk  Segment = "cod_risk"
)

// Segmen valid.
var AllSegments = []Segment{SegmentNew, SegmentRegular, SegmentVIP, SegmentHighRisk, SegmentCODRisk}

// Valid melaporkan apakah kode segmen dikenal.
func (s Segment) Valid() bool {
	for _, v := range AllSegments {
		if s == v {
			return true
		}
	}
	return false
}

// Metrics adalah metrik agregat pelanggan. Sumber kebenaran dihitung dari data
// order/shipment historis (modul Order, TASK-026); struktur ini format simpan.
type Metrics struct {
	OrdersCount      int      `json:"orders_count"`
	TotalSpend       float64  `json:"total_spend"`
	AOV              float64  `json:"aov"`
	ReturnRate       float64  `json:"return_rate"`
	CODSuccessRate   *float64 `json:"cod_success_rate,omitempty"`
	PreferredCourier string   `json:"preferred_courier,omitempty"`
}

// Segmentasi otomatis: rules pertama yang cocok menang. Urutan dari yang paling
// spesifik (risiko) ke yang paling umum. Ambang sengaja konstanta bernama —
// tanpa magic number (TASK-023 aturan).
const (
	// VIP: pelanggan setia — spend total tinggi ATAU order banyak.
	VIPTotalSpend  = 50_000_000 // IDR 50 juta
	VIPOrdersCount = 25
	// Regular: sudah pernah bertransaksi cukup sering.
	RegularOrdersCount = 3
	// HighRisk: banyak retur.
	HighRiskReturnRate = 0.3
	// CODRisk: pembayaran COD sering gagal.
	CODRiskFailureRate = 0.3
)

// AutoSegment menghitung segmen dari metrik. Pelanggan tanpa riwayat → new.
func AutoSegment(m Metrics) Segment {
	switch {
	case m.ReturnRate >= HighRiskReturnRate:
		return SegmentHighRisk
	case m.CODSuccessRate != nil && *m.CODSuccessRate <= 1-CODRiskFailureRate && m.OrdersCount >= RegularOrdersCount:
		return SegmentCODRisk
	case m.TotalSpend >= VIPTotalSpend || m.OrdersCount >= VIPOrdersCount:
		return SegmentVIP
	case m.OrdersCount >= RegularOrdersCount:
		return SegmentRegular
	default:
		return SegmentNew
	}
}

// Validate memeriksa invariant dasar sebelum disimpan ke DB.
func (c *Customer) Validate() error {
	fields := make(map[string]string)

	if strings.TrimSpace(c.Name) == "" {
		fields["name"] = "Nama pelanggan wajib diisi."
	}
	if strings.TrimSpace(c.Phone) == "" {
		fields["phone"] = "Nomor telepon wajib diisi."
	}
	if c.Email != "" && !strings.Contains(c.Email, "@") {
		fields["email"] = "Format email tidak valid."
	}
	if len(c.Name) > 200 {
		fields["name"] = "Nama maksimal 200 karakter."
	}

	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Stats adalah ringkasan metrik yang dikembalikan endpoint /stats.
type Stats struct {
	OrdersCount      int      `json:"orders_count"`
	TotalSpend       float64  `json:"total_spend"`
	AOV              float64  `json:"aov"`
	ReturnRate       float64  `json:"return_rate"`
	CODSuccessRate   *float64 `json:"cod_success_rate,omitempty"`
	PreferredCourier string   `json:"preferred_courier,omitempty"`
}

// FromMetrics mengubah Metrics menjadi Stats (bentuk respons API).
func FromMetrics(m Metrics) Stats {
	return Stats{
		OrdersCount:      m.OrdersCount,
		TotalSpend:       m.TotalSpend,
		AOV:              m.AOV,
		ReturnRate:       m.ReturnRate,
		CODSuccessRate:   m.CODSuccessRate,
		PreferredCourier: m.PreferredCourier,
	}
}

// Sentinel errors
var (
	ErrNotFound     = shared.NotFound("CUSTOMER_NOT_FOUND", "Pelanggan tidak ditemukan.")
	ErrPhoneTaken   = shared.Conflict("CUSTOMER_PHONE_TAKEN", "Nomor telepon sudah dipakai pelanggan lain di organisasi ini.")
	ErrManualLocked = shared.Conflict("SEGMENT_MANUAL", "Segmen diset manual; hapus override terlebih dahulu agar dihitung otomatis.")
)
