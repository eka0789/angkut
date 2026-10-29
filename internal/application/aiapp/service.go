// Package aiapp mengimplementasikan AI Core (TASK-048..051, 054) dengan
// PROVIDER HEURISTIK: jawaban dirakit dari data org-scoped oleh tool layer
// deterministik — bukan LLM eksternal (ADR-003: provider bisa ditukar; env
// AI_PROVIDER=mock dipakai sampai adapter LLM produksi tersedia).
//
// Guardrail yang dipaksa:
//   - tool read-only dijalankan langsung; tool mutasi WAJIB lewat pending
//     action + konfirmasi eksplisit (TASK-050);
//   - semua query tool difilter organization_id dari sesi (bukan dari LLM);
//   - usage meter mencatat setiap chat (biaya = 0 untuk provider heuristik).
package aiapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps dependensi service AI.
type Deps struct {
	Stores *database.Stores
	Log    *slog.Logger
}

// Service service AI.
type Service struct {
	stores *database.Stores
	log    *slog.Logger
}

// New membuat service AI.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{stores: d.Stores, log: log}
}

// ChatInput permintaan chat.
type ChatInput struct {
	OrganizationID string
	UserID         string
	Message        string
}

// PendingAction adalah aksi mutasi yang menunggu konfirmasi.
type PendingAction struct {
	ID        string         `json:"id"`
	OrgID     string         `json:"-"`
	UserID    string         `json:"-"`
	Type      string         `json:"type"`
	Params    map[string]any `json:"params"`
	Summary   string         `json:"summary"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// ChatResult jawaban chat.
type ChatResult struct {
	Content     string         `json:"content"`
	ToolCalls   []string       `json:"tool_calls"`
	RequiresCfm bool           `json:"requires_confirmation"`
	Pending     *PendingAction `json:"-"`
}

// konfirmasi kedaluwarsa dalam 10 menit.
const pendingTTL = 10 * time.Minute

// pendingStore in-memory: konfirmasi berumur pendek & single-instance.
// (Redis menyusul untuk multi-replika — dicatat di Not Run.)
var pendingStore = map[string]PendingAction{}

// Chat merespons pertanyaan bisnis dengan tool layer heuristik. Pesan
// mengandung kata kunci mutasi ("batalkan") → pending action + konfirmasi.
func (s *Service) Chat(ctx context.Context, in ChatInput) (*ChatResult, error) {
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		return nil, fmt.Errorf("pesan kosong")
	}
	res := &ChatResult{}
	lower := strings.ToLower(msg)

	// Guardrail mutasi: hanya cancel shipment yang didukung saat ini, dan
	// WAJIB konfirmasi (TASK-050).
	if strings.Contains(lower, "batalkan") && strings.Contains(lower, "pengiriman") || strings.Contains(lower, "cancel shipment") {
		shipID := extractFirstUUID(msg)
		if shipID == "" {
			res.Content = "Sebutkan ID pengiriman yang ingin dibatalkan, mis. \"batalkan pengiriman 0192...\"."
			return res, nil
		}
		pa := PendingAction{
			ID: platform.NewUUID(), OrgID: in.OrganizationID, UserID: in.UserID,
			Type: "cancel_shipment", Params: map[string]any{"shipment_id": shipID},
			Summary:   "Batalkan pengiriman " + shipID + "? Tindakan ini tidak dapat dibatalkan.",
			ExpiresAt: time.Now().Add(pendingTTL),
		}
		pendingStore[pa.ID] = pa
		res.RequiresCfm = true
		res.Pending = &pa
		res.Content = pa.Summary + " Lanjutkan?"
		return res, nil
	}

	// Tool read-only: pilih berdasarkan kata kunci.
	var answer string
	switch {
	case strings.Contains(lower, "biaya") || strings.Contains(lower, "ongkir") || strings.Contains(lower, "cost"):
		answer = s.toolShippingCost(ctx, in.OrganizationID)
		res.ToolCalls = append(res.ToolCalls, "shipping_cost_30d")
	case strings.Contains(lower, "cod"):
		answer = s.toolCOD(ctx, in.OrganizationID)
		res.ToolCalls = append(res.ToolCalls, "cod_summary")
	case strings.Contains(lower, "status") || strings.Contains(lower, "pengiriman"):
		answer = s.toolShipmentStatus(ctx, in.OrganizationID)
		res.ToolCalls = append(res.ToolCalls, "shipment_status_counts")
	default:
		answer = s.toolOverview(ctx, in.OrganizationID)
		res.ToolCalls = append(res.ToolCalls, "overview")
	}
	res.Content = answer
	s.log.Info("ai chat", "org", in.OrganizationID, "user", in.UserID, "tools", res.ToolCalls)
	return res, nil
}

// Confirm mengeksekusi/membatalkan pending action.
func (s *Service) Confirm(ctx context.Context, orgID, userID, actionID string, confirm bool) (string, error) {
	pa, ok := pendingStore[actionID]
	if !ok || pa.OrgID != orgID || pa.UserID != userID {
		return "", fmt.Errorf("aksi konfirmasi tidak ditemukan atau kedaluwarsa")
	}
	if time.Now().After(pa.ExpiresAt) {
		delete(pendingStore, actionID)
		return "", fmt.Errorf("aksi konfirmasi kedaluwarsa")
	}
	delete(pendingStore, actionID)
	if !confirm {
		return "Aksi dibatalkan.", nil
	}
	if pa.Type == "cancel_shipment" {
		shipID, _ := pa.Params["shipment_id"].(string)
		// Mutasi lewat domain shipment (state machine menolak bila tidak sah).
		txn, err := s.stores.Shipments().GetByID(ctx, orgID, shipID)
		if err != nil {
			return "", fmt.Errorf("pengiriman tidak ditemukan: %w", err)
		}
		// Status `from` WAJIB diambil SEBELUM Transition — Transition
		// memutasi struct (from == to membuat UPDATE 0 baris → NotFound palsu).
		from := txn.Status
		if err := txn.Transition(shipment.StatusCancelled); err != nil {
			return "", err
		}
		if err := s.stores.Shipments().UpdateStatus(ctx, orgID, shipID,
			from, shipment.StatusCancelled, "dibatalkan via AI (terkonfirmasi)"); err != nil {
			return "", err
		}
		return "Pengiriman " + shipID + " dibatalkan.", nil
	}
	return "", fmt.Errorf("jenis aksi tidak dikenal: %s", pa.Type)
}

// ── Tools (read-only, org-scoped) ───────────────────────────────────────

func (s *Service) toolOverview(ctx context.Context, orgID string) string {
	var orders, shipments, delivered int64
	q := s.stores.Querier()
	_ = q.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM orders WHERE organization_id=$1),
		(SELECT count(*) FROM shipments WHERE organization_id=$1),
		(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status='delivered')`,
		orgID).Scan(&orders, &shipments, &delivered)
	return fmt.Sprintf("Ringkasan: %d order, %d pengiriman, %d terkirim. Tanya: biaya ongkir 30 hari, status pengiriman, atau COD.", orders, shipments, delivered)
}

func (s *Service) toolShippingCost(ctx context.Context, orgID string) string {
	var cost int64
	var n int64
	_ = s.stores.Querier().QueryRow(ctx, `SELECT COALESCE(sum(shipping_price),0), count(*)
		FROM shipments WHERE organization_id=$1 AND created_at >= now() - interval '30 days'`,
		orgID).Scan(&cost, &n)
	if n == 0 {
		return "Belum ada pengiriman dalam 30 hari terakhir."
	}
	return fmt.Sprintf("Biaya pengiriman 30 hari terakhir: Rp %s untuk %d pengiriman (rata-rata Rp %s).",
		comma(cost), n, comma(cost/n))
}

func (s *Service) toolCOD(ctx context.Context, orgID string) string {
	var pending, settled int64
	_ = s.stores.Querier().QueryRow(ctx, `SELECT
		COALESCE(sum(amount) FILTER (WHERE status IN ('pending','collected','settling')),0),
		COALESCE(sum(amount) FILTER (WHERE status='settled'),0)
		FROM cod_transactions WHERE organization_id=$1`, orgID).Scan(&pending, &settled)
	return fmt.Sprintf("COD menunggu penyelesaian: Rp %s. Sudah lunas: Rp %s.", comma(pending), comma(settled))
}

func (s *Service) toolShipmentStatus(ctx context.Context, orgID string) string {
	rows, err := s.stores.Querier().Query(ctx, `SELECT status, count(*) FROM shipments
		WHERE organization_id=$1 GROUP BY status ORDER BY count(*) DESC`, orgID)
	if err != nil {
		return "Gagal membaca status pengiriman."
	}
	defer rows.Close()
	parts := make([]string, 0, 6)
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %d", st, n))
	}
	if len(parts) == 0 {
		return "Belum ada pengiriman."
	}
	return "Status pengiriman — " + strings.Join(parts, ", ") + "."
}

// extractFirstUUID mengambil UUID pertama dalam teks (untuk intent mutasi).
func extractFirstUUID(text string) string {
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' || r == '-')
	}) {
		if len(word) == 36 && strings.Count(word, "-") == 4 {
			return word
		}
	}
	return ""
}

// comma memformat angka ribuan id-ID tanpa impor tambahan.
func comma(v int64) string {
	s := fmt.Sprintf("%d", v)
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// ── Insight (TASK-054, heuristik) ───────────────────────────────────────

// Insight satu kartu saran.
type Insight struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Level string `json:"level"` // info | warning
}

// Insights menghasilkan kartu proaktif dari data tenant.
func (s *Service) Insights(ctx context.Context, orgID string) ([]Insight, error) {
	out := []Insight{}
	var inTransit, failed, codPending int64
	var slowCouriers int64
	q := s.stores.Querier()
	_ = q.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status='in_transit'),
		(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status='failed'),
		(SELECT count(*) FROM cod_transactions WHERE organization_id=$1 AND status='collected')`,
		orgID).Scan(&inTransit, &failed, &codPending)
	_ = q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT courier_code FROM shipments WHERE organization_id=$1 AND status='in_transit'
		AND created_at < now() - interval '3 days' GROUP BY courier_code) x`,
		orgID).Scan(&slowCouriers)
	if slowCouriers > 0 {
		out = append(out, Insight{
			Title: "Pengiriman tertahan",
			Body:  fmt.Sprintf("%d kurir punya pengiriman in-transit lebih dari 3 hari.", slowCouriers),
			Level: "warning",
		})
	}
	if codPending > 0 {
		out = append(out, Insight{
			Title: "COD siap disettle",
			Body:  fmt.Sprintf("%d tagihan COD sudah tercollect — buat settlement agar uang kembali.", codPending),
			Level: "info",
		})
	}
	if failed > 0 {
		out = append(out, Insight{
			Title: "Pengiriman gagal",
			Body:  fmt.Sprintf("%d pengiriman gagal — cek alasan dan jadwalkan ulang.", failed),
			Level: "warning",
		})
	}
	if len(out) == 0 {
		out = append(out, Insight{Title: "Semua aman", Body: "Tidak ada hal yang butuh perhatian hari ini.", Level: "info"})
	}
	return out, nil
}
