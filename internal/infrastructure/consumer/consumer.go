// Package consumer menjalankan konsumsi pesan dari RabbitMQ di worker.
//
// Aturan yang dipegang semua consumer di sini:
//   - ack MANUAL: pesan hanya di-ack setelah efeknya berhasil.
//   - IDEMPOTENT: event_id dicatat di `processed_events` sebelum diproses,
//     sehingga pengiriman ulang (at-least-once) tidak berefek dua kali.
//   - Kegagalan layak-ulang → ke retry exchange (backoff TTL), lalu DLQ.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/angkut/angkut/apps/internal/infrastructure/broker"
)

// Handler memproses satu pesan. Mengembalikan error berarti pesan perlu
// dicoba ulang (retry → DLQ), kecuali error bertanda Permanent.
type Handler func(ctx context.Context, msg Message) error

// Message adalah pesan yang sudah di-parse dari broker.
type Message struct {
	ID         string // event_id dari header (fallback: routing key + timestamp)
	RoutingKey string
	Type       string
	Attempt    int
	Body       []byte
	Payload    map[string]any
	Headers    amqp.Table
}

// Deduper mencatat event_id yang sudah diproses.
type Deduper interface {
	// MarkProcessed mengembalikan (true, nil) bila ini pertama kali,
	// (false, nil) bila sudah pernah diproses.
	MarkProcessed(ctx context.Context, eventID, consumer string) (bool, error)
}

// PermanentError menandai kegagalan yang tidak akan berhasil walau diulang
// (mis. payload rusak bentuknya). Pesan seperti ini langsung ke DLQ.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return "gagal permanen: " + e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent membungkus error sebagai kegagalan permanen.
func Permanent(err error) error { return &PermanentError{Err: err} }

// IsPermanent melaporkan apakah error bertanda permanen.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// Consumer mengonsumsi satu queue.
type Consumer struct {
	Queue    string
	Keys     []string
	Prefetch int
	Handler  Handler
	Deduper  Deduper
	Log      *slog.Logger
}

// Run mendeklarasikan queue lalu mengonsumsi sampai ctx dibatalkan.
func (c *Consumer) Run(ctx context.Context, client *broker.Client) error {
	log := c.Log
	if log == nil {
		log = slog.Default()
	}
	if c.Handler == nil {
		return fmt.Errorf("consumer %s: handler wajib diisi", c.Queue)
	}

	// Klien broker menyembunyikan channel-nya; consumer memakai jalur yang sama
	// supaya topologi (exchange/queue/binding) dibangun sekali di satu tempat.
	ch, err := client.ConsumerChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := broker.DeclareConsumerQueue(ch, c.Queue, c.Keys...); err != nil {
		return err
	}

	prefetch := c.Prefetch
	if prefetch <= 0 {
		prefetch = 10
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("menyetel prefetch: %w", err)
	}

	deliveries, err := ch.Consume(c.Queue, "", false /* autoAck */, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("mulai konsumsi %s: %w", c.Queue, err)
	}

	log.Info("consumer mulai", "queue", c.Queue, "keys", c.Keys, "prefetch", prefetch)

	for {
		select {
		case <-ctx.Done():
			log.Info("consumer berhenti", "queue", c.Queue)
			return ctx.Err()

		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("channel konsumsi %s tertutup", c.Queue)
			}
			c.handle(ctx, ch, d, log)
		}
	}
}

// handle memproses satu delivery dengan ack manual.
func (c *Consumer) handle(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, log *slog.Logger) {
	msg := parseMessage(d)

	// Dedup dulu: pengiriman ulang tidak boleh berefek dua kali.
	if c.Deduper != nil && msg.ID != "" {
		first, err := c.Deduper.MarkProcessed(ctx, msg.ID, c.Queue)
		if err != nil {
			// Tidak bisa memastikan dedup → jangan proses. Nack + requeue agar
			// dicoba lagi setelah database sehat.
			log.Error("gagal mencatat dedup — pesan dikembalikan ke antrean",
				"queue", c.Queue, "event_id", msg.ID, "error", err)
			_ = d.Nack(false, true)
			return
		}
		if !first {
			log.Info("pesan duplikat diabaikan", "queue", c.Queue, "event_id", msg.ID)
			_ = d.Ack(false)
			return
		}
	}

	if err := c.Handler(ctx, msg); err != nil {
		if IsPermanent(err) || msg.Attempt > maxAttempts {
			// Tidak ada gunanya diulang: parkir di DLQ agar tidak menyumbat queue.
			log.Error("pesan gagal permanen — dikirim ke DLQ",
				"queue", c.Queue, "event_id", msg.ID, "attempt", msg.Attempt, "error", err)
			_ = d.Nack(false, false) // requeue=false → dead-letter ke DLX
			return
		}
		log.Warn("pemrosesan pesan gagal — akan dicoba ulang",
			"queue", c.Queue, "event_id", msg.ID, "attempt", msg.Attempt, "error", err)
		// requeue=false agar pesan melewati jalur retry TTL (bukan requeue
		// seketika, yang akan berputar cepat tanpa backoff).
		_ = d.Nack(false, false)
		return
	}

	if err := d.Ack(false); err != nil {
		log.Error("gagal ack pesan", "queue", c.Queue, "event_id", msg.ID, "error", err)
	}
}

// maxAttempts adalah batas percobaan sebelum pesan diparkir di DLQ.
// Konsisten dengan batas retry relay (5) agar perilakunya dapat dijelaskan
// dengan satu kalimat: "lima percobaan, lalu DLQ".
const maxAttempts = 5

// parseMessage mengubah delivery menjadi Message.
func parseMessage(d amqp.Delivery) Message {
	msg := Message{
		RoutingKey: d.RoutingKey,
		Body:       d.Body,
		Headers:    d.Headers,
		Attempt:    headerInt(d.Headers, "attempt"),
		Type:       headerString(d.Headers, "event_type"),
	}
	msg.ID = headerString(d.Headers, "event_id")
	if msg.ID == "" {
		// Fallback deterministik: kombinasi routing key + MessageId broker.
		if d.MessageId != "" {
			msg.ID = d.RoutingKey + ":" + d.MessageId
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(d.Body, &payload); err == nil {
		msg.Payload = payload
		if msg.Type == "" {
			if t, ok := payload["event_type"].(string); ok {
				msg.Type = t
			}
		}
	}
	return msg
}

func headerString(h amqp.Table, key string) string {
	if h == nil {
		return ""
	}
	if v, ok := h[key].(string); ok {
		return v
	}
	return ""
}

func headerInt(h amqp.Table, key string) int {
	if h == nil {
		return 0
	}
	switch v := h[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// RetryDelayFor mengembalikan jeda backoff untuk percobaan tertentu sesuai
// konfigurasi broker. Dipakai consumer untuk memberi tahu pemanggil kapan
// pesan akan muncul kembali.
func RetryDelayFor(cfg broker.Config, attempt int) (time.Duration, bool) {
	return cfg.RetryDelay(attempt)
}
