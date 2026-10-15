// Package webhookapp berisi penerimaan event dari pihak ketiga.
//
// Urutan yang dipertahankan ketat (planning/architecture.md §5):
//
//	verifikasi signature → simpan mentah → dedup → publish ke outbox
//
// Verifikasi terjadi SEBELUM apa pun disimpan. Event dengan signature tidak sah
// tetap dicatat sebagai bukti keamanan, tetapi tidak pernah diteruskan untuk
// diproses — jadi tidak ada jalur di mana payload tak terverifikasi mengubah
// status pengiriman.
package webhookapp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/event"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/webhook"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// maxRawBody membatasi ukuran body webhook agar tidak ada payload raksasa.
// Nilai ini kebijakan pertahanan, bukan parameter bisnis.
const maxRawBody = 1 << 20 // 1 MiB

// Deps adalah dependensi service webhook.
type Deps struct {
	Tx      *database.TxManager
	Stores  *database.Stores
	Events  webhook.Repository
	Secrets webhook.Secrets
	Audit   audit.Recorder
	Log     *slog.Logger
}

// Service adalah service penerimaan webhook.
type Service struct {
	tx      *database.TxManager
	stores  *database.Stores
	events  webhook.Repository
	secrets webhook.Secrets
	audit   audit.Recorder
	log     *slog.Logger
}

// New membuat service webhook.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, events: d.Events, secrets: d.Secrets, audit: d.Audit, log: log}
}

// Ingest menerima satu event webhook.
//
// Signature diverifikasi lebih dulu. Bila tidak sah, payload TIDAK diteruskan
// ke outbox — hanya dicatat sebagai event bermasalah untuk ditelusuri.
func (s *Service) Ingest(ctx context.Context, in webhook.IngestInput, signatureHeader string) (*webhook.Result, error) {
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		return nil, shared.Validation(map[string]string{"provider": "Provider wajib diisi."})
	}
	if len(in.RawBody) == 0 {
		return nil, shared.Validation(map[string]string{"body": "Body permintaan kosong."})
	}
	if len(in.RawBody) > maxRawBody {
		return nil, shared.Validation(map[string]string{"body": "Body melebihi batas ukuran yang diizinkan."})
	}

	// Payload hasil parse hanya untuk ekstraksi ringan; bila body bukan JSON,
	// event tetap disimpan (raw body adalah sumber kebenarannya).
	payload := in.Payload
	if payload == nil {
		payload = map[string]any{}
		if err := json.Unmarshal(in.RawBody, &payload); err != nil {
			payload = map[string]any{"unparsed": string(in.RawBody)}
		}
	}

	ev := &webhook.Event{
		ID:          platform.NewUUID(),
		Provider:    provider,
		EventID:     webhook.ExtractEventID(payload),
		EventType:   webhook.ExtractEventType(payload),
		PayloadHash: webhook.HashPayload(in.RawBody),
		Payload:     payload,
		RawBody:     in.RawBody,
		ContentType: strings.TrimSpace(in.ContentType),
		Headers:     in.Headers,
		SourceIP:    strings.TrimSpace(in.SourceIP),
		AWB:         webhook.ExtractAWB(payload),
		Status:      webhook.StatusReceived,
		CreatedAt:   time.Now().UTC(),
	}

	// ── Verifikasi signature SEBELUM menyentuh apa pun ──────────────────
	secret, algo, headerName, err := s.secrets.SecretFor(ctx, provider)
	if err != nil {
		if shared.Is(err, shared.ErrNotFound) {
			// Provider belum dikonfigurasi: jangan diam-diam menerima.
			ev.SignatureValid = false
			ev.SignatureError = "rahasia provider belum dikonfigurasi"
			if storeErr := s.events.Store(ctx, ev); storeErr != nil && !isDuplicate(storeErr) {
				return nil, storeErr
			}
			s.auditRejected(ctx, ev, "rahasia provider belum dikonfigurasi")
			return nil, webhook.ErrSecretless
		}
		return nil, err
	}

	if sigErr := webhook.VerifySignature(algo, secret, in.RawBody, signatureHeader); sigErr != nil {
		ev.SignatureValid = false
		ev.SignatureError = sigErr.Error()
		if storeErr := s.events.Store(ctx, ev); storeErr != nil && !isDuplicate(storeErr) {
			return nil, storeErr
		}
		s.auditRejected(ctx, ev, ev.SignatureError)
		s.log.Warn("webhook ditolak: signature tidak sah",
			"provider", provider, "header", headerName, "event_id", ev.EventID)
		return nil, sigErr
	}
	ev.SignatureValid = true

	// Pra-cek duplikat DI LUAR transaksi.
	//
	// Penting: setelah unique violation, transaksi Postgres berada dalam status
	// gagal — semua perintah berikutnya (termasuk COMMIT) ditolak. Karena itu
	// duplikat yang umum terjadi ditangani di sini, bukan di dalam transaksi.
	if existing, dupErr := s.findDuplicate(ctx, ev); dupErr != nil {
		return nil, dupErr
	} else if existing != nil {
		s.log.Info("webhook duplikat diterima — diabaikan (idempotent)",
			"provider", provider, "event_id", ev.EventID, "awb", ev.AWB)
		return &webhook.Result{Event: existing, Duplicate: true}, nil
	}

	// ── Simpan mentah + publish ke outbox dalam SATU transaksi ─────────
	err = s.tx.InTx(ctx, func(q database.Querier) error {
		tx := s.stores.WithTx(q)

		if err := tx.Webhooks().Store(ctx, ev); err != nil {
			if isDuplicate(err) {
				// Balapan: provider mengirim ulang tepat saat kita memproses.
				// Kembalikan sentinel agar transaksi di-rollback dengan rapi
				// (bukan di-commit dalam keadaan gagal).
				return errDuplicateRace
			}
			return err
		}

		// Event diteruskan lewat outbox (ADR-005) — consumer di worker yang
		// menerjemahkannya menjadi tracking event + perubahan status.
		return tx.Outbox().Publish(ctx, event.New(
			platform.NewUUID(),
			event.TrackingEventReceived,
			ev.ID,
			ev.OrganizationID,
			ev.CreatedAt,
			map[string]any{
				"webhook_event_id": ev.ID,
				"provider":         ev.Provider,
				"event_id":         ev.EventID,
				"event_type":       ev.EventType,
				"awb":              ev.AWB,
				"payload":          ev.Payload,
			},
		))
	})
	if err != nil {
		if errors.Is(err, errDuplicateRace) {
			// Balapan terdeteksi: perlakukan sama seperti duplikat biasa.
			if existing, findErr := s.findDuplicate(ctx, ev); findErr == nil && existing != nil {
				s.log.Info("webhook duplikat (balapan) — diabaikan",
					"provider", provider, "event_id", ev.EventID)
				return &webhook.Result{Event: existing, Duplicate: true}, nil
			}
			return &webhook.Result{Event: ev, Duplicate: true}, nil
		}
		return nil, err
	}

	s.recordAudit(ctx, audit.ActionWebhookReceived, ev, nil)
	s.log.Info("webhook diterima", "provider", provider, "event_id", ev.EventID, "awb", ev.AWB)
	return &webhook.Result{Event: ev}, nil
}

// findDuplicate mencari event yang sudah tersimpan dengan identitas sama.
// Mengembalikan (nil, nil) bila belum ada — bukan error.
func (s *Service) findDuplicate(ctx context.Context, ev *webhook.Event) (*webhook.Event, error) {
	if existing, err := s.events.FindByProviderEventID(ctx, ev.Provider, ev.EventID); err != nil {
		if !errors.Is(err, webhook.ErrNotFound) {
			return nil, err
		}
	} else if existing != nil {
		return existing, nil
	}
	if ev.EventID != "" {
		// Provider memberi event_id: kunci hash tidak dipakai (dua event
		// berbeda boleh berisi payload identik).
		return nil, nil
	}
	existing, err := s.events.FindByProviderHash(ctx, ev.Provider, ev.PayloadHash)
	if err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return existing, nil
}

// errDuplicateRace menandai duplikat yang terdeteksi SAAT transaksi berjalan.
// Dipakai untuk memaksa rollback bersih, bukan commit di transaksi gagal.
var errDuplicateRace = errors.New("webhook duplikat terdeteksi saat menyimpan")

// ClaimPending mengambil event yang siap diproses (signature sah, belum diproses).
func (s *Service) ClaimPending(ctx context.Context, limit int) ([]webhook.Event, error) {
	return s.events.ClaimPending(ctx, limit)
}

// MarkProcessed menandai event selesai diproses.
func (s *Service) MarkProcessed(ctx context.Context, id string) error {
	if err := s.events.MarkProcessed(ctx, id); err != nil {
		return err
	}
	if ev, err := s.events.GetByID(ctx, id); err == nil {
		s.recordAudit(ctx, audit.ActionWebhookProcessed, ev, nil)
	}
	return nil
}

// MarkFailed mencatat kegagalan pemrosesan sebuah event.
func (s *Service) MarkFailed(ctx context.Context, id, reason string) error {
	return s.events.MarkFailed(ctx, id, reason)
}

// List mengembalikan halaman event webhook untuk penelusuran operator.
func (s *Service) List(ctx context.Context, provider string, status webhook.Status, limit, offset int) ([]webhook.Event, int, error) {
	return s.events.List(ctx, provider, status, limit, offset)
}

// auditRejected mencatat penolakan webhook sebagai peristiwa keamanan.
func (s *Service) auditRejected(ctx context.Context, ev *webhook.Event, reason string) {
	s.recordAudit(ctx, audit.ActionWebhookRejected, ev, map[string]any{
		"reason":     reason,
		"event_id":   ev.EventID,
		"event_type": ev.EventType,
		"awb":        ev.AWB,
	})
}

// recordAudit mencatat peristiwa webhook; kegagalan audit tidak menggagalkan.
//
// entity_id di audit_logs bertipe uuid, jadi yang dikirim WAJIB id webhook itu
// sendiri. AWB (teks bebas) masuk ke `changes` — bila dikirim sebagai
// entity_id, database menolaknya dan transaksi pemanggilnya ikut gagal.
func (s *Service) recordAudit(ctx context.Context, action string, ev *webhook.Event, extra map[string]any) {
	if s.audit == nil {
		return
	}
	changes := map[string]any{
		"provider":        ev.Provider,
		"signature_valid": ev.SignatureValid,
	}
	if ev.AWB != "" {
		changes["awb"] = ev.AWB
	}
	for k, v := range extra {
		changes[k] = v
	}
	err := s.audit.Record(ctx, audit.Entry{
		OrganizationID: ev.OrganizationID,
		Action:         action,
		EntityType:     "webhook_event",
		EntityID:       ev.ID,
		Changes:        changes,
	})
	if err != nil {
		s.log.Warn("audit webhook gagal", "webhook_id", ev.ID, "error", err)
	}
}

// isDuplicate melaporkan apakah error berarti event sudah pernah diterima.
// Memakai sentinel error domain, bukan pencocokan pesan.
func isDuplicate(err error) bool { return errors.Is(err, webhook.ErrDuplicate) }
