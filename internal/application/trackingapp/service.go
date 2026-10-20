// Package trackingapp berisi logika aplikasi pelacakan: mencatat event,
// memindahkan status pengiriman, dan menyusun timeline.
//
// Semua efek samping (perubahan status shipment + baris outbox) ditulis dalam
// SATU transaksi bersama event tracking-nya. Ini inti ADR-005: tidak boleh ada
// status berubah tanpa event, dan sebaliknya.
package trackingapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/event"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
	"github.com/angkut/angkut/apps/internal/realtime"
)

// Deps adalah dependensi service tracking.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Events tracking.Repository
	Audit  audit.Recorder
	Log    *slog.Logger

	// Notifier menyiarkan peristiwa realtime (SSE/WS). Optional — bila nil
	// dipakai no-op, karena publish adalah best-effort dan kebenaran data
	// tetap di database.
	Notifier realtime.Notifier
}

// Service adalah service aplikasi tracking.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	events tracking.Repository
	audit  audit.Recorder
	log    *slog.Logger
	real   realtime.Notifier
}

// New membuat service tracking.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	notif := d.Notifier
	if notif == nil {
		notif = realtime.NopNotifier{}
	}
	return &Service{tx: d.Tx, stores: d.Stores, events: d.Events, audit: d.Audit, log: log, real: notif}
}

// AppendInput adalah permintaan mencatat satu event.
type AppendInput struct {
	ShipmentID  string
	Status      tracking.EventStatus
	Description string
	Location    string
	Latitude    *float64
	Longitude   *float64
	OccurredAt  time.Time
	Source      tracking.Source
	RawEventID  string
}

// AppendResult menjelaskan apa yang terjadi saat event dicatat.
type AppendResult struct {
	Event *tracking.Event

	// Duplicate=true berarti event sudah pernah tercatat (kunci idempotensi
	// alami). Ini BUKAN kegagalan: pengirim yang mengulang cukup diberi tahu.
	Duplicate bool

	// ShipmentStatus adalah status pengiriman setelah event ini (bila berubah).
	ShipmentStatus shipment.Status
	StatusChanged  bool
}

// Append mencatat satu event pelacakan dan, bila event itu memindahkan status,
// memperbarui pengiriman + menulis event outbox — semuanya dalam SATU transaksi.
func (s *Service) Append(ctx context.Context, orgID, actorUserID, requestID string, in AppendInput) (*AppendResult, error) {
	if strings.TrimSpace(in.ShipmentID) == "" {
		return nil, shared.Validation(map[string]string{"shipment_id": "Pengiriman wajib diisi."})
	}
	if !in.Status.Valid() {
		return nil, shared.Validation(map[string]string{"status": "Status tracking tidak dikenal."})
	}
	source := in.Source
	if source == "" {
		source = tracking.SourceSystem
	}
	if !source.Valid() {
		return nil, shared.Validation(map[string]string{"source": "Sumber event tidak dikenal."})
	}
	occurred := in.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}
	occurred = occurred.UTC()

	ev := &tracking.Event{
		ID:             platform.NewUUID(),
		OrganizationID: orgID,
		ShipmentID:     strings.TrimSpace(in.ShipmentID),
		Status:         in.Status,
		Description:    strings.TrimSpace(in.Description),
		Location:       strings.TrimSpace(in.Location),
		Latitude:       in.Latitude,
		Longitude:      in.Longitude,
		OccurredAt:     occurred,
		Source:         source,
		RawEventID:     strings.TrimSpace(in.RawEventID),
		CreatedAt:      time.Now().UTC(),
	}
	if ev.Description == "" {
		ev.Description = tracking.DescriptionFor(ev.Status)
	}
	if err := ev.Validate(); err != nil {
		return nil, err
	}

	res := &AppendResult{}
	err := s.tx.InTx(ctx, func(q database.Querier) error {
		tx := s.stores.WithTx(q)

		// Savepoint wajib: pelanggaran unique (kunci alami duplikat) membuat
		// seluruh transaksi berstatus `aborted` di Postgres, sehingga COMMIT
		// berikutnya ikut gagal walau handler sudah berniat menganggap ini
		// sukses. Dengan savepoint, jalur duplikat cukup melakukan ROLLBACK TO
		// lalu transaksi tetap bisa dilanjutkan/di-commit.
		const sp = "sp_tracking_append"
		if _, err := q.Exec(ctx, "SAVEPOINT "+sp); err != nil {
			return err
		}
		if err := tx.Tracking().Append(ctx, ev); err != nil {
			if tracking.IsDuplicate(err) {
				if _, rerr := q.Exec(ctx, "ROLLBACK TO SAVEPOINT "+sp); rerr != nil {
					return rerr
				}
				res.Duplicate = true
				return nil
			}
			return err
		}
		res.Event = ev

		// Event informasi (created, sorting) tidak memindahkan status pengiriman.
		target, promotes := tracking.ShipmentStatusFor(ev.Status)
		if !promotes {
			return nil
		}

		sh, err := tx.Shipments().GetByID(ctx, orgID, ev.ShipmentID)
		if err != nil {
			return err
		}
		res.ShipmentStatus = sh.Status

		// Transisi ilegal bukan kesalahan fatal: kurir nyata kadang mengirim
		// event yang tidak berurutan (mis. delivered setelah sync terlambat).
		// Event TETAP dicatat sebagai jejak, tetapi status tidak dipaksa.
		if !sh.Status.CanTransitionTo(target) {
			s.log.Warn("event tracking tidak memindahkan status (transisi tidak sah)",
				"shipment_id", sh.ID, "from", string(sh.Status), "to", string(target),
				"event_status", string(ev.Status))
			return nil
		}

		if err := tx.Shipments().UpdateStatus(ctx, orgID, sh.ID, sh.Status, target, ""); err != nil {
			return err
		}
		res.ShipmentStatus = target
		res.StatusChanged = true

		// Outbox ditulis di transaksi yang sama (ADR-005).
		return tx.Outbox().Publish(ctx, event.New(
			platform.NewUUID(),
			shipmentEventType(target),
			sh.ID,
			orgID,
			occurred,
			map[string]any{
				"shipment_id": sh.ID,
				"awb":         sh.AWB,
				"status":      string(target),
				"location":    ev.Location,
				"occurred_at": occurred.Format(time.RFC3339),
				"source":      string(ev.Source),
			},
		))
	})
	if err != nil {
		return nil, err
	}

	if res.Duplicate {
		s.log.Debug("event tracking duplikat diabaikan", "shipment_id", ev.ShipmentID, "status", string(ev.Status))
		return res, nil
	}

	s.recordAudit(ctx, orgID, actorUserID, requestID, ev)
	s.notify(ctx, orgID, res, ev)
	return res, nil
}

// notify menyiarkan peristiwa tracking ke klien realtime (SSE/WS) via Redis
// pub/sub — best-effort: kegagalan hanya dicatat, TIDAK membalikkan status
// yang sudah tersimpan (monitoring) maupun memgagalkan permintaan (dan pesan
// yang hilang dipulihkan klien lewat snapshot REST saat tersambung ulang).
func (s *Service) notify(ctx context.Context, orgID string, res *AppendResult, ev *tracking.Event) {
	if res == nil || res.Event == nil {
		return
	}
	sh, err := s.stores.Shipments().GetByID(ctx, orgID, ev.ShipmentID)
	if err != nil {
		s.log.Debug("realtime: shipment tidak dapat dibaca untuk notifikasi",
			"shipment_id", ev.ShipmentID, "error", err)
		return
	}
	payload := map[string]any{
		"awb":    sh.AWB,
		"status": string(res.ShipmentStatus),
		"event": map[string]any{
			"status":       string(ev.Status),
			"description":  ev.Description,
			"location":     ev.Location,
			"occurred_at":  ev.OccurredAt.Format(time.RFC3339),
			"raw_event_id": ev.RawEventID,
		},
	}
	_ = s.real.Publish(realtime.Message{
		Type:       realtime.TypeTrackingUpdated,
		Topics:     []string{realtime.TrackChannel(sh.AWB), realtime.OrgShipmentsChannel(orgID)},
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})
}

// Timeline menyusun timeline sebuah pengiriman beserta penilaian keyakinan.
func (s *Service) Timeline(ctx context.Context, orgID, shipmentID string) (*tracking.Timeline, *shipment.Shipment, error) {
	sh, err := s.stores.Shipments().GetByID(ctx, orgID, shipmentID)
	if err != nil {
		return nil, nil, err
	}
	events, err := s.events.ListByShipment(ctx, orgID, sh.ID)
	if err != nil {
		return nil, nil, err
	}
	tl := tracking.BuildTimeline(sh.AWB, events)
	return &tl, sh, nil
}

// TimelineByAWB menyusun timeline dari nomor resi.
func (s *Service) TimelineByAWB(ctx context.Context, orgID, awb string) (*tracking.Timeline, *shipment.Shipment, error) {
	id, err := s.stores.Tracking().FindShipmentIDByAWB(ctx, orgID, strings.TrimSpace(awb))
	if err != nil {
		return nil, nil, err
	}
	return s.Timeline(ctx, orgID, id)
}

// TimelinePublik menyusun timeline untuk endpoint pelacakan publik (TASK-035)
// yang hanya menerima resi — pemanggilnya tidak berafiliasi pada organisasi
// mana pun, jadi cakupan tenant tidak bisa dipakai sebagai kunci.
//
// Pencarian PEMILIK resi memang lintas organisasi (resi unik global — migrasi
// 0020), tetapi setelah pemiliknya diketahui seluruh pembacaan detail kembali
// lewat jalur tenant-scoped biasa memakai organisasi pemilik tadi. Tidak ada
// query data yang berjalan tanpa filter organisasi.
func (s *Service) TimelinePublik(ctx context.Context, awb string) (*tracking.Timeline, *shipment.Shipment, error) {
	id, orgID, err := s.stores.Tracking().FindOwnerByAWBGlobal(ctx, awb)
	if err != nil {
		return nil, nil, err
	}
	return s.Timeline(ctx, orgID, id)
}

// List mengembalikan halaman event dengan filter.
func (s *Service) List(ctx context.Context, orgID string, f tracking.Filter) ([]tracking.Event, int, error) {
	return s.events.List(ctx, orgID, f)
}

// shipmentEventType memetakan status pengiriman ke nama event katalog.
func shipmentEventType(s shipment.Status) event.Type {
	switch s {
	case shipment.StatusPickedUp:
		return event.ShipmentPickedUp
	case shipment.StatusInTransit:
		return event.ShipmentInTransit
	case shipment.StatusDelivered:
		return event.ShipmentDelivered
	case shipment.StatusFailed:
		return event.ShipmentFailed
	case shipment.StatusReturned:
		return event.ShipmentReturned
	case shipment.StatusCancelled:
		return event.ShipmentCancelled
	default:
		return event.ShipmentUpdated
	}
}

// recordAudit mencatat event pelacakan; kegagalan audit tidak menggagalkan.
func (s *Service) recordAudit(ctx context.Context, orgID, actorUserID, requestID string, ev *tracking.Event) {
	if s.audit == nil {
		return
	}
	err := s.audit.Record(ctx, audit.Entry{
		OrganizationID: orgID,
		ActorUserID:    actorUserID,
		Action:         audit.ActionTrackingRecorded,
		EntityType:     "shipment",
		EntityID:       ev.ShipmentID,
		Changes: map[string]any{
			"tracking_status": string(ev.Status),
			"source":          string(ev.Source),
			"occurred_at":     ev.OccurredAt.Format(time.RFC3339),
		},
		RequestID: requestID,
	})
	if err != nil {
		s.log.Warn("audit tracking gagal", "shipment_id", ev.ShipmentID, "error", err)
	}
}
