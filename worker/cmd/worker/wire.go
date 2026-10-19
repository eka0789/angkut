package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/application/trackingapp"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/infrastructure/broker"
	"github.com/angkut/angkut/apps/internal/infrastructure/consumer"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/infrastructure/outboxrelay"
	"github.com/angkut/angkut/apps/internal/realtime"
	workerhandler "github.com/angkut/angkut/apps/worker/internal/handler"
	"github.com/redis/go-redis/v9"
)

// Nama queue consumer. Satu queue per jenis pekerjaan agar kegagalan pada satu
// consumer tidak menyumbat yang lain, dan agar scaling dapat dilakukan per
// consumer (mis. menambah replika khusus tracking).
const (
	queueWebhookProcessor = "angkut.webhook-processor"
	queueTrackingSync     = "angkut.tracking-sync"
	queueNotification     = "angkut.notification"
)

// buildRuntime merakit consumer + relay yang siap dijalankan.
//
// Mengembalikan daftar fungsi "run" agar main hanya mengurus siklus hidup
// (start/stop), bukan detail wiring.
func buildRuntime(ctx context.Context, cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool, rdb redis.UniversalClient) ([]func(context.Context) error, error) {
	stores := database.NewStores(pool)
	tx := database.NewTxManager(pool)

	// Notifier realtime: peristiwa yang diproses worker (webhook → tracking)
	// ikut disiarkan ke klien SSE/WS lewat Redis pub/sub. Tanpa Redis, publish
	// menjadi no-op — data tetap benar, hanya tanpa pembaruan hidup.
	trackingSvc := trackingapp.New(trackingapp.Deps{
		Tx:       tx,
		Stores:   stores,
		Events:   stores.Tracking(),
		Audit:    stores.Audit(),
		Log:      log,
		Notifier: realtime.NewRedisPublisher(rdb),
	})

	// ── Consumer ─────────────────────────────────────────────────────────
	webhookProcessor := &workerhandler.WebhookProcessor{
		Webhooks:  stores.Webhooks(),
		Tracking:  trackingSvc,
		Shipments: stores.Tracking(),
		Log:       log,
	}
	trackingSync := &workerhandler.TrackingSyncHandler{Tracking: trackingSvc, Log: log}

	consumers := []*consumer.Consumer{
		{
			Queue:   queueWebhookProcessor,
			Keys:    []string{"tracking.event.received"},
			Handler: webhookProcessor.Handle,
			Deduper: stores.ProcessedEvents(),
			Log:     log,
		},
		{
			Queue: queueTrackingSync,
			Keys: []string{
				"shipment.created", "shipment.updated", "shipment.picked_up",
				"shipment.in_transit", "shipment.delivered", "shipment.failed",
				"shipment.returned", "shipment.cancelled",
			},
			Handler: trackingSync.Handle,
			Deduper: stores.ProcessedEvents(),
			Log:     log,
		},
		{
			// Notifikasi menyusul (TASK-040); handler sementara mencatat ke log
			// agar pesan tidak menumpuk di queue dan jejaknya tetap terlihat.
			Queue:   queueNotification,
			Keys:    []string{"notification.send"},
			Handler: (&workerhandler.ShipmentEventLogger{Log: log}).Handle,
			Deduper: stores.ProcessedEvents(),
			Log:     log,
		},
	}

	runners := make([]func(context.Context) error, 0, len(consumers)+1)

	if !cfg.MQ.Enabled || cfg.MQ.URL == "" {
		// Tanpa broker, worker tetap hidup: relay akan melaporkan bahwa broker
		// tidak dikonfigurasi dan consumer tidak dijalankan. Ini disengaja agar
		// `make worker` berguna di dev tanpa RabbitMQ.
		log.Warn("rabbitmq tidak aktif — consumer tidak dijalankan",
			"RABBITMQ_ENABLED", cfg.MQ.Enabled, "url_kosong", cfg.MQ.URL == "")
		return runners, nil
	}

	client, err := broker.Dial(broker.DefaultConfig(cfg.MQ.URL))
	if err != nil {
		return nil, fmt.Errorf("menyambung broker: %w", err)
	}

	for _, c := range consumers {
		c := c
		runners = append(runners, func(ctx context.Context) error {
			return c.Run(ctx, client)
		})
	}

	// ── Outbox relay ─────────────────────────────────────────────────────
	relay := outboxrelay.New(outboxrelay.DefaultConfig(), stores.Outbox(), client, log)
	runners = append(runners, relay.Run)

	return runners, nil
}

// runAll menjalankan semua runner secara paralel dan menunggu yang pertama
// berhenti. Bila satu komponen mati, seluruh worker dimatikan agar kegagalan
// terlihat (bukan setengah hidup yang membingungkan operator).
func runAll(ctx context.Context, log *slog.Logger, runners []func(context.Context) error) error {
	if len(runners) == 0 {
		<-ctx.Done()
		return nil
	}

	errCh := make(chan error, len(runners))
	for i, run := range runners {
		i, run := i, run
		go func() {
			if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("runner %d: %w", i, err)
				return
			}
			errCh <- nil
		}()
	}

	// Tunggu yang pertama selesai/bermasalah.
	err := <-errCh
	if err != nil {
		return err
	}
	// Satu runner berhenti tanpa error berarti worker berhenti normal.
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}
