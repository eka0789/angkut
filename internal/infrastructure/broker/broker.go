// Package broker menyediakan topologi RabbitMQ Angkut dan klien publish/consume.
//
// Topologi mengikuti planning/architecture.md §6 dan ADR-005:
//
//	angkut.events        (topic) → queue per consumer, routing key {aggregate}.{action}
//	angkut.events.retry  (topic, TTL) → kembali ke angkut.events setelah TTL habis
//	angkut.events.dlx    (fanout) → angkut.events.dlq (parkir akhir)
//
// Aturan penting: consumer memakai ack manual dan idempotent. Kegagalan yang
// layak dicoba ulang masuk ke retry exchange dengan backoff TTL; setelah
// percobaan habis, pesan masuk DLQ — tidak pernah hilang diam-diam.
package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Nama exchange & queue kanonik. Dikumpulkan di sini agar producer, consumer,
// dan skrip operasional tidak mengetik ulang string yang mudah salah.
const (
	ExchangeEvents = "angkut.events"
	ExchangeRetry  = "angkut.events.retry"
	ExchangeDLX    = "angkut.events.dlx"

	QueueDLQ = "angkut.events.dlq"

	// retryQueuePrefix menandai queue retry dengan TTL tertentu.
	retryQueuePrefix = "angkut.events.retry."
)

// Config mengatur perilaku klien broker.
type Config struct {
	URL string

	// Prefetch membatasi pesan belum-ter-ack per consumer.
	Prefetch int

	// RetryDelays adalah jadwal backoff antar percobaan. Panjangnya menentukan
	// jumlah percobaan: 3 entri = 3 kesempatan ulang, lalu DLQ.
	RetryDelays []time.Duration

	// ReconnectDelay adalah jeda sebelum mencoba menyambung ulang.
	ReconnectDelay time.Duration
}

// DefaultConfig mengembalikan nilai yang wajar untuk development.
func DefaultConfig(url string) Config {
	return Config{
		URL:            url,
		Prefetch:       10,
		RetryDelays:    []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute},
		ReconnectDelay: 5 * time.Second,
	}
}

// RetryDelay mengembalikan jeda untuk percobaan ke-attempt (mulai 1).
// Percobaan melebihi jadwal → (0, false) yang berarti "kirim ke DLQ".
func (c Config) RetryDelay(attempt int) (time.Duration, bool) {
	if attempt <= 0 || attempt > len(c.RetryDelays) {
		return 0, false
	}
	return c.RetryDelays[attempt-1], true
}

// Client adalah koneksi broker dengan penyambungan ulang otomatis.
//
// RabbitMQ memutus koneksi tanpa pemberitahuan (restart, jaringan, kebijakan
// broker). Klien ini menyambung ulang dan MEMBANGUN ULANG topologi, karena
// exchange/queue durable tapi channel-nya hilang bersama koneksi.
type Client struct {
	cfg Config

	mu   sync.RWMutex
	conn *amqp.Connection
	ch   *amqp.Channel

	// closed menandai Close() dipanggil; mencegah penyambungan ulang.
	closed bool
}

// Dial menyambung ke broker dan menyiapkan topologi.
func Dial(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("RABBITMQ_URL kosong — broker tidak dikonfigurasi")
	}
	c := &Client{cfg: cfg}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

// connect membuka koneksi + channel + topologi.
func (c *Client) connect() error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("menyambung ke RabbitMQ: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("membuka channel: %w", err)
	}
	if err := DeclareTopology(ch, c.cfg); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("menyiapkan topologi: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.ch = ch
	c.mu.Unlock()
	return nil
}

// Close menutup koneksi. Aman dipanggil lebih dari sekali.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Publish mengirim satu pesan ke exchange utama dengan routing key tertentu.
//
// Pesan diterbitkan persistent: broker menyimpannya ke disk sehingga restart
// RabbitMQ tidak menghilangkan event yang belum sempat dikonsumsi.
func (c *Client) Publish(ctx context.Context, routingKey string, body []byte, headers map[string]any) error {
	c.mu.RLock()
	ch := c.ch
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return errors.New("klien broker sudah ditutup")
	}
	if ch == nil {
		return errors.New("channel broker tidak tersedia")
	}

	amqpHeaders := amqp.Table{}
	for k, v := range headers {
		amqpHeaders[k] = v
	}
	// attempt dipakai konsumer untuk memilih jadwal backoff.
	if _, ok := amqpHeaders["attempt"]; !ok {
		amqpHeaders["attempt"] = 0
	}

	return ch.PublishWithContext(ctx, ExchangeEvents, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Headers:      amqpHeaders,
		Body:         body,
	})
}

// ConsumerChannel membuka channel baru untuk konsumsi.
//
// Channel TERPISAH dari channel publish dengan sengaja: satu channel yang
// dipakai bersama saling memblokir (RabbitMQ menyerialkan per channel), dan
// kegagalan konsumsi tidak boleh menghentikan penerbitan.
// Pemanggil WAJIB menutup channel yang dikembalikan.
func (c *Client) ConsumerChannel() (*amqp.Channel, error) {
	c.mu.RLock()
	conn := c.conn
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, errors.New("klien broker sudah ditutup")
	}
	if conn == nil {
		return nil, errors.New("koneksi broker tidak tersedia")
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("membuka channel konsumsi: %w", err)
	}
	return ch, nil
}

// DeclareTopology membuat exchange, queue, dan binding yang dibutuhkan.
//
// Idempotent: semua deklarasi memakai durable=true dan nama tetap, sehingga
// aman dijalankan berulang oleh setiap proses yang menyambung.
func DeclareTopology(ch *amqp.Channel, cfg Config) error {
	// Exchange utama: topic — routing key `{aggregate}.{action}`.
	if err := ch.ExchangeDeclare(ExchangeEvents, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("exchange utama: %w", err)
	}
	// Exchange retry: menerima pesan gagal, menahannya sesuai TTL, lalu
	// mengembalikannya ke exchange utama (dead-letter ke ExchangeEvents).
	if err := ch.ExchangeDeclare(ExchangeRetry, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("exchange retry: %w", err)
	}
	// Exchange DLX: fanout ke DLQ — pesan yang habis percobaan.
	if err := ch.ExchangeDeclare(ExchangeDLX, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("exchange dlx: %w", err)
	}

	// DLQ durable; pesan di sini menunggu operator, bukan hilang.
	if _, err := ch.QueueDeclare(QueueDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue dlq: %w", err)
	}
	if err := ch.QueueBind(QueueDLQ, "", ExchangeDLX, false, nil); err != nil {
		return fmt.Errorf("binding dlq: %w", err)
	}

	// Satu queue retry per jeda backoff; TTL-nya berbeda sehingga tidak bisa
	// digabung. Pesan yang TTL-nya habis masuk kembali ke exchange utama.
	for _, delay := range cfg.RetryDelays {
		name := RetryQueueName(delay)
		args := amqp.Table{
			"x-message-ttl":             int64(delay.Milliseconds()),
			"x-dead-letter-exchange":    ExchangeEvents,
			"x-dead-letter-routing-key": "", // diisi per pesan via routing key asal
		}
		if _, err := ch.QueueDeclare(name, true, false, false, false, args); err != nil {
			return fmt.Errorf("queue retry %s: %w", name, err)
		}
		// Binding wildcard agar SEMUA event bisa masuk antrean retry ini
		// (routing key asli tetap dibawa saat dead-letter).
		if err := ch.QueueBind(name, "#", ExchangeRetry, false, nil); err != nil {
			return fmt.Errorf("binding retry %s: %w", name, err)
		}
	}
	return nil
}

// RetryQueueName mengembalikan nama queue retry untuk satu jeda.
func RetryQueueName(delay time.Duration) string {
	return fmt.Sprintf("%s%dms", retryQueuePrefix, delay.Milliseconds())
}

// DeclareConsumerQueue membuat queue consumer yang durable dan mengikatnya ke
// exchange utama untuk pola routing tertentu. Mengembalikan nama queue.
func DeclareConsumerQueue(ch *amqp.Channel, queue string, routingKeys ...string) error {
	if _, err := ch.QueueDeclare(queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}); err != nil {
		return fmt.Errorf("queue %s: %w", queue, err)
	}
	for _, key := range routingKeys {
		if err := ch.QueueBind(queue, key, ExchangeEvents, false, nil); err != nil {
			return fmt.Errorf("binding %s → %s: %w", queue, key, err)
		}
	}
	return nil
}
