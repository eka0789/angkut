// Package returns berisi model & state machine retur (TASK-045/046).
//
// State machine (ketat, lompatan ditolak):
//
//	requested ──approve──► approved ──receive──► received ──inspect──► inspected
//	    │                      │
//	    └──reject──► rejected  └──(cancel tidak disediakan di kontrak)
//	inspected ──refund──► refunded
package returns

import (
	"fmt"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Status retur.
type Status string

const (
	StatusRequested Status = "requested"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
	StatusReceived  Status = "received"
	StatusInspected Status = "inspected"
	StatusRefunded  Status = "refunded"
)

// Tipe retur.
type Type string

const (
	TypeRefund   Type = "refund"
	TypeExchange Type = "exchange"
)

// Kondisi item saat inspect.
type Condition string

const (
	ConditionGood    Condition = "good"
	ConditionDamaged Condition = "damaged"
	ConditionLost    Condition = "lost"
	ConditionUnknown Condition = "unknown"
)

// ValidCondition melaporkan apakah kondisi dikenal.
func ValidCondition(c string) bool {
	switch Condition(c) {
	case ConditionGood, ConditionDamaged, ConditionLost, ConditionUnknown:
		return true
	}
	return false
}

// Return adalah satu permintaan retur.
type Return struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	OrderID        string     `json:"order_id,omitempty"`
	ShipmentID     string     `json:"shipment_id,omitempty"`
	Type           Type       `json:"type"`
	Reason         string     `json:"reason,omitempty"`
	Status         Status     `json:"status"`
	RefundAmount   int64      `json:"refund_amount"`
	RefundMethod   string     `json:"refund_method,omitempty"`
	DecidedBy      string     `json:"decided_by,omitempty"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	RefundedAt     *time.Time `json:"refunded_at,omitempty"`
	CreatedBy      string     `json:"created_by,omitempty"`
	Items          []Item     `json:"items,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Item adalah barang dalam retur.
type Item struct {
	ID           string    `json:"id"`
	OrderItemID  string    `json:"order_item_id,omitempty"`
	NameSnapshot string    `json:"name_snapshot,omitempty"`
	Qty          int       `json:"qty"`
	Condition    Condition `json:"condition"`
	Restocked    bool      `json:"restocked"`
}

// CreateInput adalah permintaan membuat retur.
type CreateInput struct {
	OrganizationID string
	OrderID        string
	ShipmentID     string
	Type           string
	Reason         string
	Actor          string
	Items          []Item
}

// ValidateCreate memeriksa bentuk permintaan retur.
func ValidateCreate(in CreateInput) error {
	fields := make(map[string]string)
	if strings.TrimSpace(in.OrderID) == "" && strings.TrimSpace(in.ShipmentID) == "" {
		fields["order_id"] = "Retur harus merujuk order atau pengiriman."
	}
	t := Type(in.Type)
	if t != "" && t != TypeRefund && t != TypeExchange {
		fields["type"] = "Tipe retur harus refund atau exchange."
	}
	if strings.TrimSpace(in.Reason) == "" {
		fields["reason"] = "Alasan retur wajib diisi."
	}
	if len(in.Items) == 0 {
		fields["items"] = "Minimal satu item untuk diretur."
	}
	for i, it := range in.Items {
		if it.Qty <= 0 {
			fields[fmt.Sprintf("items.%d.qty", i)] = "Jumlah harus lebih dari 0."
		}
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Transition memeriksa kebolehan perpindahan status (state machine ketat).
func (r *Return) Transition(target Status) error {
	allowed := false
	switch r.Status {
	case StatusRequested:
		allowed = target == StatusApproved || target == StatusRejected
	case StatusApproved:
		allowed = target == StatusReceived
	case StatusReceived:
		allowed = target == StatusInspected
	case StatusInspected:
		allowed = target == StatusRefunded
	}
	if !allowed {
		return shared.New("RETURN_INVALID_TRANSITION",
			fmt.Sprintf("Retur tidak bisa berpindah dari %s ke %s.", r.Status, target),
			shared.ErrConflict)
	}
	return nil
}

// Policy adalah kebijakan retur satu organisasi (TASK-046).
type Policy struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organization_id"`
	WindowDays       int       `json:"window_days"`
	AutoApproveLimit int64     `json:"auto_approve_limit"`
	Conditions       []string  `json:"conditions,omitempty"`
	UpdatedBy        string    `json:"updated_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DefaultPolicy mengembalikan kebijakan bawaan bila org belum mengaturnya.
func DefaultPolicy(orgID string) Policy {
	return Policy{OrganizationID: orgID, WindowDays: 7, AutoApproveLimit: 0, Conditions: []string{}}
}

// Validate memeriksa invariant kebijakan.
func (p *Policy) Validate() error {
	fields := make(map[string]string)
	if p.WindowDays < 0 {
		fields["window_days"] = "Jeda hari tidak boleh negatif."
	}
	if p.AutoApproveLimit < 0 {
		fields["auto_approve_limit"] = "Batas auto-approve tidak boleh negatif."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Evaluate memutuskan apakah retur otomatis disetujui sesuai kebijakan:
// jumlah refund ≤ auto_approve_limit (limit 0 = selalu review manual).
func (p Policy) Evaluate(refundAmount int64) (autoApprove bool) {
	return p.AutoApproveLimit > 0 && refundAmount <= p.AutoApproveLimit
}

// ErrWindowClosed dilempar saat retur melewati jeda hari kebijakan.
func ErrWindowClosed(days int) error {
	return shared.New("RETURN_WINDOW_CLOSED",
		fmt.Sprintf("Batas retur %d hari sudah terlewat.", days),
		shared.ErrValidation)
}

// Valid melaporkan apakah status retur dikenal.
func (s Status) Valid() bool {
	switch s {
	case StatusRequested, StatusApproved, StatusRejected, StatusReceived,
		StatusInspected, StatusRefunded:
		return true
	}
	return false
}

// Valid melaporkan apakah tipe retur dikenal.
func (t Type) Valid() bool {
	return t == TypeRefund || t == TypeExchange
}
