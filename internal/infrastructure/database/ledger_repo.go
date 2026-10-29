package database

import (
	"context"

	"github.com/angkut/angkut/apps/internal/domain/ledger"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// LedgerRepo menulis & membaca buku besar append-only. Tidak ada UPDATE/DELETE
// di sini — DB juga memblokirnya lewat trigger (defence in depth).
type LedgerRepo struct{ q Querier }

// Insert menyimpan satu entri ledger (di dalam transaksi bisnis pemanggil).
func (r *LedgerRepo) Insert(ctx context.Context, e *ledger.Entry) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO ledger_entries (
			id, organization_id, entry_date, account, debit, credit, ref_type, ref_id, memo, created_by, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,'')::uuid,$11)`,
		e.ID, e.OrganizationID, e.EntryDate, e.Account, e.Debit, e.Credit,
		e.RefType, e.RefID, e.Memo, e.CreatedBy, e.CreatedAt)
	return err
}

// ExistsForRef memeriksa apakah sudah ada entri untuk (account, ref_type,
// ref_id) — dasar idempotensi refund/settlement (replay = entri pertama).
func (r *LedgerRepo) ExistsForRef(ctx context.Context, orgID, account, refType, refID string) (bool, error) {
	var ok bool
	err := r.q.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM ledger_entries
		WHERE organization_id=$1 AND account=$2 AND ref_type=$3 AND ref_id=$4)`,
		orgID, account, refType, refID).Scan(&ok)
	return ok, err
}

// Balances mengembalikan saldo per akun untuk organisasi.
func (r *LedgerRepo) Balances(ctx context.Context, orgID string) ([]ledger.Balance, error) {
	rows, err := r.q.Query(ctx, `
		SELECT account, COALESCE(sum(debit),0), COALESCE(sum(credit),0)
		FROM ledger_entries WHERE organization_id=$1 GROUP BY account ORDER BY account`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ledger.Balance, 0, 8)
	for rows.Next() {
		var b ledger.Balance
		if err := rows.Scan(&b.Account, &b.Debit, &b.Credit); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// FinancialReport merangkum angka TASK-047 dari ledger (net logistics cost).
func (r *LedgerRepo) FinancialReport(ctx context.Context, orgID string) (*ledger.FinancialReport, error) {
	balances, err := r.Balances(ctx, orgID)
	if err != nil {
		return nil, err
	}
	rep := &ledger.FinancialReport{}
	for _, b := range balances {
		switch b.Account {
		case ledger.AccountShippingExpense:
			rep.ShippingExpense = b.Debit
		case ledger.AccountCODFee:
			rep.CODFee = b.Debit
		case ledger.AccountInsuranceFee:
			rep.InsuranceFee = b.Debit
		case ledger.AccountRefund:
			rep.Refunds = b.Debit
		case ledger.AccountCODCollected:
			rep.CODRevenue = b.Credit
		case ledger.AccountPaymentReceived:
			rep.PaymentReceived = b.Debit
		}
	}
	rep.NetLogisticsCost = rep.ShippingExpense + rep.CODFee + rep.InsuranceFee + rep.Refunds
	return rep, nil
}

// List mengembalikan halaman entri untuk audit finance.
func (r *LedgerRepo) List(ctx context.Context, orgID string, account string, limit, offset int) ([]ledger.Entry, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if account != "" {
		if !ledger.ValidAccount(account) {
			return nil, 0, shared.Validation(map[string]string{"account": "Akun ledger tidak dikenal."})
		}
		where += ` AND account=$` + itoa(n)
		args = append(args, account)
		n++
	}
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM ledger_entries WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	rows, err := r.q.Query(ctx, `SELECT id, organization_id, entry_date::text, account, debit, credit,
		ref_type, ref_id, COALESCE(memo,''), COALESCE(created_by::text,''), created_at
		FROM ledger_entries WHERE `+where+` ORDER BY created_at DESC, id LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ledger.Entry, 0, limit)
	for rows.Next() {
		var e ledger.Entry
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.EntryDate, &e.Account, &e.Debit, &e.Credit,
			&e.RefType, &e.RefID, &e.Memo, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
