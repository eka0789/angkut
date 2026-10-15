package webhookapp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/application/webhookapp"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/webhook"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// testCipherKey adalah kunci 32-byte base64 untuk uji (bukan kunci produksi).
const testCipherKey = "ZGV2LW9ubHktZW5jcnlwdGlvbi1rZXktMzJieXRlcyE="

// setupIngest menyiapkan database + service webhook, dilewati bila DB tidak ada.
func setupIngest(t *testing.T) (*webhookapp.Service, *pgxpool.Pool, string) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL kosong — lewati uji integrasi webhook")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("membuka pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("Postgres tidak terjangkau (%v) — lewati", err)
	}
	t.Cleanup(pool.Close)

	stores := database.NewStores(pool)
	cipher, err := cryptox.NewCipher(testCipherKey)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}

	provider := "uji" + uuid.NewString()[:8]
	secrets := database.NewWebhookSecretRepo(stores.Querier(), cipher)
	if err := secrets.Upsert(ctx, provider, "rahasia-uji", webhook.AlgoSHA256, "X-Signature"); err != nil {
		t.Fatalf("menyimpan rahasia: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM webhook_secrets WHERE provider=$1`, provider)
		_, _ = pool.Exec(ctx, `DELETE FROM webhook_events WHERE provider=$1`, provider)
	})

	svc := webhookapp.New(webhookapp.Deps{
		Tx:      database.NewTxManager(pool),
		Stores:  stores,
		Events:  stores.Webhooks(),
		Secrets: secrets,
		Audit:   stores.Audit(),
	})
	return svc, pool, provider
}

func TestIngestStoresEventWithValidSignature(t *testing.T) {
	svc, pool, provider := setupIngest(t)
	ctx := context.Background()

	body := []byte(`{"awb":"ITG-1","status":"delivered","event_id":"itg-evt-1"}`)
	sig := webhookSign(body, "rahasia-uji")

	res, err := svc.Ingest(ctx, webhook.IngestInput{
		Provider: provider, RawBody: body, ContentType: "application/json", SourceIP: "127.0.0.1",
	}, sig)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.Duplicate {
		t.Error("event baru tidak boleh ditandai duplikat")
	}
	if res.Event.ID == "" {
		t.Error("id event wajib terisi")
	}
	if !res.Event.SignatureValid {
		t.Error("signature_valid harus true")
	}
	if res.Event.AWB != "ITG-1" {
		t.Errorf("awb terekstraksi = %q, ingin ITG-1", res.Event.AWB)
	}

	// Event tersimpan dan penerusannya masuk outbox (satu transaksi).
	var stored int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM webhook_events WHERE provider=$1`, provider).Scan(&stored); err != nil {
		t.Fatalf("menghitung event: %v", err)
	}
	if stored != 1 {
		t.Errorf("event tersimpan = %d, ingin 1", stored)
	}

	var outbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events
		  WHERE event_type='tracking.event.received' AND aggregate_id=$1::uuid`,
		res.Event.ID).Scan(&outbox); err != nil {
		t.Fatalf("menghitung outbox: %v", err)
	}
	if outbox != 1 {
		t.Errorf("baris outbox = %d, ingin 1 (event harus diteruskan ke relay)", outbox)
	}
}

func TestIngestRejectsInvalidSignature(t *testing.T) {
	svc, pool, provider := setupIngest(t)
	ctx := context.Background()

	body := []byte(`{"awb":"ITG-2","event_id":"itg-evt-2"}`)
	_, err := svc.Ingest(ctx, webhook.IngestInput{
		Provider: provider, RawBody: body,
	}, "signature-palsu")
	if err == nil {
		t.Fatal("signature tidak sah harus ditolak")
	}
	if !shared.Is(err, shared.ErrForbidden) {
		t.Errorf("error = %v, ingin Forbidden", err)
	}

	// Payload TIDAK boleh diteruskan ke outbox walau tersimpan sebagai bukti.
	var outbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE aggregate_id IN
		   (SELECT id FROM webhook_events WHERE provider=$1)`, provider).Scan(&outbox); err != nil {
		t.Fatalf("menghitung outbox: %v", err)
	}
	if outbox != 0 {
		t.Errorf("baris outbox = %d, ingin 0 (payload tak terverifikasi tidak diteruskan)", outbox)
	}

	// Tetapi buktinya ADA: event tercatat dengan signature_valid=false.
	var stored int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM webhook_events WHERE provider=$1 AND NOT signature_valid`,
		provider).Scan(&stored); err != nil {
		t.Fatalf("menghitung event: %v", err)
	}
	if stored != 1 {
		t.Errorf("event bukti tersimpan = %d, ingin 1", stored)
	}
}

func TestIngestIsIdempotentForRepeatedEventID(t *testing.T) {
	// GATE Phase 4: webhook duplikat diproses sekali.
	svc, pool, provider := setupIngest(t)
	ctx := context.Background()

	body := []byte(`{"awb":"ITG-3","status":"delivered","event_id":"itg-evt-dup"}`)
	sig := webhookSign(body, "rahasia-uji")

	first, err := svc.Ingest(ctx, webhook.IngestInput{Provider: provider, RawBody: body}, sig)
	if err != nil {
		t.Fatalf("Ingest pertama: %v", err)
	}
	if first.Duplicate {
		t.Error("penerimaan pertama bukan duplikat")
	}

	// Provider mengirim ulang event yang sama.
	second, err := svc.Ingest(ctx, webhook.IngestInput{Provider: provider, RawBody: body}, sig)
	if err != nil {
		t.Fatalf("Ingest kedua (duplikat) tidak boleh error: %v", err)
	}
	if !second.Duplicate {
		t.Error("penerimaan kedua harus ditandai duplikat")
	}

	// Hanya SATU baris tersimpan, dan hanya SATU penerusan ke outbox.
	var events, outbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM webhook_events WHERE provider=$1`, provider).Scan(&events); err != nil {
		t.Fatalf("menghitung event: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE aggregate_id=$1::uuid`,
		first.Event.ID).Scan(&outbox); err != nil {
		t.Fatalf("menghitung outbox: %v", err)
	}
	if events != 1 {
		t.Errorf("event tersimpan = %d, ingin 1", events)
	}
	if outbox != 1 {
		t.Errorf("outbox = %d, ingin 1 — duplikat tidak boleh menerbitkan event kedua", outbox)
	}
}

func TestIngestRejectsProviderWithoutSecret(t *testing.T) {
	svc, _, _ := setupIngest(t)
	ctx := context.Background()

	_, err := svc.Ingest(ctx, webhook.IngestInput{
		Provider: "provider-tanpa-rahasia-" + uuid.NewString()[:6],
		RawBody:  []byte(`{"awb":"X"}`),
	}, "apa saja")
	if err == nil {
		t.Fatal("provider tanpa rahasia harus GAGAL, bukan diterima")
	}
	if !errors.Is(err, webhook.ErrSecretless) {
		t.Errorf("error = %v, ingin ErrSecretless", err)
	}
}

func TestIngestRejectsEmptyBody(t *testing.T) {
	svc, _, provider := setupIngest(t)
	ctx := context.Background()

	_, err := svc.Ingest(ctx, webhook.IngestInput{Provider: provider}, "x")
	if err == nil {
		t.Fatal("body kosong harus ditolak")
	}
}

func TestIngestSurvivesNonJSONBody(t *testing.T) {
	// Kurir kadang mengirim body yang bukan JSON. Payload mentah tetap harus
	// tersimpan (raw body adalah sumber kebenaran) dan tidak boleh 500.
	svc, pool, provider := setupIngest(t)
	ctx := context.Background()

	body := []byte(`awb=ITG-RAW&status=picked_up`)
	sig := webhookSign(body, "rahasia-uji")

	res, err := svc.Ingest(ctx, webhook.IngestInput{
		Provider: provider, RawBody: body, ContentType: "application/x-www-form-urlencoded",
	}, sig)
	if err != nil {
		t.Fatalf("Ingest body non-JSON: %v", err)
	}
	if res.Event == nil {
		t.Fatal("event wajib terbentuk")
	}

	// Raw body tersimpan apa adanya, sehingga bisa diproses ulang nanti.
	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT raw_body FROM webhook_events WHERE id=$1::uuid`, res.Event.ID).Scan(&raw); err != nil {
		t.Fatalf("membaca raw_body: %v", err)
	}
	if string(raw) != string(body) {
		t.Errorf("raw_body = %q, ingin %q", raw, body)
	}
}

// webhookSign menghitung HMAC-SHA256 heksadesimal untuk body — sama dengan
// yang dihitung provider saat mengirim.
func webhookSign(body []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}
