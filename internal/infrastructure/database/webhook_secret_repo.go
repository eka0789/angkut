package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/webhook"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
)

// WebhookSecretRepo menyimpan rahasia verifikasi signature per provider.
//
// Rahasia disimpan TERENKRIPSI (AES-GCM lewat cryptox). Membaca nilai terbuka
// hanya terjadi di SecretFor — satu-satunya jalur, sehingga tidak ada tempat
// lain yang bisa menyentuh nilai mentah.
type WebhookSecretRepo struct {
	q      Querier
	cipher *cryptox.Cipher
}

// NewWebhookSecretRepo membuat repository rahasia webhook.
func NewWebhookSecretRepo(q Querier, cipher *cryptox.Cipher) *WebhookSecretRepo {
	return &WebhookSecretRepo{q: q, cipher: cipher}
}

// Upsert menyimpan/mengganti rahasia satu provider.
func (r *WebhookSecretRepo) Upsert(ctx context.Context, provider, secret string, algo webhook.Algo, header string) error {
	if r.cipher == nil {
		return errors.New("cipher belum dikonfigurasi — rahasia webhook tidak dapat disimpan")
	}
	enc, err := r.cipher.Encrypt([]byte(secret))
	if err != nil {
		return err
	}
	if algo == "" {
		algo = webhook.AlgoSHA256
	}
	if header == "" {
		header = "X-Signature"
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO webhook_secrets (id, provider, secret_enc, signature_header, algo, is_active)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, true)
		ON CONFLICT (provider) DO UPDATE
		   SET secret_enc = EXCLUDED.secret_enc,
		       signature_header = EXCLUDED.signature_header,
		       algo = EXCLUDED.algo,
		       is_active = true`,
		provider, enc, header, string(algo))
	return err
}

// SecretFor membuka rahasia provider untuk keperluan verifikasi signature.
func (r *WebhookSecretRepo) SecretFor(ctx context.Context, provider string) (string, webhook.Algo, string, error) {
	var (
		enc    []byte
		algo   string
		header string
	)
	err := r.q.QueryRow(ctx, `
		SELECT secret_enc, algo, signature_header
		  FROM webhook_secrets WHERE provider=$1 AND is_active`, provider).Scan(&enc, &algo, &header)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", webhook.ErrSecretless
	}
	if err != nil {
		return "", "", "", err
	}
	if r.cipher == nil {
		return "", "", "", errors.New("cipher belum dikonfigurasi — rahasia webhook tidak dapat dibuka")
	}
	plain, err := r.cipher.Decrypt(enc)
	if err != nil {
		return "", "", "", err
	}
	return string(plain), webhook.Algo(algo), header, nil
}
