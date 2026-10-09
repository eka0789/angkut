// Command api menjalankan HTTP API Angkut.
//
// Wiring mengikuti dev-docs/architecture/backend-structure.md:
// config → logger → observability → db/redis → adapters → services →
// handlers → router → server → graceful shutdown.
//
// Phase 1 hanya menyambungkan komponen yang sudah ada. Dependensi eksternal
// (Postgres/Redis) bersifat opsional di dev: bila URL kosong atau tak
// terjangkau, API tetap menyala agar `make api` langsung berguna tanpa Docker.
// Di production, dependensi wajib dan kegagalan = berhenti.
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

	"github.com/angkut/angkut/apps/api/internal/infrastructure/health"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/observability"
	"github.com/angkut/angkut/apps/internal/version"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
	log.Info("angkut-api starting",
		"version", version.String(), "env", cfg.Env, "port", cfg.API.Port)

	metrics := observability.NewRegistry()

	checkers := make([]handler.Checker, 0, 2)
	closers := make([]func(context.Context) error, 0, 3)

	// ── Postgres ─────────────────────────────────────────────────────────
	// pool dideklarasikan di luar blok agar dapat diteruskan ke perakit router.
	var pool *pgxpool.Pool
	if cfg.DB.URL != "" {
		p, err := openPostgres(context.Background(), cfg)
		if err != nil {
			if cfg.IsProduction() {
				return fmt.Errorf("membuka postgres: %w", err)
			}
			log.Warn("postgres tidak tersedia — lanjut tanpa DB (mode dev)", "error", err)
		} else {
			pool = p
			checkers = append(checkers, health.New("postgres", pool))
			closers = append(closers, func(context.Context) error { pool.Close(); return nil })
			log.Info("postgres terhubung")
		}
	} else if cfg.IsProduction() {
		return errors.New("DATABASE_URL wajib di production")
	}

	// ── Redis ────────────────────────────────────────────────────────────
	if cfg.RDB.URL != "" {
		rdb, err := openRedis(context.Background(), cfg)
		if err != nil {
			if cfg.IsProduction() {
				return fmt.Errorf("membuka redis: %w", err)
			}
			log.Warn("redis tidak tersedia — lanjut tanpa cache (mode dev)", "error", err)
		} else {
			checkers = append(checkers, health.New("redis", redisPinger{rdb}))
			closers = append(closers, func(context.Context) error { return rdb.Close() })
			log.Info("redis terhubung")
		}
	}

	// ── Router & server ──────────────────────────────────────────────────
	router, err := buildRouter(cfg, log, pool, metrics, checkers)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:         ":" + cfg.API.Port,
		Handler:      router,
		ReadTimeout:  cfg.API.ReadTimeout,
		WriteTimeout: cfg.API.WriteTimeout,
		IdleTimeout:  cfg.API.IdleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case sig := <-stop:
		log.Info("shutdown diminta", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.API.ShutdownGrace)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown http gagal", "error", err)
	}
	for _, closeFn := range closers {
		if err := closeFn(ctx); err != nil {
			log.Warn("menutup dependensi gagal", "error", err)
		}
	}
	log.Info("angkut-api berhenti dengan rapi")
	return nil
}
