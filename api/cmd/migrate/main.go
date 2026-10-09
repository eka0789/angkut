// Command migrate menjalankan migrasi database.
//
//	go run ./api/cmd/migrate up
//	go run ./api/cmd/migrate down [n]
//	go run ./api/cmd/migrate status
//
// DATABASE_URL diambil dari environment (lihat apps/.env.example).
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/migrate"
	"github.com/angkut/angkut/apps/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DB.URL == "" {
		return fmt.Errorf("DATABASE_URL belum diisi (lihat apps/.env.example)")
	}

	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("tidak dapat terhubung ke database: %w", err)
	}

	runner, err := migrate.New(pool, migrations.FS)
	if err != nil {
		return err
	}

	switch cmd {
	case "up":
		n, err := runner.Up(ctx)
		if err != nil {
			return err
		}
		current, _ := runner.Current(ctx)
		fmt.Printf("migrate up: %d migrasi diterapkan (versi sekarang: %04d)\n", n, current)
	case "down":
		n := 1
		if len(os.Args) > 2 {
			parsed, err := strconv.Atoi(os.Args[2])
			if err != nil {
				return fmt.Errorf("argumen n tidak valid: %q", os.Args[2])
			}
			n = parsed
		}
		reverted, err := runner.Down(ctx, n)
		if err != nil {
			return err
		}
		current, _ := runner.Current(ctx)
		fmt.Printf("migrate down: %d migrasi dibatalkan (versi sekarang: %04d)\n", reverted, current)
	case "status":
		current, err := runner.Current(ctx)
		if err != nil {
			return err
		}
		pending, err := runner.Pending(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("versi sekarang: %04d\n", current)
		fmt.Printf("belum diterapkan: %v\n", pending)
	default:
		return fmt.Errorf("perintah tidak dikenal %q — pakai: up | down [n] | status", cmd)
	}
	return nil
}