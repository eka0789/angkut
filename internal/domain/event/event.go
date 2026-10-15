// Package event berisi kontrak event domain yang mengalir lewat outbox ke
// RabbitMQ (ADR-005). Event ditulis dalam transaksi yang SAMA dengan perubahan
// state, lalu direlay worker — sehingga tidak ada state berubah tanpa event.
package event

import (
	"strings"
	"time"
)

// Type adalah nama event. Format routing key di RabbitMQ adalah `{aggregate}.{action}`,
// sehingga nama event sekaligus menentukan tujuan routing.
type Type string

// Katalog event (planning/architecture.md §6). Dikumpulkan di sini agar
// producer dan consumer tidak bergantung pada string yang diketik manual.
const (
	ShipmentCreated  Type = "shipment.created"
	ShipmentUpdated  Type = "shipment.updated"
	ShipmentPickedUp Type = "shipment.picked_up"
	ShipmentInTransit Type = "shipment.in_transit"
	ShipmentDelivered Type = "shipment.delivered"
	ShipmentFailed    Type = "shipment.failed"
	ShipmentReturned  Type = "shipment.returned"
	ShipmentCancelled Type = "shipment.cancelled"

	OrderCreated   Type = "order.created"
	OrderCancelled Type = "order.cancelled"

	TrackingEventReceived Type = "tracking.event.received"
	NotificationSend      Type = "notification.send"
)

// Aggregate adalah jenis entitas pemilik event. Nilai ini dipakai untuk
// ordering per-aggregate dan untuk kolom aggregate_type di outbox.
type Aggregate string

const (
	AggregateShipment Aggregate = "shipment"
	AggregateOrder    Aggregate = "order"
	AggregateTracking Aggregate = "tracking"
)

// Event adalah satu niat menerbitkan pesan, ditulis di dalam transaksi bisnis.
type Event struct {
	// ID adalah identitas event; consumer memakainya untuk dedup sehingga
	// pengiriman ulang (at-least-once) tidak diproses dua kali.
	ID string

	AggregateType Aggregate
	AggregateID   string

	OrganizationID string
	Type           Type

	Payload map[string]any
	Headers map[string]any

	OccurredAt time.Time
}

// RoutingKey mengembalikan kunci routing RabbitMQ (`{aggregate}.{action}`).
// Diturunkan dari Type agar tidak bisa lepas sinkron dengan katalog.
func (e Event) RoutingKey() string { return string(e.Type) }

// AggregateOf menurunkan jenis aggregate dari nama event. Dipakai bila
// pemanggil tidak menyebutkannya eksplisit.
func AggregateOf(t Type) Aggregate {
	name := string(t)
	if i := strings.IndexByte(name, '.'); i > 0 {
		return Aggregate(name[:i])
	}
	return ""
}

// New membuat event dengan ID dan waktu kejadian yang harus diberikan pemanggil
// (agar dapat diuji dan agar waktunya sama dengan transaksi bisnisnya).
func New(id string, t Type, aggregateID, orgID string, at time.Time, payload map[string]any) Event {
	return Event{
		ID:             id,
		AggregateType:  AggregateOf(t),
		AggregateID:    aggregateID,
		OrganizationID: orgID,
		Type:           t,
		Payload:        payload,
		OccurredAt:     at,
	}
}

// Status adalah keadaan satu baris outbox selama perjalanan relay.
type Status string

const (
	StatusPending   Status = "pending"
	StatusPublished Status = "published"
	StatusFailed    Status = "failed"
	StatusDead      Status = "dead"
)

// Record adalah satu baris outbox yang dibaca relay.
type Record struct {
	ID             int64
	AggregateType  string
	AggregateID    string
	OrganizationID string
	Type           Type
	Payload        map[string]any
	Headers        map[string]any
	Attempts       int
	CreatedAt      time.Time
}

// RoutingKey mengembalikan kunci routing RabbitMQ untuk baris outbox.
// Formatnya sama dengan Event.RoutingKey (`{aggregate}.{action}`).
func (r Record) RoutingKey() string { return string(r.Type) }