// Package pickup berisi permintaan penjemputan paket oleh kurir dari gudang.
package pickup

import (
	"fmt"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Status adalah status permintaan penjemputan.
type Status string

const (
	StatusRequested  Status = "requested"
	StatusScheduled  Status = "scheduled"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

// AllStatuses adalah daftar kanonik status.
var AllStatuses = []Status{StatusRequested, StatusScheduled, StatusCompleted, StatusCancelled}

// Valid melaporkan apakah status dikenal.
func (s Status) Valid() bool {
	for _, v := range AllStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Terminal melaporkan apakah status adalah akhir.
func (s Status) Terminal() bool {
	return s == StatusCompleted || s == StatusCancelled
}

// Request adalah satu permintaan penjemputan.
type Request struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	WarehouseID    string    `json:"warehouse_id"`
	CourierCode    string    `json:"courier_code"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
	Status         Status    `json:"status"`
	Notes          string    `json:"notes,omitempty"`
	CreatedBy      string    `json:"created_by,omitempty"`
	ShipmentIDs    []string  `json:"shipment_ids,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate memeriksa invariant.
func (r *Request) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(r.WarehouseID) == "" {
		fields["warehouse_id"] = "Gudang wajib diisi."
	}
	if strings.TrimSpace(r.CourierCode) == "" {
		fields["courier_code"] = "Kurir wajib diisi."
	}
	if !r.Status.Valid() {
		fields["status"] = "Status penjemputan tidak dikenal."
	}
	if r.ScheduledAt != nil && r.ScheduledAt.IsZero() {
		fields["scheduled_at"] = "Waktu jadwal tidak valid."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// CanTransition menegakkan state machine.
func (r *Request) CanTransition(target Status) bool {
	switch r.Status {
	case StatusRequested:
		return target == StatusScheduled || target == StatusCancelled
	case StatusScheduled:
		return target == StatusCompleted || target == StatusCancelled
	default:
		return false
	}
}

// Transition menerapkan perubahan status.
func (r *Request) Transition(target Status) error {
	if !r.CanTransition(target) {
		return shared.New("PICKUP_INVALID_TRANSITION",
			fmt.Sprintf("Tidak bisa memindahkan penjemputan dari %s ke %s.", r.Status, target),
			shared.ErrConflict)
	}
	r.Status = target
	r.UpdatedAt = time.Now().UTC()
	return nil
}

var (
	ErrNotFound = shared.NotFound("PICKUP_NOT_FOUND", "Permintaan penjemputan tidak ditemukan.")
)