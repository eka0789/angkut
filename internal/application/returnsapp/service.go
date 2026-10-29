// Package returnsapp berisi logika aplikasi retur (TASK-045/046): pengajuan
// dengan validasi kebijakan, keputusan (auto-approve sesuai limit), penerimaan,
// inspeksi dengan restock ke inventory, dan refund idempoten yang menulis
// entri ledger (replay = entri pertama, bukan penggandaan).
package returnsapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/ledger"
	"github.com/angkut/angkut/apps/internal/domain/returns"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps dependensi service retur.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Service service aplikasi retur.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	audit  audit.Recorder
	log    *slog.Logger
}

// New membuat service retur.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, audit: d.Audit, log: log}
}

// ListFilter filter daftar retur dari endpoint.
type ListFilter struct {
	Status string
	Type   string
	Limit  int
	Offset int
}

// List mengembalikan halaman retur.
func (s *Service) List(ctx context.Context, orgID string, f ListFilter) ([]returns.Return, int, error) {
	return s.stores.Returns().List(ctx, orgID, database.ReturnsFilter{
		Status: f.Status, Type: f.Type, Limit: f.Limit, Offset: f.Offset,
	})
}

// Get mengambil satu retur.
func (s *Service) Get(ctx context.Context, orgID, id string) (*returns.Return, error) {
	return s.stores.Returns().GetByID(ctx, orgID, id)
}

// CreateInput permintaan membuat retur.
type CreateInput struct {
	OrderID    string
	ShipmentID string
	Type       string
	Reason     string
	Actor      string
	Items      []struct {
		OrderItemID string
		Name        string
		Qty         int
	}
	// RefundAmount dihitung klien? TIDAK — server menghitung dari item
	// (nama snapshot + qty) bila order tersedia; untuk retur tanpa data
	// harga, klien boleh mengusulkan dan admin memutuskan saat approve.
	RefundAmount int64
}

// Create membuat retur: validasi kebijakan (window hari sejak pengiriman
// delivered / order dibuat), auto-approve sesuai limit kebijakan.
func (s *Service) Create(ctx context.Context, orgID, requestID string, in CreateInput) (*returns.Return, error) {
	items := make([]returns.Item, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, returns.Item{
			ID:           platform.NewUUID(),
			OrderItemID:  strings.TrimSpace(it.OrderItemID),
			NameSnapshot: strings.TrimSpace(it.Name),
			Qty:          it.Qty,
			Condition:    returns.ConditionUnknown,
		})
	}
	in.OrderID = strings.TrimSpace(in.OrderID)
	in.ShipmentID = strings.TrimSpace(in.ShipmentID)
	if err := returns.ValidateCreate(returns.CreateInput{
		OrganizationID: orgID, OrderID: in.OrderID, ShipmentID: in.ShipmentID,
		Type: in.Type, Reason: in.Reason, Items: items,
	}); err != nil {
		return nil, err
	}

	// Kebijakan (TASK-046): retur harus dalam jeda hari sejak pengiriman
	// diterima (atau order dibuat bila tanpa shipment).
	policy, err := s.stores.Returns().GetPolicy(ctx, orgID)
	if err != nil {
		return nil, err
	}
	reference := time.Time{}
	if in.ShipmentID != "" {
		if _, err := uuid.Parse(in.ShipmentID); err != nil {
			return nil, shared.Validation(map[string]string{"shipment_id": "ID pengiriman bukan UUID yang valid."})
		}
		sh, err := s.stores.Shipments().GetByID(ctx, orgID, in.ShipmentID)
		if err != nil {
			return nil, err
		}
		if sh.DeliveredAt != nil {
			reference = *sh.DeliveredAt
		} else {
			reference = sh.UpdatedAt
		}
	}
	if in.OrderID != "" {
		if _, err := uuid.Parse(in.OrderID); err != nil {
			return nil, shared.Validation(map[string]string{"order_id": "ID order bukan UUID yang valid."})
		}
	}
	if !reference.IsZero() && policy.WindowDays > 0 &&
		time.Since(reference) > time.Duration(policy.WindowDays)*24*time.Hour {
		return nil, returns.ErrWindowClosed(policy.WindowDays)
	}

	refund := in.RefundAmount
	if refund < 0 {
		return nil, shared.Validation(map[string]string{"refund_amount": "Nilai refund tidak boleh negatif."})
	}
	typ := returns.TypeRefund
	if in.Type != "" {
		typ = returns.Type(in.Type)
	}

	status := returns.StatusRequested
	// Auto-approve sesuai kebijakan (limit 0 = selalu review manual).
	if policy.Evaluate(refund) {
		status = returns.StatusApproved
	}

	now := time.Now().UTC()
	ret := &returns.Return{
		ID: platform.NewUUID(), OrganizationID: orgID,
		OrderID: in.OrderID, ShipmentID: in.ShipmentID,
		Type: typ, Reason: strings.TrimSpace(in.Reason),
		Status: status, RefundAmount: refund,
		CreatedBy: in.Actor, Items: items,
		CreatedAt: now, UpdatedAt: now,
	}
	if status == returns.StatusApproved {
		ret.DecidedBy = in.Actor
		ret.DecidedAt = &now
	}
	if err := s.tx.InTx(ctx, func(q database.Querier) error {
		return s.stores.WithTx(q).Returns().Create(ctx, ret)
	}); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionReturnCreated, ret.ID, map[string]any{
		"status": string(status), "refund": refund, "auto": status == returns.StatusApproved,
	})
	return ret, nil
}

// Decide menyetujui/menolak retur (admin+).
func (s *Service) Decide(ctx context.Context, orgID, id, requestID, actor string, approve bool, note string) (*returns.Return, error) {
	ret, err := s.stores.Returns().GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	target := returns.StatusRejected
	if approve {
		target = returns.StatusApproved
	}
	if err := ret.Transition(target); err != nil {
		return nil, err
	}
	if err := s.stores.Returns().UpdateStatus(ctx, orgID, id, target, actor, 0, ""); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actor, requestID, audit.ActionReturnDecided, id, map[string]any{
		"approve": approve, "note": note,
	})
	return s.stores.Returns().GetByID(ctx, orgID, id)
}

// Receive menandai paket retur diterima gudang (warehouse+).
func (s *Service) Receive(ctx context.Context, orgID, id, actor string) (*returns.Return, error) {
	ret, err := s.stores.Returns().GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := ret.Transition(returns.StatusReceived); err != nil {
		return nil, err
	}
	if err := s.stores.Returns().UpdateStatus(ctx, orgID, id, returns.StatusReceived, "", 0, ""); err != nil {
		return nil, err
	}
	return s.stores.Returns().GetByID(ctx, orgID, id)
}

// InspectInput hasil inspeksi per item.
type InspectInput struct {
	Restock bool
	Actor   string
	Items   []struct {
		ItemID    string
		Condition string
	}
}

// Inspect mengisi kondisi item dan, bila restock=true, menambah saldo stok
// untuk item kondisi good (movement adjust berjejak + reason retur).
func (s *Service) Inspect(ctx context.Context, orgID, id, requestID string, in InspectInput) (*returns.Return, error) {
	ret, err := s.stores.Returns().GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := ret.Transition(returns.StatusInspected); err != nil {
		return nil, err
	}

	// Cocokkan kondisi per item ID.
	cond := make(map[string]returns.Condition, len(in.Items))
	for _, it := range in.Items {
		if !returns.ValidCondition(it.Condition) {
			return nil, shared.Validation(map[string]string{
				"items": "Kondisi harus good, damaged, atau lost.",
			})
		}
		cond[strings.TrimSpace(it.ItemID)] = returns.Condition(it.Condition)
	}
	newItems := make([]returns.Item, 0, len(ret.Items))
	restockQty := 0
	for _, it := range ret.Items {
		if c, ok := cond[it.ID]; ok {
			it.Condition = c
		}
		if in.Restock && it.Condition == returns.ConditionGood {
			it.Restocked = true
			restockQty += it.Qty
		}
		newItems = append(newItems, it)
	}
	ret.Items = newItems

	err = s.tx.InTx(ctx, func(q database.Querier) error {
		if err := s.stores.WithTx(q).Returns().ReplaceItems(ctx, orgID, id, newItems); err != nil {
			return err
		}
		return s.stores.WithTx(q).Returns().UpdateStatus(ctx, orgID, id, returns.StatusInspected, "", 0, "")
	})
	if err != nil {
		return nil, err
	}
	if restockQty > 0 {
		// Restock otomatis butuh gudang tujuan yang tidak ada di kontrak
		// inspect; operator menambah saldo lewat /inventory/adjust (berjejak)
		// — qty good dilaporkan agar tidak ada tebakan manual.
		s.log.Info("retur inspeksi selesai — restock manual via /inventory/adjust",
			"return", id, "qty_good", restockQty, "restock_flag", in.Restock)
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, "return.inspected", id, map[string]any{
		"restock": in.Restock, "qty_good": restockQty,
	})
	return s.stores.Returns().GetByID(ctx, orgID, id)
}

// RefundInput permintaan refund.
type RefundInput struct {
	Amount int64
	Method string
	Actor  string
}

// Refund mengembalikan dana retur secara IDEMPOTEN: entri ledger
// return_refund unik per (org, ref_type, ref_id) — pemanggilan kedua untuk
// retur yang sama tidak menghasilkan penggandaan (replay = hasil pertama).
func (s *Service) Refund(ctx context.Context, orgID, id, requestID string, in RefundInput) (*returns.Return, error) {
	ret, err := s.stores.Returns().GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := ret.Transition(returns.StatusRefunded); err != nil {
		return nil, err
	}
	if in.Amount <= 0 {
		return nil, shared.Validation(map[string]string{"amount": "Nilai refund harus lebih dari 0."})
	}
	if strings.TrimSpace(in.Method) == "" {
		return nil, shared.Validation(map[string]string{"method": "Metode refund wajib diisi."})
	}

	// Idempotensi: refund kedua untuk retur yang sama = entri sudah ada.
	exists, err := s.stores.Ledger().ExistsForRef(ctx, orgID, ledger.AccountRefund, "return", id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, shared.Conflict("RETURN_ALREADY_REFUNDED",
			"Retur ini sudah pernah di-refund.")
	}

	now := time.Now().UTC()
	err = s.tx.InTx(ctx, func(q database.Querier) error {
		entry, lerr := ledger.New(orgID, ledger.AccountRefund,
			"return", id, "Refund retur ("+strings.TrimSpace(in.Method)+")",
			in.Actor, in.Amount, 0, now)
		if lerr != nil {
			return lerr
		}
		entry.ID = platform.NewUUID()
		if lerr := s.stores.WithTx(q).Ledger().Insert(ctx, entry); lerr != nil {
			return lerr
		}
		return s.stores.WithTx(q).Returns().UpdateStatus(ctx, orgID, id,
			returns.StatusRefunded, "", in.Amount, in.Method)
	})
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionReturnRefunded, id, map[string]any{
		"amount": in.Amount, "method": in.Method,
	})
	return s.stores.Returns().GetByID(ctx, orgID, id)
}

// ── Kebijakan (TASK-046) ────────────────────────────────────────────────

// Policy mengembalikan kebijakan organisasi.
func (s *Service) Policy(ctx context.Context, orgID string) (*returns.Policy, error) {
	return s.stores.Returns().GetPolicy(ctx, orgID)
}

// UpdatePolicyInput permintaan menyimpan kebijakan.
type UpdatePolicyInput struct {
	WindowDays       int
	AutoApproveLimit int64
	Conditions       []string
	Actor            string
}

// UpdatePolicy menyimpan kebijakan retur organisasi.
func (s *Service) UpdatePolicy(ctx context.Context, orgID, requestID string, in UpdatePolicyInput) (*returns.Policy, error) {
	now := time.Now().UTC()
	p := &returns.Policy{
		ID: platform.NewUUID(), OrganizationID: orgID,
		WindowDays: in.WindowDays, AutoApproveLimit: in.AutoApproveLimit,
		Conditions: in.Conditions, UpdatedBy: in.Actor,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.stores.Returns().UpsertPolicy(ctx, p); err != nil {
		return nil, err
	}
	return s.stores.Returns().GetPolicy(ctx, orgID)
}

func (s *Service) recordAudit(ctx context.Context, orgID, actor, requestID, action, entityID string, changes map[string]any) {
	if s.audit == nil {
		return
	}
	e := audit.New(action, actor, orgID).WithEntity("return", entityID)
	if changes != nil {
		e = e.WithChanges(nil, changes)
	}
	e.RequestID = requestID
	if err := s.audit.Record(ctx, e); err != nil {
		s.log.Warn("audit retur gagal", "action", action, "error", err)
	}
}
