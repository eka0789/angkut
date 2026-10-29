// Package cod berisi entitas & aturan COD (TASK-041/042) yang murni:
// transaksi tagihan COD per pengiriman, settlement per kurir & periode,
// dan state machine-nya.
//
// Aturan yang dijaga:
//   - settlement TIDAK mengubah transaksi lama — hanya memindahkan status
//     pending/collected → settling (entri ledger ditulis saat mark-paid);
//   - transaksi yang sudah settling/settled tidak boleh masuk settlement lain;
//   - semua penggabungan ATOMIK: satu transaksi gagal = seluruh settlement
//     batal (pola yang sama dengan transfer stok).
package cod

import (
	"fmt"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Status transaksi COD.
type Status string

const (
	StatusPending   Status = "pending"   // pengiriman berjalan, uang belum diterima kurir
	StatusCollected Status = "collected" // paket terkirim — kurir memegang uang
	StatusSettling  Status = "settling"  // masuk settlement, menunggu transfer kurir
	StatusSettled   Status = "settled"   // kurir sudah transfer (mark-paid)
	StatusFailed    Status = "failed"    // pengiriman gagal/cancel — tidak jadi tagihan
)

// Valid melaporkan apakah status dikenal.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusCollected, StatusSettling, StatusSettled, StatusFailed:
		return true
	}
	return false
}

// SettlementStatus adalah status rekonsiliasi.
type SettlementStatus string

const (
	SettlementPending SettlementStatus = "pending" // menunggu transfer kurir
	SettlementPaid    SettlementStatus = "paid"    // sudah diterima (mark-paid)
)

// Transaction adalah satu tagihan COD untuk satu pengiriman.
type Transaction struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	ShipmentID     string     `json:"shipment_id"`
	CourierCode    string     `json:"courier_code"`
	Amount         int64      `json:"amount"`
	Fee            int64      `json:"fee"`
	Status         Status     `json:"status"`
	SettlementID   string     `json:"settlement_id,omitempty"`
	CollectedAt    *time.Time `json:"collected_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Net adalah nilai yang seharusnya diterima seller (amount − fee kurir).
func (t Transaction) Net() int64 { return t.Amount - t.Fee }

// Settlement adalah rekonsiliasi sekumpulan transaksi COD per kurir.
type Settlement struct {
	ID             string           `json:"id"`
	OrganizationID string           `json:"organization_id"`
	CourierCode    string           `json:"courier_code"`
	PeriodStart    string           `json:"period_start"` // YYYY-MM-DD
	PeriodEnd      string           `json:"period_end"`   // YYYY-MM-DD
	Status         SettlementStatus `json:"status"`
	TotalAmount    int64            `json:"total_amount"`
	TotalFee       int64            `json:"total_fee"`
	// NetAmount dihitung DB (GENERATED): total − fee.
	Reference string `json:"reference,omitempty"`
	// IdempotencyKey tidak diekspos ke klien (dipakai replay submit).
	IdempotencyKey string        `json:"-"`
	PaidAt         *time.Time    `json:"paid_at,omitempty"`
	ProofURL       string        `json:"proof_url,omitempty"`
	CreatedBy      string        `json:"created_by,omitempty"`
	Items          []Transaction `json:"items,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// CreateSettlementInput adalah permintaan membuat settlement.
type CreateSettlementInput struct {
	OrganizationID string
	CourierCode    string
	PeriodStart    string
	PeriodEnd      string
	TransactionIDs []string
	Actor          string
}

// ValidateCreate memeriksa aturan settlement sebelum menyentuh DB.
func ValidateCreate(in CreateSettlementInput) error {
	fields := make(map[string]string)
	if strings.TrimSpace(in.CourierCode) == "" {
		fields["courier_code"] = "Kurir wajib dipilih."
	}
	if !validDate(in.PeriodStart) {
		fields["period_start"] = "Periode awal harus format YYYY-MM-DD."
	}
	if !validDate(in.PeriodEnd) {
		fields["period_end"] = "Periode akhir harus format YYYY-MM-DD."
	}
	if validDate(in.PeriodStart) && validDate(in.PeriodEnd) && in.PeriodEnd < in.PeriodStart {
		fields["period_end"] = "Periode akhir tidak boleh sebelum awal."
	}
	if len(in.TransactionIDs) == 0 {
		fields["transaction_ids"] = "Pilih minimal satu transaksi COD."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// MarkPaidInput adalah permintaan menandai settlement lunas.
type MarkPaidInput struct {
	Reference string
	PaidAt    *time.Time
	ProofURL  string
	Actor     string
}

func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// Summary adalah rekap COD untuk dashboard finance.
type Summary struct {
	Total              int64 `json:"total"`
	Pending            int64 `json:"pending"`
	Collected          int64 `json:"collected"`
	Settling           int64 `json:"settling"`
	Settled            int64 `json:"settled"`
	ExpectedSettlement int64 `json:"expected_settlement"` // collected + settling (uang di kurir)
	Count              int64 `json:"count"`
}

// ErrTransaksiTidakSettleable dipakai saat transaksi tidak boleh masuk
// settlement (status salah / milik org lain / kurir berbeda).
func ErrTransaksiTidakSettleable(id string) error {
	return shared.New("COD_TRANSACTION_NOT_SETTLEABLE",
		fmt.Sprintf("Transaksi %s tidak bisa dimasukkan settlement (status salah atau bukan milik org/kurir ini).", id),
		shared.ErrValidation)
}

// ErrAlreadySettled dilempar saat mark-paid pada settlement yang sudah paid.
func ErrAlreadySettled() error {
	return shared.Conflict("COD_SETTLEMENT_ALREADY_PAID",
		"Settlement ini sudah ditandai lunas.")
}
