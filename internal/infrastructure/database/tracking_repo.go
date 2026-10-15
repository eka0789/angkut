package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/tracking"
)

// TrackingRepo menyimpan event tracking tenant-scoped.
//
// Tabel `tracking_events` append-only: file ini sengaja TIDAK menyediakan
// Update atau Delete. Koreksi jejak dilakukan dengan menambah event baru.
type TrackingRepo struct{ q Querier }

const trackingCols = `id, organization_id, shipment_id, status, COALESCE(description,''),
	COALESCE(location,''), latitude, longitude, occurred_at, source,
	COALESCE(raw_event_id::text,''), created_at`

// Append menyimpan satu event. Mengembalikan tracking.ErrDuplicate bila kunci
// idempotensi alami (shipment_id, status, occurred_at, source) sudah ada —
// pemanggil memperlakukannya sebagai sukses, bukan kegagalan.
func (r *TrackingRepo) Append(ctx context.Context, e *tracking.Event) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO tracking_events (
			id, organization_id, shipment_id, status, description, location,
			latitude, longitude, occurred_at, source, raw_event_id, created_at
		) VALUES ($1,$2,$3::uuid,$4,NULLIF($5,''),NULLIF($6,''),
			$7,$8,$9,$10,NULLIF($11,'')::uuid,$12)`,
		e.ID, e.OrganizationID, e.ShipmentID, string(e.Status), e.Description, e.Location,
		e.Latitude, e.Longitude, e.OccurredAt, string(e.Source), e.RawEventID, e.CreatedAt,
	)
	if isUniqueViolationOn(err, "uq_tracking_event_natural") {
		return tracking.ErrDuplicate
	}
	if err != nil && isForeignKeyOn(err, "tracking_events_shipment_id_fkey") {
		return tracking.ErrShipmentNotFound
	}
	return err
}

// ListByShipment mengembalikan seluruh event satu pengiriman, terurut naik.
// Urutan di sini deterministik (waktu, lalu id) agar hasil stabil.
func (r *TrackingRepo) ListByShipment(ctx context.Context, orgID, shipmentID string) ([]tracking.Event, error) {
	rows, err := r.q.Query(ctx,
		`SELECT `+trackingCols+` FROM tracking_events
		  WHERE organization_id=$1 AND shipment_id=$2::uuid
		  ORDER BY occurred_at, id`, orgID, shipmentID)
	if err != nil {
		return nil, err
	}
	return scanTrackingEvents(rows)
}

// List mengembalikan halaman event dengan filter.
func (r *TrackingRepo) List(ctx context.Context, orgID string, f tracking.Filter) ([]tracking.Event, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	where := `organization_id=$1`
	args := []any{orgID}
	if f.ShipmentID != "" {
		args = append(args, f.ShipmentID)
		where += ` AND shipment_id=$` + itoa(len(args)) + `::uuid`
	}
	if f.Status != "" {
		args = append(args, string(f.Status))
		where += ` AND status=$` + itoa(len(args))
	}
	if f.Source != "" {
		args = append(args, string(f.Source))
		where += ` AND source=$` + itoa(len(args))
	}
	if f.From != "" {
		args = append(args, f.From)
		where += ` AND occurred_at >= $` + itoa(len(args)) + `::timestamptz`
	}
	if f.To != "" {
		args = append(args, f.To)
		where += ` AND occurred_at < $` + itoa(len(args)) + `::timestamptz`
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM tracking_events WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := r.q.Query(ctx,
		`SELECT `+trackingCols+` FROM tracking_events WHERE `+where+
			` ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out, err := scanTrackingEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func scanTrackingEvents(rows pgx.Rows) ([]tracking.Event, error) {
	defer rows.Close()
	out := make([]tracking.Event, 0)
	for rows.Next() {
		var (
			e      tracking.Event
			status string
			source string
		)
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.ShipmentID, &status,
			&e.Description, &e.Location, &e.Latitude, &e.Longitude,
			&e.OccurredAt, &source, &e.RawEventID, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Status = tracking.EventStatus(status)
		e.Source = tracking.Source(source)
		out = append(out, e)
	}
	return out, rows.Err()
}

// FindShipmentIDByAWB mencari id pengiriman dari resi (tenant-scoped).
// Dipakai endpoint pelacakan berbasis AWB.
func (r *TrackingRepo) FindShipmentIDByAWB(ctx context.Context, orgID, awb string) (string, error) {
	var id string
	err := r.q.QueryRow(ctx,
		`SELECT id::text FROM shipments WHERE organization_id=$1 AND awb=$2`, orgID, awb).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", tracking.ErrShipmentNotFound
	}
	return id, err
}

// ProcessedEventRepo mencatat event_id yang sudah diproses consumer.
// PRIMARY KEY event_id membuat dedup terjadi di database, bukan di memori —
// sehingga tetap benar walau ada beberapa replika worker.
type ProcessedEventRepo struct{ q Querier }

// MarkProcessed mencatat sebuah event sebagai sudah diproses. Mengembalikan
// (true, nil) bila ini pertama kali, (false, nil) bila sudah pernah diproses.
func (r *ProcessedEventRepo) MarkProcessed(ctx context.Context, eventID, consumer string) (bool, error) {
	tag, err := r.q.Exec(ctx, `
		INSERT INTO processed_events (event_id, consumer)
		VALUES ($1,$2) ON CONFLICT (event_id) DO NOTHING`, eventID, consumer)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
