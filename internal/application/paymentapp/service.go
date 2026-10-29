// Package paymentapp berisi logika aplikasi pembayaran & invoice (TASK-044,
// provider mock): invoice diterbitkan bersama order non-COD, payment masuk
// IDEMPOTEN via provider_ref (replay = payment yang sama), dan pembayaran
// menulis entri ledger payment_received/revenue.
package paymentapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/ledger"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps dependensi service payment.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Service service aplikasi pembayaran.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	audit  audit.Recorder
	log    *slog.Logger
}

// New membuat service pembayaran.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, audit: d.Audit, log: log}
}

// ── Penerbitan invoice (dipanggil orderapp) ─────────────────────────────

// IssueInvoice menerbitkan invoice untuk order non-COD. Dipanggil DI DALAM
// transaksi order; nomor diterbitkan di transaksi yang sama.
func (s *Service) IssueInvoice(ctx context.Context, q database.Querier, orgID, orderID string, amount int64) (string, error) {
	if amount <= 0 {
		return "", shared.Validation(map[string]string{"amount": "Nilai invoice harus lebih dari 0."})
	}
	st := s.stores.WithTx(q)
	number, err := st.Invoices().NextInvoiceNumber(ctx, orgID, time.Now().UTC().Year())
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	inv := &database.Invoice{
		ID: platform.NewUUID(), OrganizationID: orgID,
		Number: number, OrderID: orderID, Amount: amount,
		Status: "issued", CreatedAt: now, UpdatedAt: now,
	}
	if err := st.Invoices().CreateInvoice(ctx, inv); err != nil {
		return "", err
	}
	return inv.ID, nil
}

// ── Endpoint-facing ─────────────────────────────────────────────────────

// ListInvoices mengembalikan halaman invoice.
func (s *Service) ListInvoices(ctx context.Context, orgID string, limit, offset int) ([]database.Invoice, int, error) {
	return s.stores.Invoices().ListInvoices(ctx, orgID, limit, offset)
}

// ListPayments mengembalikan halaman payment.
func (s *Service) ListPayments(ctx context.Context, orgID string, limit, offset int) ([]database.Payment, int, error) {
	return s.stores.Payments().ListPayments(ctx, orgID, limit, offset)
}

// RecordInput permintaan mencatat pembayaran masuk.
type RecordInput struct {
	InvoiceID string
	Provider  string
	Ref       string
	Amount    int64
	Actor     string
	Data      map[string]any
}

// Record mencatat pembayaran secara IDEMPOTEN (provider_ref sama = replay)
// lalu menandai invoice paid + menulis entri ledger. Semuanya atomik.
func (s *Service) Record(ctx context.Context, orgID, requestID string, in RecordInput) (*database.Payment, bool, error) {
	if strings.TrimSpace(in.Ref) == "" {
		return nil, false, shared.Validation(map[string]string{"provider_ref": "Referensi provider wajib diisi."})
	}
	if in.Amount <= 0 {
		return nil, false, shared.Validation(map[string]string{"amount": "Nilai pembayaran harus lebih dari 0."})
	}
	if in.InvoiceID != "" {
		if _, err := uuid.Parse(in.InvoiceID); err != nil {
			return nil, false, shared.Validation(map[string]string{"invoice_id": "ID invoice bukan UUID yang valid."})
		}
	}
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		provider = "mock"
	}

	p := &database.Payment{
		ID: platform.NewUUID(), OrganizationID: orgID,
		InvoiceID: in.InvoiceID, Provider: provider, ProviderRef: strings.TrimSpace(in.Ref),
		Amount: in.Amount, Payload: in.Data, CreatedAt: time.Now().UTC(),
	}
	var replayed bool
	err := s.tx.InTx(ctx, func(q database.Querier) error {
		_, rerr, err := s.stores.WithTx(q).Payments().CreatePayment(ctx, p)
		if err != nil {
			return err
		}
		replayed = rerr
		if replayed {
			return nil // replay: tidak ada efek samping baru
		}
		if in.InvoiceID != "" {
			if err := s.stores.WithTx(q).Invoices().MarkInvoicePaid(ctx, orgID, in.InvoiceID); err != nil {
				return err
			}
		}
		entry, lerr := ledger.New(orgID, ledger.AccountPaymentReceived,
			"payment", p.ID, "Pembayaran "+provider+" ref "+p.ProviderRef,
			in.Actor, in.Amount, 0, time.Now().UTC())
		if lerr != nil {
			return lerr
		}
		entry.ID = platform.NewUUID()
		return s.stores.WithTx(q).Ledger().Insert(ctx, entry)
	})
	if err != nil {
		return nil, false, err
	}
	if !replayed {
		s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionPaymentReceived, p.ID, map[string]any{
			"amount": in.Amount, "provider": provider, "ref": p.ProviderRef,
		})
		s.log.Info("payment diterima", "org", orgID, "payment", p.ID, "amount", in.Amount)
	}
	got, err := s.stores.Payments().GetByRef(ctx, orgID, provider, p.ProviderRef)
	if err != nil {
		return p, replayed, nil
	}
	return got, replayed, nil
}

func (s *Service) recordAudit(ctx context.Context, orgID, actor, requestID, action, entityID string, changes map[string]any) {
	if s.audit == nil {
		return
	}
	e := audit.New(action, actor, orgID).WithEntity("payment", entityID)
	if changes != nil {
		e = e.WithChanges(nil, changes)
	}
	e.RequestID = requestID
	if err := s.audit.Record(ctx, e); err != nil {
		s.log.Warn("audit payment gagal", "action", action, "error", err)
	}
}
