package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/codapp"
	"github.com/angkut/angkut/apps/internal/application/paymentapp"
	"github.com/angkut/angkut/apps/internal/application/returnsapp"
	"github.com/angkut/angkut/apps/internal/domain/cod"
	"github.com/angkut/angkut/apps/internal/domain/ledger"
	"github.com/angkut/angkut/apps/internal/domain/returns"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// CODService adalah antarmuka service COD untuk handler.
type CODService interface {
	Transactions(ctx context.Context, orgID string, f codapp.ListFilter) ([]cod.Transaction, int, error)
	Summary(ctx context.Context, orgID string) (*cod.Summary, error)
	Settlements(ctx context.Context, orgID string, f codapp.SettlementFilter) ([]cod.Settlement, int, error)
	Settlement(ctx context.Context, orgID, id string) (*cod.Settlement, error)
	SubmitSettlement(ctx context.Context, orgID, requestID string, in codapp.SubmitInput) (*cod.Settlement, error)
	MarkPaid(ctx context.Context, orgID, settlementID, requestID string, in codapp.MarkPaidInput) (*cod.Settlement, error)
	ExportCSV(ctx context.Context, orgID, courierCode, periodStart, periodEnd string) ([]byte, error)
}

// ReturnsService adalah antarmuka service retur untuk handler.
type ReturnsService interface {
	List(ctx context.Context, orgID string, f returnsapp.ListFilter) ([]returns.Return, int, error)
	Get(ctx context.Context, orgID, id string) (*returns.Return, error)
	Create(ctx context.Context, orgID, requestID string, in returnsapp.CreateInput) (*returns.Return, error)
	Decide(ctx context.Context, orgID, id, requestID, actor string, approve bool, note string) (*returns.Return, error)
	Receive(ctx context.Context, orgID, id, actor string) (*returns.Return, error)
	Inspect(ctx context.Context, orgID, id, requestID string, in returnsapp.InspectInput) (*returns.Return, error)
	Refund(ctx context.Context, orgID, id, requestID string, in returnsapp.RefundInput) (*returns.Return, error)
	Policy(ctx context.Context, orgID string) (*returns.Policy, error)
	UpdatePolicy(ctx context.Context, orgID, requestID string, in returnsapp.UpdatePolicyInput) (*returns.Policy, error)
}

// PaymentService adalah antarmuka service pembayaran untuk handler.
type PaymentService interface {
	ListInvoices(ctx context.Context, orgID string, limit, offset int) ([]database.Invoice, int, error)
	ListPayments(ctx context.Context, orgID string, limit, offset int) ([]database.Payment, int, error)
	Record(ctx context.Context, orgID, requestID string, in paymentapp.RecordInput) (*database.Payment, bool, error)
}

// LedgerReader membaca buku besar (implementasi database.LedgerRepo).
type LedgerReader interface {
	List(ctx context.Context, orgID string, account string, limit, offset int) ([]ledger.Entry, int, error)
	FinancialReport(ctx context.Context, orgID string) (*ledger.FinancialReport, error)
}

// MoneyHandler menangani endpoint COD, ledger, returns & kebijakan.
type MoneyHandler struct {
	cod     CODService
	returns ReturnsService
	ledger  LedgerReader
	pay     PaymentService
}

// NewMoneyHandler membuat handler money.
func NewMoneyHandler(codSvc CODService, retSvc ReturnsService, ledger LedgerReader, paySvc PaymentService) *MoneyHandler {
	return &MoneyHandler{cod: codSvc, returns: retSvc, ledger: ledger, pay: paySvc}
}

// ── Payments & invoices ─────────────────────────────────────────────────

// RegisterPayments mendaftarkan rute /payments & /invoices (finance+).
func (h *MoneyHandler) RegisterPayments(r chi.Router) {
	r.Get("/invoices", h.ListInvoices)
	r.Get("/", h.ListPayments)
	r.Post("/", h.RecordPayment)
}

func (h *MoneyHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, total, err := h.pay.ListInvoices(r.Context(), identity.OrganizationID,
		atoiDefault(r.URL.Query().Get("limit"), 50), atoiDefault(r.URL.Query().Get("offset"), 0))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{"total": total})
}

func (h *MoneyHandler) ListPayments(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, total, err := h.pay.ListPayments(r.Context(), identity.OrganizationID,
		atoiDefault(r.URL.Query().Get("limit"), 50), atoiDefault(r.URL.Query().Get("offset"), 0))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{"total": total})
}

type recordPaymentRequest struct {
	InvoiceID string         `json:"invoice_id"`
	Provider  string         `json:"provider"`
	Ref       string         `json:"provider_ref"`
	Amount    int64          `json:"amount"`
	Data      map[string]any `json:"data"`
}

// RecordPayment menangani POST /api/v1/payments — IDEMPOTEN via provider_ref.
func (h *MoneyHandler) RecordPayment(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[recordPaymentRequest](w, r)
	if !ok {
		return
	}
	p, replayed, err := h.pay.Record(r.Context(), identity.OrganizationID, requestID(r), paymentapp.RecordInput{
		InvoiceID: req.InvoiceID, Provider: req.Provider, Ref: req.Ref,
		Amount: req.Amount, Actor: identity.UserID, Data: req.Data,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	w.Header().Set("X-Idempotent-Replay", strconv.FormatBool(replayed))
	if replayed {
		response.OK(w, p)
		return
	}
	response.Created(w, p)
}

// RegisterCOD mendaftarkan rute di /api/v1/cod (finance+).
func (h *MoneyHandler) RegisterCOD(r chi.Router) {
	r.Get("/transactions", h.CODTransactions)
	r.Get("/summary", h.CODSummary)
	r.Get("/settlements", h.CODSettlements)
	r.Post("/settlements", h.CreateSettlement)
	r.Get("/settlements/export", h.ExportSettlements)
	r.Route("/settlements/{id}", func(one chi.Router) {
		one.Get("/", h.GetSettlement)
		one.Post("/mark-paid", h.MarkPaid)
	})
}

// RegisterReturns mendaftarkan rute /returns per grup RBAC. Router yang nil
// dilewati — grup lain mendaftar method berbeda pada path yang sama.
func (h *MoneyHandler) RegisterReturns(member chi.Router, write chi.Router, admin chi.Router, warehouse chi.Router, finance chi.Router) {
	if member != nil {
		member.Get("/", h.ListReturns)
		member.Get("/{id}", h.GetReturn)
	}
	if write != nil {
		write.Post("/", h.CreateReturn)
	}
	if admin != nil {
		admin.Route("/{id}", func(one chi.Router) {
			one.Post("/approve", h.ApproveReturn)
			one.Post("/reject", h.RejectReturn)
		})
	}
	if warehouse != nil {
		warehouse.Post("/{id}/receive", h.ReceiveReturn)
		warehouse.Post("/{id}/inspect", h.InspectReturn)
	}
	if finance != nil {
		finance.Post("/{id}/refund", h.RefundReturn)
	}
}

// RegisterPolicy mendaftarkan rute kebijakan pada konteks /return-policies.
func (h *MoneyHandler) RegisterPolicy(member chi.Router, admin chi.Router) {
	if member != nil {
		member.Get("/", h.GetPolicy)
	}
	if admin != nil {
		admin.Put("/", h.UpdatePolicy)
	}
}

// RegisterLedger mendaftarkan rute /ledger & /analytics/financial (finance+).
func (h *MoneyHandler) RegisterLedger(r chi.Router) {
	r.Get("/entries", h.LedgerEntries)
}

// parseRFC3339 mem-parsing timestamp klien dengan pesan baku.
func parseRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(s))
}

// ── COD ─────────────────────────────────────────────────────────────────

func (h *MoneyHandler) CODTransactions(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.cod.Transactions(r.Context(), identity.OrganizationID, codapp.ListFilter{
		Status:      strings.TrimSpace(q.Get("status")),
		CourierCode: strings.TrimSpace(q.Get("courier")),
		Limit:       atoiDefault(q.Get("limit"), 50),
		Offset:      atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

func (h *MoneyHandler) CODSummary(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	s, err := h.cod.Summary(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, s)
}

func (h *MoneyHandler) CODSettlements(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.cod.Settlements(r.Context(), identity.OrganizationID, codapp.SettlementFilter{
		Status:      strings.TrimSpace(q.Get("status")),
		CourierCode: strings.TrimSpace(q.Get("courier")),
		Limit:       atoiDefault(q.Get("limit"), 50),
		Offset:      atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

type settlementRequest struct {
	CourierCode    string   `json:"courier_code"`
	PeriodStart    string   `json:"period_start"`
	PeriodEnd      string   `json:"period_end"`
	TransactionIDs []string `json:"transaction_ids"`
}

func (h *MoneyHandler) CreateSettlement(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[settlementRequest](w, r)
	if !ok {
		return
	}
	st, err := h.cod.SubmitSettlement(r.Context(), identity.OrganizationID, requestID(r), codapp.SubmitInput{
		CourierCode: req.CourierCode, PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd,
		TransactionIDs: req.TransactionIDs, Actor: identity.UserID,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, st)
}

func (h *MoneyHandler) GetSettlement(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	st, err := h.cod.Settlement(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, st)
}

type markPaidRequest struct {
	Reference string `json:"reference"`
	PaidAt    string `json:"paid_at"`
	ProofURL  string `json:"proof_url"`
}

func (h *MoneyHandler) MarkPaid(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[markPaidRequest](w, r)
	if !ok {
		return
	}
	in := codapp.MarkPaidInput{
		Reference: req.Reference, ProofURL: req.ProofURL, Actor: identity.UserID,
	}
	if ts := strings.TrimSpace(req.PaidAt); ts != "" {
		t, err := parseRFC3339(ts)
		if err != nil {
			response.Fail(w, ValidationError(map[string]string{"paid_at": "Format waktu tidak valid (RFC3339)."}))
			return
		}
		in.PaidAt = &t
	}
	st, err := h.cod.MarkPaid(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), requestID(r), in)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, st)
}

func (h *MoneyHandler) ExportSettlements(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	csv, err := h.cod.ExportCSV(r.Context(), identity.OrganizationID,
		strings.TrimSpace(q.Get("courier")), strings.TrimSpace(q.Get("period_start")), strings.TrimSpace(q.Get("period_end")))
	if err != nil {
		response.Fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="cod-settlements.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csv)
}

// ── Ledger & financial ──────────────────────────────────────────────────

func (h *MoneyHandler) LedgerEntries(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.ledger.List(r.Context(), identity.OrganizationID,
		strings.TrimSpace(q.Get("account")),
		atoiDefault(q.Get("limit"), 50), atoiDefault(q.Get("offset"), 0))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

func (h *MoneyHandler) Financial(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	rep, err := h.ledger.FinancialReport(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, rep)
}

// ── Returns ─────────────────────────────────────────────────────────────

func (h *MoneyHandler) ListReturns(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.returns.List(r.Context(), identity.OrganizationID, returnsapp.ListFilter{
		Status: strings.TrimSpace(q.Get("status")),
		Type:   strings.TrimSpace(q.Get("type")),
		Limit:  atoiDefault(q.Get("limit"), 50),
		Offset: atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

func (h *MoneyHandler) GetReturn(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	ret, err := h.returns.Get(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, ret)
}

type createReturnRequest struct {
	OrderID      string `json:"order_id"`
	ShipmentID   string `json:"shipment_id"`
	Type         string `json:"type"`
	Reason       string `json:"reason"`
	RefundAmount int64  `json:"refund_amount"`
	Items        []struct {
		OrderItemID string `json:"order_item_id"`
		Name        string `json:"name"`
		Qty         int    `json:"qty"`
	} `json:"items"`
}

func (h *MoneyHandler) CreateReturn(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[createReturnRequest](w, r)
	if !ok {
		return
	}
	in := returnsapp.CreateInput{
		OrderID: req.OrderID, ShipmentID: req.ShipmentID,
		Type: req.Type, Reason: req.Reason, Actor: identity.UserID,
		RefundAmount: req.RefundAmount,
	}
	for _, it := range req.Items {
		in.Items = append(in.Items, struct {
			OrderItemID string
			Name        string
			Qty         int
		}{it.OrderItemID, it.Name, it.Qty})
	}
	ret, err := h.returns.Create(r.Context(), identity.OrganizationID, requestID(r), in)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, ret)
}

type decideReturnRequest struct {
	Note string `json:"note"`
}

func (h *MoneyHandler) ApproveReturn(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

func (h *MoneyHandler) RejectReturn(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

func (h *MoneyHandler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[decideReturnRequest](w, r)
	if !ok {
		return
	}
	ret, err := h.returns.Decide(r.Context(), identity.OrganizationID,
		chi.URLParam(r, "id"), requestID(r), identity.UserID, approve, req.Note)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, ret)
}

func (h *MoneyHandler) ReceiveReturn(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	ret, err := h.returns.Receive(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, ret)
}

type inspectReturnRequest struct {
	Restock bool `json:"restock"`
	Items   []struct {
		ItemID    string `json:"item_id"`
		Condition string `json:"condition"`
	} `json:"items"`
}

func (h *MoneyHandler) InspectReturn(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[inspectReturnRequest](w, r)
	if !ok {
		return
	}
	in := returnsapp.InspectInput{Restock: req.Restock, Actor: identity.UserID}
	for _, it := range req.Items {
		in.Items = append(in.Items, struct {
			ItemID    string
			Condition string
		}{it.ItemID, it.Condition})
	}
	ret, err := h.returns.Inspect(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), requestID(r), in)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, ret)
}

type refundReturnRequest struct {
	Amount int64  `json:"amount"`
	Method string `json:"method"`
}

func (h *MoneyHandler) RefundReturn(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[refundReturnRequest](w, r)
	if !ok {
		return
	}
	ret, err := h.returns.Refund(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), requestID(r),
		returnsapp.RefundInput{Amount: req.Amount, Method: req.Method, Actor: identity.UserID})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, ret)
}

func (h *MoneyHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	p, err := h.returns.Policy(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, p)
}

type policyRequest struct {
	WindowDays       int      `json:"window_days"`
	AutoApproveLimit int64    `json:"auto_approve_limit"`
	Conditions       []string `json:"conditions"`
}

func (h *MoneyHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[policyRequest](w, r)
	if !ok {
		return
	}
	p, err := h.returns.UpdatePolicy(r.Context(), identity.OrganizationID, requestID(r), returnsapp.UpdatePolicyInput{
		WindowDays: req.WindowDays, AutoApproveLimit: req.AutoApproveLimit,
		Conditions: req.Conditions, Actor: identity.UserID,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, p)
}
