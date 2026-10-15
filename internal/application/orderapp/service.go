// Package orderapp berisi logika aplikasi order: pembuatan transaksional
// (order + item dalam satu tx), penerbitan nomor, dan transisi status.
// Boundary transaksi ada di sini (aturan proyek), bukan di handler/repository.
package orderapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/order"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps adalah dependensi service order.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Orders order.Repository
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Service adalah service aplikasi order.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	orders order.Repository
	audit  audit.Recorder
	log    *slog.Logger
}

// New membuat service order.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, orders: d.Orders, audit: d.Audit, log: log}
}

// CreateItemInput adalah satu baris item dari pemanggil.
type CreateItemInput struct {
	VariantID   string
	ProductName string
	VariantName string
	Qty         int
	UnitPrice   int64
}

// CreateInput adalah permintaan membuat order.
type CreateInput struct {
	StoreID           string
	CustomerID        string
	ShippingAddressID string
	PaymentType       order.PaymentType
	ShippingCost      int64
	Discount          int64
	Notes             string
	ExternalOrderID   string
	Items             []CreateItemInput
}

// Create membuat order beserta itemnya dalam SATU transaksi. Nomor order
// diterbitkan di dalam transaksi yang sama; bila bentrok (unique index),
// pembuatan diulang beberapa kali dengan nomor berikutnya.
func (s *Service) Create(ctx context.Context, orgID, actorUserID, requestID string, in CreateInput) (*order.Order, error) {
	paymentType := in.PaymentType
	if paymentType == "" {
		paymentType = order.PaymentPrepaid
	}

	now := time.Now().UTC()
	o := &order.Order{
		ID:                platform.NewUUID(),
		OrganizationID:    orgID,
		StoreID:           strings.TrimSpace(in.StoreID),
		CustomerID:        strings.TrimSpace(in.CustomerID),
		ExternalOrderID:   strings.TrimSpace(in.ExternalOrderID),
		Status:            order.StatusDraft,
		PaymentType:       paymentType,
		ShippingCost:      in.ShippingCost,
		Discount:          in.Discount,
		ShippingAddressID: strings.TrimSpace(in.ShippingAddressID),
		Notes:             strings.TrimSpace(in.Notes),
		CreatedAt:         now,
		UpdatedAt:         now,
		Items:             make([]order.Item, 0, len(in.Items)),
	}
	for _, it := range in.Items {
		o.Items = append(o.Items, order.Item{
			ID:                  platform.NewUUID(),
			OrganizationID:      orgID,
			OrderID:             o.ID,
			VariantID:           strings.TrimSpace(it.VariantID),
			ProductNameSnapshot: strings.TrimSpace(it.ProductName),
			VariantNameSnapshot: strings.TrimSpace(it.VariantName),
			Qty:                 it.Qty,
			UnitPrice:           it.UnitPrice,
			CreatedAt:           now,
		})
	}
	// Total dihitung server dari item — bukan dikirim klien.
	o.Recalculate()
	// Validasi input saja: nomor order diterbitkan di dalam transaksi berikutnya.
	if err := o.ValidateInput(); err != nil {
		return nil, err
	}

	if err := s.createWithNumber(ctx, o); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionOrderCreated, o)
	s.log.Info("order dibuat", "order_id", o.ID, "number", o.OrderNumber, "org", orgID)
	return o, nil
}

// createWithNumber menerbitkan nomor lalu menyimpan dalam satu transaksi.
func (s *Service) createWithNumber(ctx context.Context, o *order.Order) error {
	const maxAttempts = 3
	year := o.CreatedAt.Year()

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lastErr = s.tx.InTx(ctx, func(q database.Querier) error {
			tx := s.stores.WithTx(q)
			seq, err := tx.Orders().NextOrderNumber(ctx, o.OrganizationID, year)
			if err != nil {
				return err
			}
			o.OrderNumber = order.FormatOrderNumber(year, seq)
			return tx.Orders().Create(ctx, o)
		})
		if lastErr == nil {
			return nil
		}
		if !shared.Is(lastErr, shared.ErrConflict) {
			return lastErr
		}
		// Nomor bentrok (request paralel): ulangi dengan nomor berikutnya.
		s.log.Warn("nomor order bentrok — mencoba lagi", "attempt", attempt+1)
	}
	return lastErr
}

// Get mengambil satu order.
func (s *Service) Get(ctx context.Context, orgID, id string) (*order.Order, error) {
	return s.orders.GetByID(ctx, orgID, id)
}

// List mengembalikan halaman order.
func (s *Service) List(ctx context.Context, orgID string, f order.ListFilter) ([]order.Order, int, error) {
	return s.orders.List(ctx, orgID, f)
}

// Transition memindahkan status order dengan menegakkan state machine.
// Transisi ke status yang sama adalah no-op idempotent.
func (s *Service) Transition(ctx context.Context, orgID, actorUserID, requestID, id string, target order.Status, reason string) (*order.Order, error) {
	o, err := s.orders.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	from := o.Status
	if err := o.Transition(target); err != nil {
		return nil, err
	}
	if from == target {
		return o, nil
	}
	if err := s.orders.UpdateStatus(ctx, orgID, id, from, target, reason); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionOrderStatusChanged, o)
	return o, nil
}

// Summary adalah ringkasan order untuk dasbor.
type Summary struct {
	Today       int            `json:"today"`
	StatusCount map[string]int `json:"status_counts"`
	TotalValue  int64          `json:"total_value"`
}

// GetSummary menghitung ringkasan order satu organisasi. Satu query agregat —
// bukan loop per status yang menghasilkan N query.
func (s *Service) GetSummary(ctx context.Context, orgID string) (Summary, error) {
	out := Summary{StatusCount: make(map[string]int)}
	for _, st := range order.AllStatuses {
		out.StatusCount[string(st)] = 0
	}

	rows, err := s.stores.Querier().Query(ctx, `
		SELECT status, count(*)::int, COALESCE(sum(total_amount),0)::bigint
		  FROM orders
		 WHERE organization_id=$1 AND deleted_at IS NULL
		 GROUP BY status`, orgID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		var value int64
		if err := rows.Scan(&status, &count, &value); err != nil {
			return out, err
		}
		out.StatusCount[status] = count
		out.TotalValue += value
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	if err := s.stores.Querier().QueryRow(ctx, `
		SELECT count(*)::int FROM orders
		 WHERE organization_id=$1 AND deleted_at IS NULL
		   AND created_at >= date_trunc('day', now())`, orgID).Scan(&out.Today); err != nil {
		return out, err
	}
	return out, nil
}

// recordAudit mencatat peristiwa order. Kegagalan audit TIDAK menggagalkan
// operasi bisnis (aturan audit.Recorder) — hanya dicatat ke log.
func (s *Service) recordAudit(ctx context.Context, orgID, actorUserID, requestID, action string, o *order.Order) {
	if s.audit == nil {
		return
	}
	err := s.audit.Record(ctx, audit.Entry{
		OrganizationID: orgID,
		ActorUserID:    actorUserID,
		Action:         action,
		EntityType:     "order",
		EntityID:       o.ID,
		Changes: map[string]any{
			"order_number": o.OrderNumber,
			"status":       string(o.Status),
			"total_amount": o.TotalAmount,
		},
		RequestID: requestID,
	})
	if err != nil {
		s.log.Warn("audit order gagal", "order_id", o.ID, "error", err)
	}
}
