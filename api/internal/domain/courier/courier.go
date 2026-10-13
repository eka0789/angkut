// Package courier berisi registry kurir, layanan, tarif, dan zona.
// Kurir bersifat platform-level (bukan tenant), tarif di-query per pasangan
// zona. Prinsip provider-agnostic ADR-002: logika bisnis hanya melihat
// interface Provider, adapter sungguhan/mock disambung di registry.
package courier

import (
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Courier adalah kurir terdaftar di platform.
type Courier struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	AdapterType string    `json:"adapter_type"`
	IsActive    bool      `json:"is_active"`
	LogoURL     string    `json:"logo_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Service adalah layanan pengiriman satu kurir (REG, YES, BEST, ...).
type Service struct {
	ID                string    `json:"id"`
	CourierID         string    `json:"courier_id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	SupportsCOD       bool      `json:"supports_cod"`
	SupportsPickup    bool      `json:"supports_pickup"`
	SupportsInsurance bool      `json:"supports_insurance"`
	EtaMinDays        int       `json:"eta_min_days"`
	EtaMaxDays        int       `json:"eta_max_days"`
	VolumetricDivisor int       `json:"volumetric_divisor"`
	ReliabilityScore  float64   `json:"reliability_score"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Rate adalah tarif satu layanan untuk pasangan zona dan tier berat.
type Rate struct {
	ID               string     `json:"id"`
	CourierServiceID string     `json:"courier_service_id"`
	OriginZone       string     `json:"origin_zone"`
	DestinationZone  string     `json:"destination_zone"`
	MinWeight        float64    `json:"min_weight"` // kg, inklusif
	MaxWeight        *float64   `json:"max_weight"` // kg, eksklusif; NULL = tanpa batas
	Price            int64      `json:"price"`      // IDR
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to,omitempty"`
}

// Zone adalah kelompok wilayah untuk penentuan tarif.
type Zone struct {
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // province | city | district | postal_prefix
	Values    []string  `json:"values"`
	CreatedAt time.Time `json:"created_at"`
}

// RateQuery adalah permintaan pencarian tarif untuk satu rute.
// Berat yang dipakai adalah chargeable weight — dihitung oleh pricing engine
// (TASK-023); di TASK-022 pemanggil mengirim berat siap-pakai.
type RateQuery struct {
	OriginCityID      string
	DestinationCityID string
	WeightKG          float64
	IsCOD             bool
}

// ServiceRate adalah hasil lookup: layanan + tarif yang cocok untuk query.
type ServiceRate struct {
	Courier         Courier `json:"courier"`
	Service         Service `json:"service"`
	Price           int64   `json:"price"`
	OriginZone      string  `json:"origin_zone"`
	DestinationZone string  `json:"destination_zone"`
}

// Valid memeriksa invariant query tarif.
func (q RateQuery) Validate() error {
	fields := make(map[string]string)
	if q.OriginCityID == "" {
		fields["origin_city_id"] = "Kota asal wajib diisi."
	}
	if q.DestinationCityID == "" {
		fields["destination_city_id"] = "Kota tujuan wajib diisi."
	}
	if q.WeightKG <= 0 {
		fields["weight"] = "Berat harus lebih dari 0."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// MatchesWeight melaporkan apakah rate mencakup berat tertentu.
// Rentang: min_weight <= w < max_weight; max NULL berarti tanpa batas atas.
func (r Rate) MatchesWeight(w float64) bool {
	if w < r.MinWeight {
		return false
	}
	return r.MaxWeight == nil || w < *r.MaxWeight
}

// Sentinel errors
var (
	ErrNotFound        = shared.NotFound("COURIER_NOT_FOUND", "Kurir tidak ditemukan.")
	ErrServiceNotFound = shared.NotFound("COURIER_SERVICE_NOT_FOUND", "Layanan kurir tidak ditemukan.")
	ErrZoneNotFound    = shared.NotFound("ZONE_NOT_FOUND", "Zona tarif untuk wilayah ini belum tersedia.")
	ErrRateNotFound    = shared.NotFound("RATE_NOT_FOUND", "Tarif untuk rute/berat ini belum tersedia.")
)
