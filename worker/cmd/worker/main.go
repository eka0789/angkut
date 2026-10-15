// Command worker menjalankan background worker Angkut.
//
// Tanggung jawab (bertumbuh per fase):
//   - outbox relay: memindahkan event dari tabel outbox ke RabbitMQ (ADR-005)
//   - konsumsi event: webhook-processor, tracking-sync, notifikasi (Phase 4)
//   - scheduler: low stock, daily briefing, cleanup artefak (fase berikutnya)
//
// Worker sengaja TIDAK menyentuh HTTP publik; hanya /healthz & /metrics pada
// port internal untuk probe container. Worker juga tidak mengimpor
// `api/internal/interfaces` — batas itu dijaga dengan menaruh domain,
// application, dan infrastructure di `apps/internal/` yang dipakai bersama.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/observability"
	"github.com/angkut/angkut/apps/internal/version"
)

// workerMetricsPort adalah port probe internal (tidak diekspos ke internet).
const workerMetricsPort = "9090"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := observability.New(cfg.LogLevel, cfg.Env)
	log.Info("angkut-worker starting", "version", version.String(), "env", cfg.Env)

	metrics := observability.NewRegistry()

	// ── Database ─────────────────────────────────────────────────────────
	// Worker membutuhkan DB untuk outbox, tracking, dan dedup event. Tanpa DB
	// ia tidak bisa bekerja; di dev kita lanjut tanpa consumer agar perintah
	// `make worker` tetap berguna untuk memeriksa probe.
	var pool *pgxpool.Pool
	if cfg.DB.URL != "" {
		p, err := openPool(context.Background(), cfg)
		if err != nil {
			if cfg.IsProduction() {
				return fmt.Errorf("membuka postgres: %w", err)
			}
			log.Warn("postgres tidak tersedia — consumer tidak dijalankan (mode dev)", "error", err)
		} else {
			pool = p
			defer pool.Close()
			log.Info("postgres terhubung")
		}
	} else if cfg.IsProduction() {
		return errors.New("DATABASE_URL wajib di production")
	}

	// Konteks proses: dibatalkan saat sinyal berhenti, dipakai semua runner.
	procCtx, procCancel := context.WithCancel(context.Background())
	defer procCancel()

	// ── Consumer & relay ────────────────────────────────────────────────
	var runners []func(context.Context) error
	if pool != nil {
		r, err := buildRuntime(procCtx, cfg, log, pool)
		if err != nil {
			return err
		}
		runners = r
	}
	log.Info("komponen worker siap", "jumlah", len(runners))

	runtimeErr := make(chan error, 1)
	if len(runners) > 0 {
		go func() { runtimeErr <- runAll(procCtx, log, runners) }()
	}

	// ── Probe internal ──────────────────────────────────────────────────
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/metrics", metrics.Handler())

	srv := &http.Server{
		Addr:              ":" + workerMetricsPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("worker probe listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("worker probe: %w", err)
	case err := <-runtimeErr:
		if err != nil {
			// Satu komponen bermasalah → worker dimatikan agar kegagalan
			// terlihat, bukan setengah hidup yang membingungkan operator.
			return fmt.Errorf("runtime worker: %w", err)
		}
		log.Warn("sebuah runner worker berhenti — worker dihentikan")
	case sig := <-stop:
		log.Info("shutdown diminta", "signal", sig.String())
	}

	// Hentikan runner lebih dulu agar pesan yang sedang diproses selesai
	// (ack/nack) sebelum koneksi ditutup.
	procCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown worker gagal", "error", err)
	}
	log.Info("angkut-worker berhenti dengan rapi")
	return nil
}

// openPool membuat pool Postgres dengan batas yang wajar untuk worker.
// Worker memakai koneksi lebih sedikit daripada API (bukan peladen request).
func openPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DB.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}
	poolCfg.MaxConns = cfg.DB.MaxConns
	poolCfg.MinConns = cfg.DB.MinConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("membuat pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
