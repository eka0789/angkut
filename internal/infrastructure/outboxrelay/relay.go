// Package outboxrelay menjalankan sisi "relay" dari pola transactional outbox
// (ADR-005): membaca baris pending di `outbox_events` dan menerbitkannya ke
// RabbitMQ.
//
// Sifat yang dijaga:
//   - Aman dijalankan beberapa replika: klaim memakai FOR UPDATE SKIP LOCKED.
//   - Tidak kehilangan event: baris hanya ditandai published SETELAH broker
//     mengonfirmasi; kegagalan mengembalikannya ke pending dengan backoff.
//   - Pulih dari crash: klaim basi dikembalikan ke pending secara berkala.
package outboxrelay

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/event"
)

// Publisher adalah sisi broker yang dibutuhkan relay.
type Publisher interface {
	Publish(ctx context.Context, routingKey string, body []byte, headers map[string]any) error
}

// Repository adalah sisi database yang dibutuhkan relay.
type Repository interface {
	Claim(ctx context.Context, limit int) ([]event.Record, error)
	MarkPublished(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, reason string, retryAfterSeconds int) error
	RecoverStaleClaims(ctx context.Context, olderThan time.Duration) (int64, error)
}

// Config mengatur perilaku relay.
type Config struct {
	// BatchSize adalah jumlah baris yang diklaim per putaran.
	BatchSize int
	// Interval adalah jeda antar putaran saat tidak ada pekerjaan.
	Interval time.Duration
	// StaleAfter adalah usia klaim yang dianggap ditinggalkan (relay mati).
	StaleAfter time.Duration
	// BackoffSeconds adalah jeda sebelum baris gagal dicoba lagi.
	BackoffSeconds int
}

// DefaultConfig mengembalikan nilai yang wajar untuk development.
func DefaultConfig() Config {
	return Config{
		BatchSize:      50,
		Interval:       time.Second,
		StaleAfter:     2 * time.Minute,
		BackoffSeconds: 30,
	}
}

// Relay memindahkan event dari outbox ke broker.
type Relay struct {
	cfg  Config
	repo Repository
	pub  Publisher
	log  *slog.Logger
}

// New membuat relay.
func New(cfg Config, repo Repository, pub Publisher, log *slog.Logger) *Relay {
	if log == nil {
		log = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultConfig().BatchSize
	}
	return &Relay{cfg: cfg, repo: repo, pub: pub, log: log}
}

// Run menjalankan relay sampai ctx dibatalkan.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	recoverTicker := time.NewTicker(r.cfg.StaleAfter)
	defer recoverTicker.Stop()

	r.log.Info("outbox relay mulai", "batch", r.cfg.BatchSize, "interval", r.cfg.Interval)

	for {
		select {
		case <-ctx.Done():
			r.log.Info("outbox relay berhenti")
			return ctx.Err()

		case <-recoverTicker.C:
			// Kembalikan klaim yang ditinggalkan relay yang mati.
			if n, err := r.repo.RecoverStaleClaims(ctx, r.cfg.StaleAfter); err != nil {
				r.log.Warn("gagal memulihkan klaim basi", "error", err)
			} else if n > 0 {
				r.log.Warn("klaim outbox basi dipulihkan", "jumlah", n)
			}

		case <-ticker.C:
			published, failed, err := r.drain(ctx)
			if err != nil {
				r.log.Error("putaran relay gagal", "error", err)
				continue
			}
			if published > 0 || failed > 0 {
				r.log.Info("outbox relay putaran selesai", "terbit", published, "gagal", failed)
			}
		}
	}
}

// drain memproses sampai tidak ada baris yang bisa diklaim lagi.
func (r *Relay) drain(ctx context.Context) (published, failed int, err error) {
	for {
		if ctx.Err() != nil {
			return published, failed, ctx.Err()
		}
		records, claimErr := r.repo.Claim(ctx, r.cfg.BatchSize)
		if claimErr != nil {
			return published, failed, claimErr
		}
		if len(records) == 0 {
			return published, failed, nil
		}
		for _, rec := range records {
			if pubErr := r.publishOne(ctx, rec); pubErr != nil {
				failed++
				continue
			}
			published++
		}
		// Batch penuh berarti kemungkinan masih ada lagi.
		if len(records) < r.cfg.BatchSize {
			return published, failed, nil
		}
	}
}

// publishOne menerbitkan satu baris. Kegagalan dicatat sebagai failed dengan
// backoff; baris TIDAK ditandai published sehingga tidak ada event hilang.
func (r *Relay) publishOne(ctx context.Context, rec event.Record) error {
	body, err := json.Marshal(map[string]any{
		"id":              rec.ID,
		"event_type":      string(rec.Type),
		"aggregate_type":  rec.AggregateType,
		"aggregate_id":    rec.AggregateID,
		"organization_id": rec.OrganizationID,
		"occurred_at":     rec.CreatedAt.UTC().Format(time.RFC3339Nano),
		"payload":         rec.Payload,
	})
	if err != nil {
		// Payload yang tidak bisa disandikan tidak akan pernah bisa dikirim —
		// tandai gagal permanen agar tidak berputar.
		_ = r.repo.MarkFailed(ctx, rec.ID, "payload tidak dapat disandikan: "+err.Error(), r.cfg.BackoffSeconds)
		return err
	}

	headers := map[string]any{}
	for k, v := range rec.Headers {
		headers[k] = v
	}
	headers["attempt"] = rec.Attempts

	if err := r.pub.Publish(ctx, rec.RoutingKey(), body, headers); err != nil {
		if markErr := r.repo.MarkFailed(ctx, rec.ID, err.Error(), r.cfg.BackoffSeconds); markErr != nil {
			r.log.Error("gagal menandai outbox sebagai failed", "id", rec.ID, "error", markErr)
		}
		r.log.Warn("penerbitan outbox gagal",
			"id", rec.ID, "event", string(rec.Type), "attempts", rec.Attempts, "error", err)
		return err
	}

	if err := r.repo.MarkPublished(ctx, rec.ID); err != nil {
		// Pesan SUDAH terkirim; menandai gagal akan menghasilkan duplikat.
		// Consumer idempotent (dedup event_id) yang menangani ini, dan baris
		// akan dicoba lagi — lebih baik duplikat daripada hilang.
		r.log.Error("event terkirim tetapi gagal menandai published",
			"id", rec.ID, "error", err)
		return err
	}
	return nil
}

// Publish merelai satu baris secara langsung (dipakai test & perintah manual).
func (r *Relay) Publish(ctx context.Context, rec event.Record) error {
	return r.publishOne(ctx, rec)
}

// ErrBrokerUnavailable menandai broker yang tidak dapat dijangkau. Relay
// memperlakukannya sama seperti kegagalan lain (retry), tetapi pemanggil dapat
// memakainya untuk memutuskan berhenti lebih awal.
var ErrBrokerUnavailable = errors.New("broker tidak tersedia")
