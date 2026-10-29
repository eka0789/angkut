// Package codapp berisi logika aplikasi COD (TASK-041/042): penerbitan
// transaksi COD, pengumpulan saat delivered, settlement per kurir & periode,
// mark-paid dengan entri ledger, dan ekspor CSV.
//
// Aturan CODING_RULES yang dipatuhi:
//   - settlement tidak mengubah transaksi lama (hanya status);
//   - mark-paid menulis entri ledger DI DALAM transaksi yang sama (ADR-005);
//   - Idempotency-Key di settlement & mark-paid (replay = hasil pertama);
//   - eksternal (CSV) dibangun di luar transaksi.
package codapp

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/cod"
	"github.com/angkut/angkut/apps/internal/domain/ledger"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps dependensi service COD.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Service service aplikasi COD.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	audit  audit.Recorder
	log    *slog.Logger
}

// New membuat service COD.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, audit: d.Audit, log: log}
}

// ── Penerbitan & pengumpulan (dipanggil modul lain) ─────────────────────

// IssueTransaction menerbitkan tagihan COD untuk pengiriman yang dibuat.
// Dipanggil DI DALAM transaksi shipment (q = tx) supaya tagihan lahir
// bersama pengiriman; pengiriman tanpa COD tidak memanggil ini.
func (s *Service) IssueTransaction(ctx context.Context, q database.Querier, orgID, shipmentID, courierCode string, amount, fee int64) error {
	if amount <= 0 {
		return shared.Validation(map[string]string{"cod_amount": "Nilai COD harus lebih dari 0."})
	}
	t := &cod.Transaction{
		ID: platform.NewUUID(), OrganizationID: orgID, ShipmentID: shipmentID,
		CourierCode: courierCode, Amount: amount, Fee: fee,
		Status:    cod.StatusPending,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	return s.stores.WithTx(q).COD().CreateTransaction(ctx, t)
}

// CollectShipment menandai COD terkumpul saat pengiriman delivered. Dipanggil
// DI DALAM transaksi tracking; transaksi COD yang tidak ada = bukan kegagalan
// (pengiriman non-COD).
func (s *Service) CollectShipment(ctx context.Context, q database.Querier, orgID, shipmentID string, at time.Time) error {
	err := s.stores.WithTx(q).COD().MarkCollected(ctx, orgID, shipmentID, at)
	if shared.Is(err, shared.ErrNotFound) {
		return nil // bukan kiriman COD — bukan kegagalan
	}
	return err
}

// FailShipment membatalkan tagihan COD saat pengiriman gagal/dibatalkan.
func (s *Service) FailShipment(ctx context.Context, q database.Querier, orgID, shipmentID string) error {
	err := s.stores.WithTx(q).COD().MarkFailed(ctx, orgID, shipmentID)
	if shared.Is(err, shared.ErrNotFound) {
		return nil
	}
	return err
}

// ── Endpoint-facing ─────────────────────────────────────────────────────

// ListFilter filter transaksi dari endpoint.
type ListFilter struct {
	Status      string
	CourierCode string
	Limit       int
	Offset      int
}

// Transactions mengembalikan halaman transaksi COD.
func (s *Service) Transactions(ctx context.Context, orgID string, f ListFilter) ([]cod.Transaction, int, error) {
	return s.stores.COD().ListTransactions(ctx, orgID, database.CODTxnFilter{
		Status: f.Status, CourierCode: f.CourierCode, Limit: f.Limit, Offset: f.Offset,
	})
}

// Summary mengembalikan rekap COD.
func (s *Service) Summary(ctx context.Context, orgID string) (*cod.Summary, error) {
	return s.stores.COD().Summary(ctx, orgID)
}

// SettlementFilter filter settlement dari endpoint.
type SettlementFilter struct {
	Status      string
	CourierCode string
	Limit       int
	Offset      int
}

// Settlements mengembalikan halaman settlement.
func (s *Service) Settlements(ctx context.Context, orgID string, f SettlementFilter) ([]cod.Settlement, int, error) {
	return s.stores.COD().ListSettlements(ctx, orgID, database.CODSettleFilter{
		Status: f.Status, CourierCode: f.CourierCode, Limit: f.Limit, Offset: f.Offset,
	})
}

// Settlement mengambil satu settlement + item.
func (s *Service) Settlement(ctx context.Context, orgID, id string) (*cod.Settlement, error) {
	return s.stores.COD().GetSettlement(ctx, orgID, id)
}

// SubmitInput permintaan membuat settlement.
type SubmitInput struct {
	CourierCode    string
	PeriodStart    string
	PeriodEnd      string
	TransactionIDs []string
	Actor          string
	IdempotencyKey string
}

// Submit membuat settlement secara atomik + idempoten. Replay key sama
// mengembalikan settlement pertama.
func (s *Service) Submit(ctx context.Context, orgID, requestID string, in SubmitInput) (*cod.Settlement, error) {
	in.CourierCode = strings.TrimSpace(in.CourierCode)
	in.PeriodStart = strings.TrimSpace(in.PeriodStart)
	in.PeriodEnd = strings.TrimSpace(in.PeriodEnd)
	if err := cod.ValidateCreate(cod.CreateSettlementInput{
		OrganizationID: orgID, CourierCode: in.CourierCode,
		PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd,
		TransactionIDs: in.TransactionIDs,
	}); err != nil {
		return nil, err
	}

	key := strings.TrimSpace(in.IdempotencyKey)
	if key != "" {
		if existing, err := s.stores.COD().GetSettlementIDempotency(ctx, orgID, key); err == nil {
			s.log.Info("settlement COD diputar ulang (idempoten)", "key", key, "settlement", existing.ID)
			return existing, nil
		} else if !shared.Is(err, shared.ErrNotFound) {
			return nil, err
		}
	}

	// Dedup + UUID guard input (pola bulkapp: input tidak dipercaya).
	ids := make([]string, 0, len(in.TransactionIDs))
	seen := make(map[string]bool, len(in.TransactionIDs))
	for _, id := range in.TransactionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, shared.Validation(map[string]string{
				"transaction_ids": "Daftar transaksi memuat nilai yang bukan UUID.",
			})
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, shared.Validation(map[string]string{
			"transaction_ids": "Pilih minimal satu transaksi COD.",
		})
	}

	// Muat transaksi untuk verifikasi milik-org + kurir sama + status
	// settleable SEBELUM transaksi (pesan 422 yang jelas).
	items := make([]cod.Transaction, 0, len(ids))
	var totalAmount, totalFee int64
	for _, id := range ids {
		t, err := s.stores.COD().GetTransaction(ctx, orgID, id)
		if err != nil {
			return nil, cod.ErrTransaksiTidakSettleable(id)
		}
		if t.CourierCode != in.CourierCode {
			return nil, cod.ErrTransaksiTidakSettleable(id)
		}
		if t.Status != cod.StatusPending && t.Status != cod.StatusCollected {
			return nil, cod.ErrTransaksiTidakSettleable(id)
		}
		items = append(items, *t)
		totalAmount += t.Amount
		totalFee += t.Fee
	}

	st := &cod.Settlement{
		ID: platform.NewUUID(), OrganizationID: orgID,
		CourierCode: in.CourierCode, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd,
		Status: cod.SettlementPending, TotalAmount: totalAmount, TotalFee: totalFee,
		IdempotencyKey: key, CreatedBy: in.Actor,
		Items:     items,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	err := s.tx.InTx(ctx, func(q database.Querier) error {
		return s.stores.WithTx(q).COD().CreateSettlement(ctx, st)
	})
	if err != nil {
		// Balapan dua submit key sama: putar hasil pemenang.
		if key != "" {
			if existing, gerr := s.stores.COD().GetSettlementIDempotency(ctx, orgID, key); gerr == nil {
				return existing, nil
			}
		}
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionCODSettlementCreated, st.ID, map[string]any{
		"total": totalAmount, "fee": totalFee, "items": len(items),
	})
	s.log.Info("settlement COD dibuat", "org", orgID, "settlement", st.ID,
		"total", totalAmount, "fee", totalFee, "items", len(items))
	return st, nil
}

// MarkPaidInput permintaan menandai settlement lunas.
type MarkPaidInput struct {
	Reference      string
	PaidAt         *time.Time
	ProofURL       string
	Actor          string
	IdempotencyKey string
}

// MarkPaid menandai settlement lunas dan menulis entri ledger settlement
// (debit settlement_clearing = uang masuk; entri COD revenue terpisah saat
// collected). Idempoten: settlement paid → error 409 jelas, replay key sama
// dibiarkan ke middleware idempotency global.
func (s *Service) MarkPaid(ctx context.Context, orgID, settlementID, requestID string, in MarkPaidInput) (*cod.Settlement, error) {
	settle, err := s.stores.COD().GetSettlement(ctx, orgID, settlementID)
	if err != nil {
		return nil, err
	}
	if settle.Status == cod.SettlementPaid {
		return nil, cod.ErrAlreadySettled()
	}
	now := time.Now().UTC()
	err = s.tx.InTx(ctx, func(q database.Querier) error {
		if err := s.stores.WithTx(q).COD().MarkPaid(ctx, orgID, settlementID, cod.MarkPaidInput{
			Reference: in.Reference, PaidAt: in.PaidAt, ProofURL: in.ProofURL, Actor: in.Actor,
		}); err != nil {
			return err
		}
		// Ledger: uang COD dari kurir masuk kas → debit clearing.
		entry, lerr := ledger.New(orgID, ledger.AccountSettlementClear,
			"cod_settlement", settle.ID,
			"Settlement COD "+settle.CourierCode+" "+settle.PeriodStart+".."+settle.PeriodEnd,
			in.Actor, settle.TotalAmount-settle.TotalFee, 0, now)
		if lerr != nil {
			return lerr
		}
		entry.ID = platform.NewUUID()
		return s.stores.WithTx(q).Ledger().Insert(ctx, entry)
	})
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionCODSettlementPaid, settle.ID, nil)
	return s.stores.COD().GetSettlement(ctx, orgID, settlementID)
}

// SubmitSettlement alias dari Submit agar nama kontrak endpoint jelas.
func (s *Service) SubmitSettlement(ctx context.Context, orgID, requestID string, in SubmitInput) (*cod.Settlement, error) {
	return s.Submit(ctx, orgID, requestID, in)
}

// ExportCSV mengembalikan CSV settlement (dibangun di luar transaksi).
func (s *Service) ExportCSV(ctx context.Context, orgID, courierCode, periodStart, periodEnd string) ([]byte, error) {
	sets, err := s.stores.COD().SettlementsForExport(ctx, orgID, courierCode, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("settlement_id,courier_code,period_start,period_end,status,total_amount,total_fee,net_amount,reference,paid_at\r\n")
	for _, st := range sets {
		paidAt := ""
		if st.PaidAt != nil {
			paidAt = st.PaidAt.Format(time.RFC3339)
		}
		fields := []string{
			st.ID, st.CourierCode, st.PeriodStart, st.PeriodEnd, string(st.Status),
			itoa64(st.TotalAmount), itoa64(st.TotalFee), itoa64(st.TotalAmount - st.TotalFee),
			csvField(st.Reference), csvField(paidAt),
		}
		b.WriteString(strings.Join(fields, ",") + "\r\n")
	}
	return []byte(b.String()), nil
}

// csvField melindungi field CSV (koma/kutip/injeksi formula).
func csvField(v string) string {
	v = strings.TrimSpace(v)
	if strings.ContainsAny(v, ",\"\r\n") || strings.HasPrefix(v, "=") ||
		strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") || strings.HasPrefix(v, "@") {
		v = "'" + v // anti formula-injection untuk spreadsheet
	}
	return "\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

// recordAudit menyimpan jejak audit COD.
func (s *Service) recordAudit(ctx context.Context, orgID, actor, requestID, action, entityID string, changes map[string]any) {
	if s.audit == nil {
		return
	}
	e := audit.New(action, actor, orgID).WithEntity("cod", entityID)
	if changes != nil {
		e = e.WithChanges(nil, changes)
	}
	e.RequestID = requestID
	if err := s.audit.Record(ctx, e); err != nil {
		s.log.Warn("audit COD gagal", "action", action, "error", err)
	}
}
