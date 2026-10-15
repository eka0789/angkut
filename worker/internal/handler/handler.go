// Package handler berisi handler pesan untuk worker. Setiap handler
// menerjemahkan satu jenis event menjadi perubahan state, dan WAJIB idempotent
// karena broker menjamin at-least-once (pesan bisa datang dua kali).
package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/application/trackingapp"
	"github.com/angkut/angkut/apps/internal/domain/event"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
	"github.com/angkut/angkut/apps/internal/domain/webhook"
	"github.com/angkut/angkut/apps/internal/infrastructure/consumer"
)

// TrackingRecorder adalah sisi service yang dibutuhkan handler tracking.
type TrackingRecorder interface {
	Append(ctx context.Context, orgID, actorUserID, requestID string, in trackingapp.AppendInput) (*trackingapp.AppendResult, error)
}

// WebhookStore adalah sisi repository webhook yang dibutuhkan handler.
type WebhookStore interface {
	GetByID(ctx context.Context, id string) (*webhook.Event, error)
	MarkProcessed(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, reason string) error
	MarkIgnored(ctx context.Context, id, reason string) error
}

// ShipmentResolver mencari shipment dari AWB dalam konteks tenant.
type ShipmentResolver interface {
	FindShipmentIDByAWB(ctx context.Context, orgID, awb string) (string, error)
}

// WebhookProcessor menerjemahkan event webhook yang sudah tervalidasi menjadi
// event pelacakan.
//
// Perhatikan: signature SUDAH diverifikasi di sisi HTTP sebelum pesan masuk
// antrean, jadi handler ini tidak memverifikasi ulang — tetapi ia tetap
// memperlakukan payload sebagai data tak terpercaya saat memetakan status.
type WebhookProcessor struct {
	Webhooks  WebhookStore
	Tracking  TrackingRecorder
	Shipments ShipmentResolver
	Log       *slog.Logger
}

// Handle memproses satu pesan tracking.event.received.
func (h *WebhookProcessor) Handle(ctx context.Context, msg consumer.Message) error {
	if msg.Payload == nil {
		// Bentuk payload rusak tidak akan membaik dengan pengulangan.
		return consumer.Permanent(fmt.Errorf("payload pesan kosong atau bukan JSON"))
	}

	webhookID := stringField(msg.Payload, "webhook_event_id")
	awb := strings.TrimSpace(stringField(msg.Payload, "awb"))
	provider := stringField(msg.Payload, "provider")

	if awb == "" {
		// Tanpa AWB kita tidak tahu pengiriman mana yang dimaksud. Ini bukan
		// kegagalan sistem, jadi event ditandai selesai dengan catatan.
		h.logf().Warn("webhook tanpa AWB — dilewati", "webhook_event_id", webhookID, "provider", provider)
		return h.markIgnored(ctx, webhookID, "payload tidak memuat nomor resi")
	}

	orgID := stringField(msg.Payload, "organization_id")
	if orgID == "" {
		// Kurir tidak tahu tenant; resi yang unik per organisasi dipakai untuk
		// menemukan pemiliknya. Bila tidak ketemu, event tidak bisa diterapkan.
		h.logf().Warn("webhook tanpa organisasi — dilewati", "webhook_event_id", webhookID, "awb", awb)
		return h.markIgnored(ctx, webhookID, "organisasi tidak dapat ditentukan dari payload")
	}

	shipmentID, err := h.Shipments.FindShipmentIDByAWB(ctx, orgID, awb)
	if err != nil {
		if shared.Is(err, shared.ErrNotFound) {
			// Resi tidak dikenal (mungkin milik tenant lain atau belum terdaftar).
			// Bukan kegagalan sistem: tandai agar tidak diulang selamanya.
			h.logf().Warn("webhook untuk resi tak dikenal", "awb", awb, "org", orgID)
			return h.markIgnored(ctx, webhookID, "resi "+awb+" tidak ditemukan")
		}
		return err
	}

	// Payload kurir dipetakan ke status tracking internal. Pemetaan yang gagal
	// berarti event tetap tersimpan mentah, tetapi tidak mengubah status.
	status, ok := mapProviderStatus(msg.Payload)
	if !ok {
		h.logf().Info("status provider tidak dikenali — event disimpan tanpa mengubah status",
			"awb", awb, "webhook_event_id", webhookID)
		return h.markIgnored(ctx, webhookID, "status provider tidak dikenali")
	}

	occurred := parseTime(stringField(msg.Payload, "occurred_at"))
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}

	res, err := h.Tracking.Append(ctx, orgID, "", "", trackingapp.AppendInput{
		ShipmentID: shipmentID,
		Status:     status,
		Location:   stringField(msg.Payload, "location"),
		OccurredAt: occurred,
		Source:     tracking.SourceWebhook,
		RawEventID: webhookID,
	})
	if err != nil {
		return fmt.Errorf("mencatat event pelacakan: %w", err)
	}

	if res.Duplicate {
		h.logf().Info("event pelacakan sudah ada — idempotent", "awb", awb, "status", string(status))
	}

	if webhookID != "" {
		if err := h.Webhooks.MarkProcessed(ctx, webhookID); err != nil {
			return fmt.Errorf("menandai webhook diproses: %w", err)
		}
	}

	h.logf().Info("webhook diproses",
		"awb", awb, "status", string(status), "status_berubah", res.StatusChanged)
	return nil
}

// markIgnored menandai webhook selesai tanpa perubahan state. Ini bukan
// kegagalan: mengulanginya tidak akan mengubah apa pun, sehingga pesan tidak
// perlu dicoba ulang.
func (h *WebhookProcessor) markIgnored(ctx context.Context, webhookID, reason string) error {
	if webhookID == "" {
		return nil
	}
	if err := h.Webhooks.MarkIgnored(ctx, webhookID, reason); err != nil {
		return fmt.Errorf("menandai webhook diabaikan: %w", err)
	}
	return nil
}

func (h *WebhookProcessor) logf() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

// providerStatusMap memetakan status kurir (beragam ejaan) ke status internal.
// Kunci dalam huruf kecil; pencocokan dilakukan setelah normalisasi.
var providerStatusMap = map[string]tracking.EventStatus{
	// Diambil / dalam perjalanan
	"picked":           tracking.EventPickedUp,
	"picked_up":        tracking.EventPickedUp,
	"pickup":           tracking.EventPickedUp,
	"diambil":          tracking.EventPickedUp,
	"on_process":       tracking.EventInTransit,
	"in_transit":       tracking.EventInTransit,
	"transit":          tracking.EventInTransit,
	"on_delivery":      tracking.EventOutForDelivery,
	"out_for_delivery": tracking.EventOutForDelivery,
	"delivering":       tracking.EventOutForDelivery,
	"delivered":        tracking.EventDelivered,
	"terkirim":         tracking.EventDelivered,
	"received":         tracking.EventDelivered,
	"failed":           tracking.EventFailed,
	"failed_delivery":  tracking.EventFailed,
	"gagal":            tracking.EventFailed,
	"returned":         tracking.EventReturned,
	"return":           tracking.EventReturned,
	"dikembalikan":     tracking.EventReturned,
	"sorting":          tracking.EventSorting,
	"manifest":         tracking.EventSorting,
}

// mapProviderStatus mencari status internal dari payload kurir.
//
// Beberapa provider membungkus status di dalam objek; kunci yang dicoba
// sengaja banyak karena tidak ada standar bersama antar kurir.
func mapProviderStatus(payload map[string]any) (tracking.EventStatus, bool) {
	candidates := []string{"status", "event", "event_type", "type", "status_code", "delivery_status"}
	for _, key := range candidates {
		if raw, ok := payload[key].(string); ok {
			if s, ok := lookupStatus(raw); ok {
				return s, true
			}
		}
	}
	// Coba satu level bersarang.
	for _, key := range []string{"data", "shipment", "detail"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if s, ok := mapProviderStatus(nested); ok {
				return s, true
			}
		}
	}
	return "", false
}

// lookupStatus menormalkan ejaan status sebelum mencocokkan.
func lookupStatus(raw string) (tracking.EventStatus, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	if s, ok := providerStatusMap[key]; ok {
		return s, true
	}
	// Nilai yang sudah berupa status internal diterima apa adanya.
	for _, s := range tracking.AllEventStatuses {
		if key == string(s) {
			return s, true
		}
	}
	return "", false
}

// stringField mengambil field string dari payload.
func stringField(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

// parseTime mengurai waktu dengan beberapa format yang lazim dikirim kurir.
func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// ShipmentEventLogger adalah handler sederhana untuk event shipment.* yang
// belum punya consumer fungsional (Notifikasi/Analitik menyusul). Tujuannya:
// pesan TIDAK menumpuk di queue, dan jejaknya tetap terlihat di log.
type ShipmentEventLogger struct {
	Log *slog.Logger
}

// Handle mencatat event shipment ke log.
func (h *ShipmentEventLogger) Handle(_ context.Context, msg consumer.Message) error {
	log := h.Log
	if log == nil {
		log = slog.Default()
	}
	log.Info("event shipment diterima",
		"event_id", msg.ID, "type", msg.Type, "routing_key", msg.RoutingKey)
	return nil
}

// CatalogEvents adalah daftar routing key yang dipakai untuk binding.
func CatalogEvents() []string {
	return []string{
		string(event.ShipmentCreated),
		string(event.ShipmentUpdated),
		string(event.ShipmentPickedUp),
		string(event.ShipmentInTransit),
		string(event.ShipmentDelivered),
		string(event.ShipmentFailed),
		string(event.ShipmentReturned),
		string(event.ShipmentCancelled),
		string(event.OrderCreated),
		string(event.OrderCancelled),
		string(event.TrackingEventReceived),
		string(event.NotificationSend),
	}
}

// TrackingSyncHandler mencatat event tracking dari pesan shipment.* yang
// diterbitkan trackingapp. Fungsinya menjaga timeline tetap lengkap walau
// perubahan status dilakukan dari jalur non-webhook (manual/admin).
//
// Idempotensi dijaga konsumer lewat dedup event_id, ditambah kunci alami
// (shipment_id, status, occurred_at, source) di tabel tracking_events —
// jadi pesan yang datang dua kali tidak menghasilkan baris ganda.
type TrackingSyncHandler struct {
	Tracking TrackingRecorder
	Log      *slog.Logger
}

// Handle memproses satu pesan shipment.*.
func (h *TrackingSyncHandler) Handle(ctx context.Context, msg consumer.Message) error {
	if msg.Payload == nil {
		return consumer.Permanent(fmt.Errorf("payload pesan kosong atau bukan JSON"))
	}

	shipmentID := stringField(msg.Payload, "shipment_id")
	orgID := stringField(msg.Payload, "organization_id")
	statusRaw := stringField(msg.Payload, "status")
	if shipmentID == "" || orgID == "" {
		h.logf().Warn("event shipment tanpa id/organisasi — dilewati",
			"event_id", msg.ID, "type", msg.Type)
		return nil
	}

	status, ok := lookupStatus(statusRaw)
	if !ok {
		// Status yang tidak dikenali bukan kesalahan: status shipment
		// (in_transit/delivered) memang sebagian tidak ada padanannya sebagai
		// event tracking. Catat sekali, jangan ulang.
		h.logf().Debug("status shipment tanpa padanan event tracking",
			"status", statusRaw, "shipment_id", shipmentID)
		return nil
	}

	occurred := parseTime(stringField(msg.Payload, "occurred_at"))
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}

	res, err := h.Tracking.Append(ctx, orgID, "", "", trackingapp.AppendInput{
		ShipmentID: shipmentID,
		Status:     status,
		Location:   stringField(msg.Payload, "location"),
		OccurredAt: occurred,
		Source:     tracking.SourceSync,
	})
	if err != nil {
		return fmt.Errorf("mencatat event tracking dari event shipment: %w", err)
	}
	if res.Duplicate {
		h.logf().Debug("event tracking sudah ada (idempotent)", "shipment_id", shipmentID)
	}
	return nil
}

func (h *TrackingSyncHandler) logf() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}
