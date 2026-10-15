// Command setsecret menyimpan rahasia verifikasi webhook untuk satu provider.
//
// Ada karena endpoint admin pengelola rahasia webhook belum dibuat (Phase 7).
// Rahasia selalu ditulis TERENKRIPSI — perintah ini tidak punya jalur menulis
// plaintext, sehingga tidak ada cara "salah" yang bisa dipakai keliru.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/domain/webhook"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

func main() {
	provider := flag.String("provider", "", "nama provider (mis. jne)")
	secret := flag.String("secret", "", "rahasia HMAC (dibaca dari stdin bila kosong)")
	algo := flag.String("algo", "hmac-sha256", "algoritma: hmac-sha256 | hmac-sha1")
	header := flag.String("header", "X-Signature", "nama header signature")
	flag.Parse()

	if *provider == "" {
		fmt.Fprintln(os.Stderr, "wajib: -provider")
		os.Exit(2)
	}
	value := *secret
	if value == "" {
		fmt.Fprintln(os.Stderr, "wajib: -secret (atau lewat stdin)")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "konfigurasi: %v\n", err)
		os.Exit(1)
	}
	if cfg.DB.URL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL wajib diisi")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DB.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	cipher, err := cryptox.NewCipher(cfg.Auth.SecretsKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cipher: %v\n", err)
		os.Exit(1)
	}

	repo := database.NewWebhookSecretRepo(database.NewStores(pool).Querier(), cipher)
	if err := repo.Upsert(ctx, *provider, value, webhook.Algo(*algo), *header); err != nil {
		fmt.Fprintf(os.Stderr, "menyimpan: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("rahasia webhook untuk %q tersimpan (terenkripsi, algo=%s, header=%s)\n",
		*provider, *algo, *header)
}
