// Package tracking berisi event pengiriman dan state machine yang menurunkannya.
//
// Pemisahan penting: `shipment.Status` adalah status RESMI pengiriman (dipakai
// harga, COD, KPI), sementara `tracking.EventStatus` adalah kejadian di
// lapangan yang dilaporkan kurir. Satu event tracking dapat memindahkan status
// shipment, tetapi tidak semua event melakukannya (mis. `sorting` hanya
// informasi). Pemetaan itu eksplisit di ShipmentStatusFor().
package tracking

import (
	"fmt"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
)

// EventStatus adalah kejadian yang dilaporkan kurir. Nilai HARUS sama dengan
// CHECK di migrasi 0017.
type EventStatus string

const (
	EventCreated         EventStatus = "created"
	EventPickupRequested EventStatus = "pickup_requested"
	EventPickedUp        EventStatus = "picked_up"
	EventSorting         EventStatus = "sorting"
	EventInTransit       EventStatus = "in_transit"
	EventOutForDelivery  EventStatus = "out_for_delivery"
	EventDelivered       EventStatus = "delivered"
	EventFailed          EventStatus = "failed"
	EventReturned        EventStatus = "returned"
)

// AllEventStatuses adalah urutan kanonik untuk validasi & iterasi.
var AllEventStatuses = []EventStatus{
	EventCreated, EventPickupRequested, EventPickedUp, EventSorting,
	EventInTransit, EventOutForDelivery, EventDelivered, EventFailed, EventReturned,
}

// Valid melaporkan apakah status event dikenal.
func (s EventStatus) Valid() bool {
	for _, v := range AllEventStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Source adalah asal sebuah event. Nilai HARUS sama dengan CHECK di migrasi 0017.
type Source string

const (
	SourceWebhook Source = "webhook"
	SourceManual  Source = "manual"
	SourceSystem  Source = "system"
	SourceSync    Source = "sync"
)

// Valid melaporkan apakah sumber dikenal.
func (s Source) Valid() bool {
	switch s {
	case SourceWebhook, SourceManual, SourceSystem, SourceSync:
		return true
	default:
		return false
	}
}

// Event adalah satu kejadian pengiriman. Append-only: tidak ada Update/Delete.
type Event struct {
	ID             string      `json:"id"`
	OrganizationID string      `json:"organization_id"`
	ShipmentID     string      `json:"shipment_id"`
	Status         EventStatus `json:"status"`
	Description    string      `json:"description,omitempty"`
	Location       string      `json:"location,omitempty"`
	Latitude       *float64    `json:"latitude,omitempty"`
	Longitude      *float64    `json:"longitude,omitempty"`
	OccurredAt     time.Time   `json:"occurred_at"`
	Source         Source      `json:"source"`
	RawEventID     string      `json:"raw_event_id,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
}

// Validate memeriksa invariant satu event.
func (e *Event) Validate() error {
	fields := make(map[string]string)

	if strings.TrimSpace(e.ShipmentID) == "" {
		fields["shipment_id"] = "Pengiriman wajib diisi."
	}
	if !e.Status.Valid() {
		fields["status"] = "Status tracking tidak dikenal."
	}
	if !e.Source.Valid() {
		fields["source"] = "Sumber event tidak dikenal."
	}
	if e.OccurredAt.IsZero() {
		fields["occurred_at"] = "Waktu kejadian wajib diisi."
	}
	// Koordinat harus lengkap berpasangan — sama dengan CHECK di migrasi.
	if (e.Latitude == nil) != (e.Longitude == nil) {
		fields["coordinates"] = "Latitude dan longitude harus diisi bersama."
	}
	if e.Latitude != nil && (*e.Latitude < -90 || *e.Latitude > 90) {
		fields["latitude"] = "Latitude harus antara -90 dan 90."
	}
	if e.Longitude != nil && (*e.Longitude < -180 || *e.Longitude > 180) {
		fields["longitude"] = "Longitude harus antara -180 dan 180."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// PromotesShipment melaporkan apakah event ini boleh memindahkan status
// pengiriman. Event informasi (created, sorting) tidak memindahkan status.
func (s EventStatus) PromotesShipment() bool {
	switch s {
	case EventPickedUp, EventInTransit, EventOutForDelivery, EventDelivered, EventFailed, EventReturned:
		return true
	default:
		return false
	}
}

// ShipmentStatusFor memetakan status event ke status pengiriman resmi.
// ok=false berarti event ini tidak mengubah status pengiriman.
func ShipmentStatusFor(e EventStatus) (shipment.Status, bool) {
	switch e {
	case EventPickedUp:
		return shipment.StatusPickedUp, true
	case EventInTransit:
		return shipment.StatusInTransit, true
	case EventOutForDelivery:
		return shipment.StatusOutForDelivery, true
	case EventDelivered:
		return shipment.StatusDelivered, true
	case EventFailed:
		return shipment.StatusFailed, true
	case EventReturned:
		return shipment.StatusReturned, true
	default:
		return "", false
	}
}

// DescriptionFor memberi deskripsi bawaan berbahasa Indonesia untuk sebuah
// status, dipakai bila kurir tidak mengirim deskripsi.
func DescriptionFor(s EventStatus) string {
	switch s {
	case EventCreated:
		return "Pengiriman dibuat"
	case EventPickupRequested:
		return "Penjemputan diminta"
	case EventPickedUp:
		return "Paket dijemput kurir"
	case EventSorting:
		return "Paket disortir di gudang"
	case EventInTransit:
		return "Paket dalam perjalanan"
	case EventOutForDelivery:
		return "Paket sedang diantar"
	case EventDelivered:
		return "Paket diterima penerima"
	case EventFailed:
		return "Pengantaran gagal"
	case EventReturned:
		return "Paket dikembalikan ke pengirim"
	default:
		return string(s)
	}
}

// ConfidenceLevel adalah penilaian apakah pengiriman berjalan sesuai jadwal.
type ConfidenceLevel string

const (
	ConfidenceOnTrack ConfidenceLevel = "on_track"
	ConfidenceDelayed ConfidenceLevel = "delayed"
	ConfidenceUnknown ConfidenceLevel = "unknown"
)

// Confidence adalah penilaian sederhana terhadap ETA pengiriman.
type Confidence struct {
	Level   ConfidenceLevel `json:"level"`
	Message string          `json:"message"`
	Reason  string          `json:"reason,omitempty"`
}

// EvaluateConfidence menilai apakah pengiriman masih sesuai jadwal.
//
// Aturannya sengaja sederhana dan dapat dijelaskan: dibandingkan waktu
// sekarang terhadap eta_max_days dihitung dari booked_at. Tidak ada model
// statistik di sini — klaimnya harus bisa ditelusuri pengguna.
func EvaluateConfidence(sh *shipment.Shipment, now time.Time) Confidence {
	if sh == nil {
		return Confidence{Level: ConfidenceUnknown, Message: "Data pengiriman tidak tersedia."}
	}
	if sh.Status == shipment.StatusDelivered {
		return Confidence{Level: ConfidenceOnTrack, Message: "Paket sudah diterima penerima."}
	}
	if sh.Status.IsTerminal() {
		return Confidence{
			Level:   ConfidenceUnknown,
			Message: "Pengiriman ini sudah berakhir.",
			Reason:  fmt.Sprintf("Status akhir: %s.", sh.Status),
		}
	}
	if sh.BookedAt == nil || sh.EtaMaxDays <= 0 {
		return Confidence{
			Level:   ConfidenceUnknown,
			Message: "Estimasi belum tersedia untuk pengiriman ini.",
		}
	}

	deadline := sh.BookedAt.AddDate(0, 0, sh.EtaMaxDays)
	if !now.After(deadline) {
		return Confidence{
			Level:   ConfidenceOnTrack,
			Message: "Pengiriman berjalan sesuai jadwal.",
		}
	}

	lateDays := int(now.Sub(deadline).Hours()/24) + 1
	return Confidence{
		Level:   ConfidenceDelayed,
		Message: fmt.Sprintf("Pengiriman diperkirakan terlambat sekitar %d hari.", lateDays),
		Reason:  fmt.Sprintf("Melewati estimasi maksimum %d hari sejak pengiriman dibuat.", sh.EtaMaxDays),
	}
}

// Timeline adalah hasil pemrosesan event: status terkini, jejak terurut, dan
// posisi terakhir yang diketahui.
type Timeline struct {
	AWB          string     `json:"awb"`
	Status       string     `json:"status"`
	Events       []Event    `json:"events"`
	LastLocation string     `json:"last_location,omitempty"`
	LastLat      *float64   `json:"last_lat,omitempty"`
	LastLng      *float64   `json:"last_lng,omitempty"`
	OccurredAt   *time.Time `json:"last_occurred_at,omitempty"`
}

// BuildTimeline menyusun timeline dari daftar event. Event diurutkan naik
// berdasarkan waktu kejadian (terlama dulu) karena itu urutan yang dibaca
// manusia; posisi terakhir diambil dari event TERBARU yang punya koordinat.
//
// Urutan stabil untuk waktu yang sama: event dengan waktu identik diurutkan
// berdasarkan urutan kanonik status agar hasilnya tidak berubah-ubah antar
// pemanggilan (penting: timeline adalah bukti, bukan tampilan acak).
func BuildTimeline(awb string, events []Event) Timeline {
	sorted := make([]Event, len(events))
	copy(sorted, events)
	stableSortByTime(sorted)

	tl := Timeline{AWB: awb, Events: sorted, Status: string(shipment.StatusDraft)}
	if len(sorted) == 0 {
		return tl
	}

	// Status timeline = status event terakhir.
	last := sorted[len(sorted)-1]
	tl.Status = string(last.Status)
	occurred := last.OccurredAt
	tl.OccurredAt = &occurred

	// Posisi terakhir: mundur dari event terbaru sampai menemukan koordinat.
	for i := len(sorted) - 1; i >= 0; i-- {
		if sorted[i].Latitude != nil && sorted[i].Longitude != nil {
			tl.LastLat = sorted[i].Latitude
			tl.LastLng = sorted[i].Longitude
			break
		}
	}
	// Lokasi teks: event terbaru yang punya lokasi.
	for i := len(sorted) - 1; i >= 0; i-- {
		if strings.TrimSpace(sorted[i].Location) != "" {
			tl.LastLocation = sorted[i].Location
			break
		}
	}
	return tl
}

// stableSortByTime mengurutkan event berdasarkan waktu, lalu status kanonik.
func stableSortByTime(events []Event) {
	rank := make(map[EventStatus]int, len(AllEventStatuses))
	for i, s := range AllEventStatuses {
		rank[s] = i
	}
	// Insertion sort: kohort per shipment kecil (< 100) dan stabil secara alami.
	for i := 1; i < len(events); i++ {
		cur := events[i]
		j := i - 1
		for j >= 0 && less(cur, events[j], rank) {
			events[j+1] = events[j]
			j--
		}
		events[j+1] = cur
	}
}

func less(a, b Event, rank map[EventStatus]int) bool {
	if !a.OccurredAt.Equal(b.OccurredAt) {
		return a.OccurredAt.Before(b.OccurredAt)
	}
	return rank[a.Status] < rank[b.Status]
}

// ─── sentinel error ──────────────────────────────────────────────────────

var (
	ErrNotFound         = shared.NotFound("TRACKING_NOT_FOUND", "Data pelacakan tidak ditemukan.")
	ErrShipmentNotFound = shared.NotFound("TRACKING_SHIPMENT_NOT_FOUND", "Pengiriman tidak ditemukan.")
)
