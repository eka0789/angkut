package database

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// ── Invoice ─────────────────────────────────────────────────────────────

// InvoiceRepo menyimpan invoice per order (TASK-044, provider mock).
type InvoiceRepo struct{ q Querier }

const invoiceCols = `id, organization_id, number, COALESCE(order_id::text,''), amount, status, created_at, updated_at`

// CreateInvoice menyimpan invoice; nomor diterbitkan pemanggil (dalam tx).
func (r *InvoiceRepo) CreateInvoice(ctx context.Context, inv *Invoice) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO invoices (id, organization_id, number, order_id, amount, status, created_at, updated_at)
		VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8)`,
		inv.ID, inv.OrganizationID, inv.Number, inv.OrderID, inv.Amount,
		string(inv.Status), inv.CreatedAt, inv.UpdatedAt)
	if isUniqueViolationOn(err, "uq_invoice_number") {
		return shared.Conflict("INVOICE_NUMBER_TAKEN", "Nomor invoice sudah dipakai.")
	}
	return err
}

// Invoice adalah satu tagihan.
type Invoice struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Number         string    `json:"number"`
	OrderID        string    `json:"order_id,omitempty"`
	Amount         int64     `json:"amount"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GetInvoice mengambil invoice milik organisasi.
func (r *InvoiceRepo) GetInvoice(ctx context.Context, orgID, id string) (*Invoice, error) {
	row := r.q.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE organization_id=$1 AND id=$2`, orgID, id)
	return scanInvoice(row)
}

func scanInvoice(row pgx.Row) (*Invoice, error) {
	var inv Invoice
	var status string
	err := row.Scan(&inv.ID, &inv.OrganizationID, &inv.Number, &inv.OrderID,
		&inv.Amount, &status, &inv.CreatedAt, &inv.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("INVOICE_NOT_FOUND", "Invoice tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	inv.Status = status
	return &inv, nil
}

// ListInvoices mengembalikan halaman invoice.
func (r *InvoiceRepo) ListInvoices(ctx context.Context, orgID string, limit, offset int) ([]Invoice, int, error) {
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE organization_id=$1`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.q.Query(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE organization_id=$1
		ORDER BY created_at DESC LIMIT `+itoa(limit)+` OFFSET `+itoa(offset), orgID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]Invoice, 0, limit)
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *inv)
	}
	return out, total, rows.Err()
}

// MarkInvoicePaid memindahkan issued → paid.
func (r *InvoiceRepo) MarkInvoicePaid(ctx context.Context, orgID, id string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE invoices SET status='paid' WHERE organization_id=$1 AND id=$2 AND status='issued'`,
		orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.Conflict("INVOICE_NOT_PAYABLE", "Invoice tidak dalam status issued.")
	}
	return nil
}

// NextInvoiceNumber mengembalikan nomor urut invoice berikutnya per org
// (INV-YYYY-NNNNNN) — dipanggil DI DALAM transaksi order.
func (r *InvoiceRepo) NextInvoiceNumber(ctx context.Context, orgID string, year int) (string, error) {
	prefix := "INV-" + strconv.Itoa(year) + "-"
	var n int
	err := r.q.QueryRow(ctx, `
		SELECT count(*) FROM invoices
		WHERE organization_id=$1 AND number LIKE $2 || '%'`, orgID, prefix).Scan(&n)
	if err != nil {
		return "", err
	}
	return prefix + padLeft(n+1, 6), nil
}

func padLeft(v, width int) string {
	s := strconv.Itoa(v)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// ── Payment ─────────────────────────────────────────────────────────────

// PaymentRepo menyimpan pembayaran; replay provider_ref = payment yang sama.
type PaymentRepo struct{ q Querier }

// Payment adalah satu pembayaran masuk.
type Payment struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	InvoiceID      string         `json:"invoice_id,omitempty"`
	Provider       string         `json:"provider"`
	ProviderRef    string         `json:"provider_ref"`
	Amount         int64          `json:"amount"`
	Payload        map[string]any `json:"data,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// CreatePayment menyimpan payment; bentrok unik (org, provider, ref) = replay
// → kembalikan payment yang sudah ada (replayed=true), bukan duplikat.
func (r *PaymentRepo) CreatePayment(ctx context.Context, p *Payment) (existing *Payment, replayed bool, err error) {
	payload, jerr := json.Marshal(p.Payload)
	if jerr != nil {
		return nil, false, jerr
	}
	tag, err := r.q.Exec(ctx, `
		INSERT INTO payments (id, organization_id, invoice_id, provider, provider_ref, amount, payload, created_by, created_at)
		VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7::jsonb,NULLIF($8,'')::uuid,$9)
		ON CONFLICT (organization_id, provider, provider_ref) DO NOTHING`,
		p.ID, p.OrganizationID, p.InvoiceID, p.Provider, p.ProviderRef, p.Amount,
		payload, p.CreatedAt, p.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	if tag.RowsAffected() == 0 {
		got, gerr := r.GetByRef(ctx, p.OrganizationID, p.Provider, p.ProviderRef)
		return got, gerr == nil, gerr
	}
	return p, false, nil
}

// GetByRef mengambil payment berdasar referensi provider.
func (r *PaymentRepo) GetByRef(ctx context.Context, orgID, provider, ref string) (*Payment, error) {
	row := r.q.QueryRow(ctx, `SELECT id, organization_id, COALESCE(invoice_id::text,''), provider,
		provider_ref, amount, payload, created_at
		FROM payments WHERE organization_id=$1 AND provider=$2 AND provider_ref=$3`, orgID, provider, ref)
	var p Payment
	var payload []byte
	err := row.Scan(&p.ID, &p.OrganizationID, &p.InvoiceID, &p.Provider,
		&p.ProviderRef, &p.Amount, &payload, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("PAYMENT_NOT_FOUND", "Pembayaran tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &p.Payload)
	}
	return &p, nil
}

// ListPayments mengembalikan halaman payment.
func (r *PaymentRepo) ListPayments(ctx context.Context, orgID string, limit, offset int) ([]Payment, int, error) {
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM payments WHERE organization_id=$1`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.q.Query(ctx, `SELECT id, organization_id, COALESCE(invoice_id::text,''), provider,
		provider_ref, amount, payload, created_at
		FROM payments WHERE organization_id=$1 ORDER BY created_at DESC LIMIT `+itoa(limit)+` OFFSET `+itoa(offset), orgID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]Payment, 0, limit)
	for rows.Next() {
		var p Payment
		var payload []byte
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.InvoiceID, &p.Provider,
			&p.ProviderRef, &p.Amount, &payload, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &p.Payload)
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}
