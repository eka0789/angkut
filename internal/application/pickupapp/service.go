// Package pickupapp berisi logika aplikasi pickup request (TASK-036).
package pickupapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/pickup"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps adalah dependensi service pickup.
type Deps struct {
	Tx      *database.TxManager
	Stores  *database.Stores
	Pickups pickup.Repository
	Audit   audit.Recorder
	Log     *slog.Logger
}

// Service adalah service aplikasi pickup.
type Service struct {
	tx      *database.TxManager
	stores  *database.Stores
	pickups pickup.Repository
	audit   audit.Recorder
	log     *slog.Logger
}

// New membuat service pickup.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, pickups: d.Pickups, audit: d.Audit, log: log}
}

// CreateInput adalah permintaan membuat pickup request.
type CreateInput struct {
	WarehouseID string
	CourierCode string
	ScheduledAt *time.Time
	Notes       string
	ShipmentIDs []string
}

// Create membuat pickup request.
func (s *Service) Create(ctx context.Context, orgID, actorUserID, requestID string, in CreateInput) (*pickup.Request, error) {
	ids, err := normalizeShipmentIDs(in.ShipmentIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	pr := &pickup.Request{
		ID:             platform.NewUUID(),
		OrganizationID: orgID,
		WarehouseID:    strings.TrimSpace(in.WarehouseID),
		CourierCode:    strings.TrimSpace(in.CourierCode),
		ScheduledAt:    in.ScheduledAt,
		Status:         pickup.StatusRequested,
		Notes:          strings.TrimSpace(in.Notes),
		CreatedBy:      actorUserID,
		CreatedAt:      now,
		UpdatedAt:      now,
		ShipmentIDs:    ids,
	}
	if err := pr.Validate(); err != nil {
		return nil, err
	}
	if err := s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)
		if err := st.Pickups().Create(ctx, pr); err != nil {
			return err
		}
		if len(pr.ShipmentIDs) > 0 {
			// AddShipments menyaring tenant di dalam query — pengiriman milik
			// organisasi lain tidak akan pernah terlampir.
			if err := st.Pickups().AddShipments(ctx, orgID, pr.ID, pr.ShipmentIDs); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionPickupCreated, pr.ID)
	s.log.Info("pickup dibuat", "pickup_id", pr.ID, "org", orgID, "shipments", len(pr.ShipmentIDs))
	return pr, nil
}

// normalizeShipmentIDs membuang nilai kosong, memvalidasi bentuk UUID, dan
// menghapus duplikat. Duplikat harus dibuang sebelum dibandingkan dengan jumlah
// baris yang benar-benar masuk ke DB (kalau tidak, permintaan yang sah akan
// terlihat seperti "sebagian shipment tidak ditemukan").
func normalizeShipmentIDs(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, shared.Validation(map[string]string{
				"shipment_ids": "Daftar pengiriman memuat nilai yang bukan UUID.",
			})
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// recordAudit menyimpan jejak audit pickup; kegagalan audit dicatat ke log dan
// tidak menggagalkan operasi bisnis (aturan audit.Recorder).
func (s *Service) recordAudit(ctx context.Context, orgID, actorUserID, requestID, action, pickupID string) {
	if s.audit == nil {
		return
	}
	e := audit.New(action, actorUserID, orgID).WithEntity("pickup", pickupID)
	e.RequestID = requestID
	if err := s.audit.Record(ctx, e); err != nil {
		s.log.Warn("audit pickup gagal", "action", action, "pickup_id", pickupID, "error", err)
	}
}

// Get mengambil satu pickup request.
func (s *Service) Get(ctx context.Context, orgID, id string) (*pickup.Request, error) {
	return s.pickups.GetByID(ctx, orgID, id)
}

// List mengembalikan daftar pickup request.
func (s *Service) List(ctx context.Context, orgID string, f pickup.ListFilter) ([]pickup.Request, int, error) {
	return s.pickups.List(ctx, orgID, f)
}

// Schedule menjadwalkan pickup (requested → scheduled).
func (s *Service) Schedule(ctx context.Context, orgID, actorUserID, requestID, id string, when time.Time) (*pickup.Request, error) {
	pr, err := s.pickups.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := pr.Transition(pickup.StatusScheduled); err != nil {
		return nil, err
	}
	pr.ScheduledAt = &when
	if err := s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)
		// Schedule menyimpan status DAN scheduled_at sekaligus — sebelumnya
		// jadwalnya hilang setelah reload karena hanya status yang ditulis.
		return st.Pickups().Schedule(ctx, orgID, id, when)
	}); err != nil {
		return nil, err
	}
	pr.Status = pickup.StatusScheduled
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionPickupScheduled, id)
	return pr, nil
}

// Complete menandai pickup selesai (dari scheduled).
func (s *Service) Complete(ctx context.Context, orgID, actorUserID, requestID, id string) (*pickup.Request, error) {
	pr, err := s.pickups.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := pr.Transition(pickup.StatusCompleted); err != nil {
		return nil, err
	}
	if err := s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)
		return st.Pickups().UpdateStatus(ctx, orgID, id, pickup.StatusScheduled, pickup.StatusCompleted)
	}); err != nil {
		return nil, err
	}
	pr.Status = pickup.StatusCompleted
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionPickupCompleted, id)
	return pr, nil
}

// Cancel membatalkan pickup dari status non-terminal mana pun.
func (s *Service) Cancel(ctx context.Context, orgID, actorUserID, requestID, id string) (*pickup.Request, error) {
	pr, err := s.pickups.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	// State machine domain yang memutuskan boleh/tidak — repo hanya menulis.
	if err := pr.Transition(pickup.StatusCancelled); err != nil {
		return nil, err
	}
	if err := s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)
		return st.Pickups().CancelFrom(ctx, orgID, id)
	}); err != nil {
		return nil, err
	}
	pr.Status = pickup.StatusCancelled
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionPickupCancelled, id)
	return pr, nil
}
