// Command worker menjalankan background worker Angkut.
//
// Tanggung jawab (bertumbuh per fase):
//   - konsumsi event dari RabbitMQ (outbox relay, tracking sync, COD, AI analysis)
//   - scheduler: low stock, daily briefing, cleanup artefak
//
// Phase 1: proses hidup dengan health endpoint ringan + siap menerima consumer.
// Worker sengaja TIDAK menyentuh HTTP publik; hanya /healthz & /metrics pada
// port internal untuk probe container.
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

	// Consumer & scheduler didaftarkan pada fase berikutnya. Selama itu, worker
	// tetap menyediakan probe agar deployment identik bentuknya sejak awal.
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
	case sig := <-stop:
		log.Info("shutdown diminta", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown worker gagal", "error", err)
	}
	// TODO(Phase 3+): tutup channel MQ, tunggu in-flight task selesai (drain).
	log.Info("angkut-worker berhenti dengan rapi")
	return nil
}