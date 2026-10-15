// Package address berisi entitas alamat yang terikat ke tenant (organization_id).
// Pinsip isolasi tenant ADR-008 wajib ditegakkan di setiap query.
package address

import (
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Address merepresentasikan satu alamat pengiriman/penerima yang disimpan tenant.
type Address struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Recipient      string    `json:"recipient"`
	Phone          string    `json:"phone"`
	ProvinceID     string    `json:"province_id"`
	CityID         string    `json:"city_id"`
	DistrictID     string    `json:"district_id"`
	VillageID      string    `json:"village_id,omitempty"`
	PostalCode     string    `json:"postal_code,omitempty"`
	Street         string    `json:"street"`
	Notes          string    `json:"notes,omitempty"`
	Latitude       *float64  `json:"latitude,omitempty"`
	Longitude      *float64  `json:"longitude,omitempty"`
	Label          string    `json:"label,omitempty"`
	IsDefault      bool      `json:"is_default"`
	Status         Status    `json:"status"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Status alamat.
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// ValidStatus melaporkan apakah status valid.
func (s Status) Valid() bool { return s == StatusActive || s == StatusArchived }

// Validate memeriksa invariant dasar sebelum disimpan ke DB.
// Pemeriksaan FK (province_id/city_id) diserahkan ke DB constraint.
func (a *Address) Validate() error {
	fields := make(map[string]string)

	if strings.TrimSpace(a.Recipient) == "" {
		fields["recipient"] = "Nama penerima wajib diisi."
	}
	if strings.TrimSpace(a.Phone) == "" {
		fields["phone"] = "Nomor telepon wajib diisi."
	}
	if strings.TrimSpace(a.ProvinceID) == "" {
		fields["province_id"] = "Provinsi wajib dipilih."
	}
	if strings.TrimSpace(a.CityID) == "" {
		fields["city_id"] = "Kota wajib dipilih."
	}
	if strings.TrimSpace(a.DistrictID) == "" {
		fields["district_id"] = "Kecamatan wajib dipilih."
	}
	if strings.TrimSpace(a.Street) == "" {
		fields["street"] = "Alamat jalan wajib diisi."
	}

	// Validasi map pin: kalau ada salah satu, keduanya wajib ada.
	if (a.Latitude == nil) != (a.Longitude == nil) {
		fields["pin"] = "Latitude dan longitude harus diisi bersama-sama."
	}
	if a.Latitude != nil && (*a.Latitude < -90 || *a.Latitude > 90) {
		fields["latitude"] = "Latitude harus antara -90 dan 90."
	}
	if a.Longitude != nil && (*a.Longitude < -180 || *a.Longitude > 180) {
		fields["longitude"] = "Longitude harus antara -180 dan 180."
	}

	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// ValidationResult adalah hasil validasi alamat terhadap data wilayah.
type ValidationResult struct {
	Valid       bool     `json:"valid"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

// Sentinel errors
var (
	ErrNotFound = shared.NotFound("ADDRESS_NOT_FOUND", "Alamat tidak ditemukan.")
)
