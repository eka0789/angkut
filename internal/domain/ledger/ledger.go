// Package ledger berisi model buku besar append-only (TASK-043) dan katalog
// akun. Koreksi finansial = ENTRI PEMBALIK (reversal) yang mereferensikan
// entri asli — UPDATE/DELETE diblokir trigger DB.
package ledger

import (
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Katalog akun (whitelist — tidak ada akun liar dari input).
const (
	AccountCODReceivable   = "cod_receivable"      // uang COD di tangan kurir (debit)
	AccountCODCollected    = "cod_collected"       // pendapatan COD dikumpulkan (credit)
	AccountSettlementClear = "settlement_clearing" // penyelesaian COD (debit saat paid)
	AccountShippingExpense = "shipping_expense"    // ongkos kirim (debit)
	AccountCODFee          = "cod_fee"             // fee COD kurir (debit)
	AccountInsuranceFee    = "insurance_fee"       // biaya asuransi (debit)
	AccountRefund          = "return_refund"       // refund retur (debit)
	AccountPaymentReceived = "payment_received"    // pembayaran invoice (debit)
	AccountRevenue         = "revenue"             // pendapatan invoice (credit)
)

// ValidAccount melaporkan apakah akun dikenal — sumber input TIDAK dipercaya.
func ValidAccount(a string) bool {
	switch a {
	case AccountCODReceivable, AccountCODCollected, AccountSettlementClear,
		AccountShippingExpense, AccountCODFee, AccountInsuranceFee,
		AccountRefund, AccountPaymentReceived, AccountRevenue:
		return true
	}
	return false
}

// Entry adalah satu baris buku besar.
type Entry struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	EntryDate      string    `json:"entry_date"` // YYYY-MM-DD
	Account        string    `json:"account"`
	Debit          int64     `json:"debit"`
	Credit         int64     `json:"credit"`
	RefType        string    `json:"ref_type"`
	RefID          string    `json:"ref_id"`
	Memo           string    `json:"memo,omitempty"`
	CreatedBy      string    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// New memvalidasi lalu membangun entri: tepat satu sisi (debit XOR credit)
// bernilai > 0, akun harus whitelisted.
func New(orgID, account, refType, refID, memo, actor string, debit, credit int64, at time.Time) (*Entry, error) {
	if !ValidAccount(account) {
		return nil, shared.Validation(map[string]string{"account": "Akun ledger tidak dikenal."})
	}
	if (debit > 0 && credit > 0) || (debit == 0 && credit == 0) {
		return nil, shared.Validation(map[string]string{
			"debit":  "Tepat satu sisi harus terisi (debit XOR credit).",
			"credit": "Tepat satu sisi harus terisi (debit XOR credit).",
		})
	}
	if debit < 0 || credit < 0 {
		return nil, shared.Validation(map[string]string{
			"debit":  "Nilai tidak boleh negatif.",
			"credit": "Nilai tidak boleh negatif.",
		})
	}
	return &Entry{
		OrganizationID: orgID, Account: account,
		Debit: debit, Credit: credit,
		RefType: refType, RefID: refID, Memo: memo, CreatedBy: actor,
		EntryDate: at.Format("2006-01-02"), CreatedAt: at,
	}, nil
}

// Balance adalah saldo satu akun.
type Balance struct {
	Account string `json:"account"`
	Debit   int64  `json:"debit"`
	Credit  int64  `json:"credit"`
}

// Net mengembalikan saldo bersih sisi debit − credit.
func (b Balance) Net() int64 { return b.Debit - b.Credit }

// FinancialReport adalah ringkasan keuangan (TASK-047: net logistics cost).
type FinancialReport struct {
	ShippingExpense int64 `json:"shipping_expense"`
	CODFee          int64 `json:"courier_fees"`
	InsuranceFee    int64 `json:"insurance"`
	Refunds         int64 `json:"refunds"`
	CODRevenue      int64 `json:"cod_revenue"`
	PaymentReceived int64 `json:"payment_received"`
	// NetLogisticsCost = ongkos kirim + fee COD + asuransi + refund yang
	// kita keluarkan — angka yang benar-benar membebani seller.
	NetLogisticsCost int64 `json:"net_logistics_cost"`
}
