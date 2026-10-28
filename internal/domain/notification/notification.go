// Package notification berisi entitas & aturan notifikasi (TASK-040) yang
// murni: tanpa akses DB, tanpa jaringan.
//
// Kontrak yang dijaga di sini:
//   - template memakai placeholder {{variabel}}; rendering Substitution-safe
//     (variabel tak dikenal dibiarkan apa adanya, bukan error);
//   - preferensi per pengguna, bukan per organisasi — notifikasi bersifat
//     pribadi;
//   - event key NOTIFIKASI terpisah dari event domain (shipment.delivered),
//     sehingga kanal bisa_added tanpa mengubah katalog event.
package notification

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Channel adalah kanal pengiriman notifikasi. WhatsApp/SMS/push menyusul lewat
// adapter (belum ada di TASK-040).
type Channel string

const (
	ChannelInApp Channel = "inapp"
	ChannelEmail Channel = "email"
)

// ValidChannel melaporkan apakah kanal dikenal.
func (c Channel) Valid() bool { return c == ChannelInApp || c == ChannelEmail }

// Priority menentukan tampilan (urutan, warna) notifikasi.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// ValidPriority melaporkan apakah prioritas dikenal.
func (p Priority) Valid() bool {
	return p == PriorityLow || p == PriorityNormal || p == PriorityHigh
}

// EventKey adalah kunci event notifikasi (mis. "shipment.delivered"). Terpisah
// dari event domain supaya template & preferensi punya namespace sendiri.
type EventKey string

// Katalog event notifikasi yang didukung. Nilainya = kunci template.
const (
	EventShipmentCreated   EventKey = "shipment.created"
	EventShipmentPickedUp  EventKey = "shipment.picked_up"
	EventShipmentInTransit EventKey = "shipment.in_transit"
	EventShipmentDelivered EventKey = "shipment.delivered"
	EventShipmentFailed    EventKey = "shipment.failed"
	EventShipmentReturned  EventKey = "shipment.returned"
	EventShipmentCancelled EventKey = "shipment.cancelled"
	EventBulkJobCompleted  EventKey = "bulk.job_completed"
)

// ValidEventKey melaporkan apakah event notifikasi dikenal.
func (k EventKey) Valid() bool {
	switch k {
	case EventShipmentCreated, EventShipmentPickedUp, EventShipmentInTransit,
		EventShipmentDelivered, EventShipmentFailed, EventShipmentReturned,
		EventShipmentCancelled, EventBulkJobCompleted:
		return true
	}
	return false
}

// EventKeys mengembalikan seluruh event notifikasi yang didukung (terurut).
func EventKeys() []EventKey {
	return []EventKey{
		EventShipmentCreated, EventShipmentPickedUp, EventShipmentInTransit,
		EventShipmentDelivered, EventShipmentFailed, EventShipmentReturned,
		EventShipmentCancelled, EventBulkJobCompleted,
	}
}

// DefaultTemplate adalah template bawaan per event & channel. Dipakai saat
// organisasi belum customizing template — thus server tidak pernah gagal
// hanya karena template kosong.
type DefaultTemplate struct {
	Title string
	Body  string
}

// DefaultTemplates adalah katalog template bawaan.
var DefaultTemplates = map[EventKey]DefaultTemplate{
	EventShipmentCreated: {
		Title: "Pengiriman {{awb}} dibuat",
		Body:  "Pengiriman {{awb}} dengan nomor resi {{awb}} sudah dibuat dan menunggu penjemputan.",
	},
	EventShipmentPickedUp: {
		Title: "Pengiriman {{awb}} dijemput",
		Body:  "Kurir telah menjemput paket {{awb}}.",
	},
	EventShipmentInTransit: {
		Title: "Pengiriman {{awb}} dalam perjalanan",
		Body:  "Paket {{awb}} sedang dalam perjalanan.",
	},
	EventShipmentDelivered: {
		Title: "Pengiriman {{awb}} selesai",
		Body:  "Paket {{awb}} telah diterima.",
	},
	EventShipmentFailed: {
		Title: "Pengiriman {{awb}} gagal",
		Body:  "Pengiriman {{awb}} bermasalah: {{reason}}.",
	},
	EventShipmentReturned: {
		Title: "Pengiriman {{awb}} dikembalikan",
		Body:  "Paket {{awb}} dikembalikan ke pengirim.",
	},
	EventShipmentCancelled: {
		Title: "Pengiriman {{awb}} dibatalkan",
		Body:  "Pengiriman {{awb}} dibatalkan: {{reason}}.",
	},
	EventBulkJobCompleted: {
		Title: "Pengiriman massal selesai",
		Body:  "Job pengiriman massal selesai: {{created}} resi terbit, {{failed}} baris gagal.",
	},
}

// Notification adalah satu notifikasi yang sudah dirender untuk satu pengguna.
type Notification struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	UserID         string         `json:"user_id"`
	EventKey       EventKey       `json:"event_key"`
	Channel        Channel        `json:"channel"`
	Priority       Priority       `json:"priority"`
	Title          string         `json:"title"`
	Body           string         `json:"body"`
	Data           map[string]any `json:"data,omitempty"`
	ReadAt         *time.Time     `json:"read_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Read apakah notifikasi sudah dibaca.
func (n Notification) Read() bool { return n.ReadAt != nil }

// Template adalah template pesan milik satu organisasi.
type Template struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Key            EventKey  `json:"key"`
	Channel        Channel   `json:"channel"`
	TitleTemplate  string    `json:"title_template"`
	BodyTemplate   string    `json:"body_template"`
	Enabled        bool      `json:"enabled"`
	UpdatedBy      string    `json:"updated_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Preference adalah preferensi satu pengguna untuk satu event & kanal.
type Preference struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	EventKey  EventKey  `json:"event_key"`
	Channel   Channel   `json:"channel"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Render mengisi placeholder {{variabel}} pada template. Variabel yang tidak
// tersedia dibiarkan apa adanya — lebih baik terlihat kurang lengkap daripada
// pesan hilang total.
func Render(tpl string, vars map[string]any) string {
	if tpl == "" {
		return ""
	}
	// Placeholder diurutkan agar nama panjang lebih dulu (mis. {{awb}} vs
	// {{awb_label}}) tidak tertimpa.
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })

	out := tpl
	for _, k := range keys {
		val, ok := vars[k]
		if !ok {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+k+"}}", fmt.Sprintf("%v", val))
	}
	return out
}

// TemplateFor mengembalikan template bawaan untuk event & kanal.
func TemplateFor(key EventKey, ch Channel) (DefaultTemplate, bool) {
	t, ok := DefaultTemplates[key]
	if !ok {
		return DefaultTemplate{}, false
	}
	return t, true
}

// PriorityFor menetapkan prioritas bawaan per event. Peristiwa yang butuh
// tindakan cepat (gagal, dibatalkan) mendapat prioritas lebih tinggi.
func PriorityFor(key EventKey) Priority {
	switch key {
	case EventShipmentFailed, EventShipmentCancelled:
		return PriorityHigh
	case EventShipmentDelivered:
		return PriorityNormal
	default:
		return PriorityLow
	}
}

// ShipmentNotice adalah satu perubahan status pengiriman yang akan menjadi
// notifikasi. Struct ini jembatan antara tracking (pemicu) dan notifikasi
// (penerima) — keduanya mengimpor domain, bukan satu sama lain.
type ShipmentNotice struct {
	OrganizationID string
	ShipmentID     string
	AWB            string
	// Status adalah status BARU pengiriman (mis. "in_transit").
	Status      string
	ActorUserID string
	OccurredAt  time.Time
}

// EventKeyForShipmentStatus memetakan status pengiriman ke event notifikasi.
// Status tanpa notifikasi (mis. pending_pickup awal) mengembalikan ok=false.
func EventKeyForShipmentStatus(status string) (EventKey, bool) {
	switch status {
	case "picked_up":
		return EventShipmentPickedUp, true
	case "in_transit":
		return EventShipmentInTransit, true
	case "out_for_delivery":
		return EventShipmentInTransit, true // satu rumpun: perjalanan
	case "delivered":
		return EventShipmentDelivered, true
	case "failed":
		return EventShipmentFailed, true
	case "returned":
		return EventShipmentReturned, true
	case "cancelled":
		return EventShipmentCancelled, true
	default:
		return "", false
	}
}
