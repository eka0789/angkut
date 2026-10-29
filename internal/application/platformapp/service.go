// Package platformapp menyatukan fitur Platform (TASK-057..063): API key
// (CRUD + autentikasi X-API-Key), search tenant-scoped (trigram), automation
// rule IF/THEN dengan execution log, dan kuota langganan (quota guard).
package platformapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// jsonMarshal/jsonUnmarshal pembungkus encoding/json agar impor tetap di satu tempat.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func jsonUnmarshal(raw []byte, out any) error { return json.Unmarshal(raw, out) }

// Deps dependensi service platform.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Service service platform.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	audit  audit.Recorder
	log    *slog.Logger
}

// New membuat service platform.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, audit: d.Audit, log: log}
}

// ── API Key (TASK-059) ──────────────────────────────────────────────────

// APIKey adalah satu kunci API (tanpa hash).
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedBy  string     `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateKeyInput permintaan membuat kunci.
type CreateKeyInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
	Actor     string
}

// CreateKeyResult mengembalikan kunci MENTAH SEKALI (respons tunggal).
type CreateKeyResult struct {
	Key    APIKey `json:"api_key"`
	RawKey string `json:"key"` // hanya muncul di respons create
}

// validScopes adalah allow-list scope API (contract module 23).
var validScopes = map[string]bool{
	"shipments:read": true, "shipments:write": true,
	"orders:read": true, "tracking:read": true, "webhooks:manage": true,
}

// CreateKey membuat API key baru: kunci mentah di-generate acak, hanya hash
// yang disimpan. Kunci tampil SEKALI di respons.
func (s *Service) CreateKey(ctx context.Context, orgID, requestID string, in CreateKeyInput) (*CreateKeyResult, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, shared.Validation(map[string]string{"name": "Nama kunci wajib diisi."})
	}
	scopes := make([]string, 0, len(in.Scopes))
	for _, sc := range in.Scopes {
		sc = strings.TrimSpace(sc)
		if !validScopes[sc] {
			return nil, shared.Validation(map[string]string{
				"scopes": "Scope tidak dikenal: " + sc + " (yang sah: shipments:read, shipments:write, orders:read, tracking:read, webhooks:manage).",
			})
		}
		scopes = append(scopes, sc)
	}
	if len(scopes) == 0 {
		scopes = []string{"shipments:read"}
	}

	// Kunci: ak_<32 byte hex>. Prefix 12 char pertama disimpan tampil.
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("membangkitkan kunci: %w", err)
	}
	rawKey := "ak_" + hex.EncodeToString(raw)
	prefix := rawKey[:15]
	sum := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(sum[:])

	now := time.Now().UTC()
	id := platform.NewUUID()
	if _, err := s.stores.Querier().Exec(ctx, `
		INSERT INTO api_keys (id, organization_id, name, key_prefix, key_hash, scopes, expires_at, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,NULLIF($8,'')::uuid,$9,$9)`,
		id, orgID, name, prefix, keyHash, mustJSON(scopes), in.ExpiresAt, in.Actor, now); err != nil {
		return nil, err
	}
	s.auditRecord(ctx, orgID, in.Actor, requestID, "apikey.created", id, nil)
	s.log.Info("api key dibuat", "org", orgID, "prefix", prefix)
	return &CreateKeyResult{
		Key: APIKey{ID: id, Name: name, KeyPrefix: prefix, Scopes: scopes,
			ExpiresAt: in.ExpiresAt, CreatedBy: in.Actor, CreatedAt: now},
		RawKey: rawKey,
	}, nil
}

// ListKeys mengembalikan daftar kunci (tanpa hash).
func (s *Service) ListKeys(ctx context.Context, orgID string) ([]APIKey, error) {
	rows, err := s.stores.Querier().Query(ctx, `
		SELECT id, name, key_prefix, scopes, last_used_at, expires_at, revoked_at,
			COALESCE(created_by::text,''), created_at
		FROM api_keys WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]APIKey, 0, 8)
	for rows.Next() {
		var k APIKey
		var scopes []byte
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &scopes, &k.LastUsedAt,
			&k.ExpiresAt, &k.RevokedAt, &k.CreatedBy, &k.CreatedAt); err != nil {
			return nil, err
		}
		_ = unmarshalScopes(scopes, &k.Scopes)
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeKey mencabut kunci (soft delete).
func (s *Service) RevokeKey(ctx context.Context, orgID, id, actor, requestID string) error {
	tag, err := s.stores.Querier().Exec(ctx, `
		UPDATE api_keys SET revoked_at=now()
		WHERE organization_id=$1 AND id=$2 AND revoked_at IS NULL`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.NotFound("API_KEY_NOT_FOUND", "Kunci API tidak ditemukan atau sudah dicabut.")
	}
	s.auditRecord(ctx, orgID, actor, requestID, "apikey.revoked", id, nil)
	return nil
}

// AuthenticateKey memverifikasi X-API-Key: hash cocok, belum dicabut/kedaluwarsa.
// Mengembalikan (orgID, scopes, error). Dipakai middleware API publik.
func (s *Service) AuthenticateKey(ctx context.Context, rawKey string) (string, []string, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return "", nil, shared.ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(rawKey))
	var orgID string
	var scopes []byte
	var revoked, expired *time.Time
	err := s.stores.Querier().QueryRow(ctx, `
		SELECT organization_id::text, scopes, revoked_at, expires_at
		FROM api_keys WHERE key_hash=$1`, hex.EncodeToString(sum[:])).
		Scan(&orgID, &scopes, &revoked, &expired)
	if err != nil {
		return "", nil, shared.ErrUnauthorized
	}
	if revoked != nil || (expired != nil && expired.Before(time.Now().UTC())) {
		return "", nil, shared.New("API_KEY_REVOKED", "Kunci API tidak aktif.", shared.ErrUnauthorized)
	}
	var scopeList []string
	_ = unmarshalScopes(scopes, &scopeList)
	_, _ = s.stores.Querier().Exec(ctx,
		`UPDATE api_keys SET last_used_at=now() WHERE key_hash=$1`,
		hex.EncodeToString(sum[:]))
	return orgID, scopeList, nil
}

// ── Search (TASK-063) ───────────────────────────────────────────────────

// SearchHit satu hasil pencarian lintas entitas.
type SearchHit struct {
	Type   string `json:"type"` // shipment | order | customer
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// Search mencari di shipment (resi trigram), order (nomor), customer (nama).
// Semua query tenant-scoped; LIMIT ketat (oracle prevention).
func (s *Service) Search(ctx context.Context, orgID, query string, limit int) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	if len(q) < 2 {
		return nil, shared.Validation(map[string]string{"q": "Minimal 2 karakter."})
	}
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	pattern := "%" + q + "%"
	out := make([]SearchHit, 0, limit)

	rows, err := s.stores.Querier().Query(ctx, `
		SELECT id::text, awb, status FROM shipments
		WHERE organization_id=$1 AND awb ILIKE $2 ORDER BY created_at DESC LIMIT $3`,
		orgID, pattern, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h SearchHit
		_ = h
		var id, awb, status string
		if err := rows.Scan(&id, &awb, &status); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, SearchHit{Type: "shipment", ID: id, Title: awb, Detail: status})
	}
	rows.Close()

	rows2, err := s.stores.Querier().Query(ctx, `
		SELECT id::text, order_number, status FROM orders
		WHERE organization_id=$1 AND order_number ILIKE $2 ORDER BY created_at DESC LIMIT $3`,
		orgID, pattern, limit)
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var h SearchHit
		_ = h
		var id, number, status string
		if err := rows2.Scan(&id, &number, &status); err != nil {
			rows2.Close()
			return nil, err
		}
		out = append(out, SearchHit{Type: "order", ID: id, Title: number, Detail: status})
	}
	rows2.Close()

	rows3, err := s.stores.Querier().Query(ctx, `
		SELECT id::text, name, phone FROM customers
		WHERE organization_id=$1 AND (name ILIKE $2 OR phone ILIKE $2)
		ORDER BY created_at DESC LIMIT $3`,
		orgID, pattern, limit)
	if err != nil {
		return nil, err
	}
	for rows3.Next() {
		var h SearchHit
		_ = h
		var id, name, phone string
		if err := rows3.Scan(&id, &name, &phone); err != nil {
			rows3.Close()
			return nil, err
		}
		out = append(out, SearchHit{Type: "customer", ID: id, Title: name, Detail: phone})
	}
	rows3.Close()

	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ── Subscription quota (TASK-058) ───────────────────────────────────────

// QuotaCheck hasil pemeriksaan kuota.
type QuotaCheck struct {
	Allowed bool   `json:"allowed"`
	Plan    string `json:"plan"`
	Used    int64  `json:"used"`
	Limit   int64  `json:"limit"` // -1 = unlimited
}

// CheckShipmentQuota memeriksa kuota pengiriman bulan ini untuk organisasi.
// Plan free dibatasi (limit di tabel plans); tanpa subscription = free.
func (s *Service) CheckShipmentQuota(ctx context.Context, orgID string) (*QuotaCheck, error) {
	qc := &QuotaCheck{Allowed: true, Plan: "free", Limit: -1}
	var planName string
	var maxShipments *int64
	var periodStart, periodEnd time.Time
	err := s.stores.Querier().QueryRow(ctx, `
		SELECT COALESCE(p.name,'free'), p.max_shipments_per_month, s.current_period_start, s.current_period_end
		FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.organization_id=$1 AND s.status='active'
		ORDER BY s.current_period_start DESC LIMIT 1`, orgID).
		Scan(&planName, &maxShipments, &periodStart, &periodEnd)
	if err != nil {
		// Tanpa subscription aktif: pakai plan default (free) dari tabel plans.
		_ = s.stores.Querier().QueryRow(ctx, `
			SELECT name, max_shipments_per_month FROM plans
			WHERE name='free' OR price_monthly=0 ORDER BY price_monthly LIMIT 1`).
			Scan(&planName, &maxShipments)
	} else {
		qc.Plan = planName
	}
	if maxShipments == nil {
		return qc, nil // unlimited
	}
	qc.Limit = *maxShipments
	var used int64
	if err := s.stores.Querier().QueryRow(ctx, `
		SELECT count(*) FROM shipments
		WHERE organization_id=$1 AND created_at >= $2 AND created_at < $3`,
		orgID, periodStart, periodEnd).Scan(&used); err != nil {
		return nil, err
	}
	qc.Used = used
	qc.Allowed = used < qc.Limit
	return qc, nil
}

// ── Automation (TASK-057) ───────────────────────────────────────────────

// AutomationRule satu aturan IF/THEN.
type AutomationRule struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Event     string    `json:"event"`     // mis. shipment.delivered
	Condition string    `json:"condition"` // ekspresi sederhana: field=value
	Action    string    `json:"action"`    // mis. notify
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AutomationExecution log satu eksekusi.
type AutomationExecution struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	Status    string    `json:"status"` // executed | skipped | failed
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ── helper ──────────────────────────────────────────────────────────────

func mustJSON(v any) string {
	b, err := jsonMarshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalScopes(raw []byte, out *[]string) error {
	return jsonUnmarshal(raw, out)
}

func (s *Service) auditRecord(ctx context.Context, orgID, actor, requestID, action, entityID string, changes map[string]any) {
	if s.audit == nil {
		return
	}
	e := audit.New(action, actor, orgID).WithEntity("platform", entityID)
	if changes != nil {
		e = e.WithChanges(nil, changes)
	}
	e.RequestID = requestID
	if err := s.audit.Record(ctx, e); err != nil {
		s.log.Warn("audit platform gagal", "action", action, "error", err)
	}
}
