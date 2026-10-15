package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/courier"
)

// CourierRepo membaca registry kurir (platform-level, tanpa tenant filter).
type CourierRepo struct{ q Querier }

const courierCols = `id, code, name, adapter_type, is_active, COALESCE(logo_url,''), created_at, updated_at`

func (r *CourierRepo) ListActive(ctx context.Context) ([]courier.Courier, error) {
	rows, err := r.q.Query(ctx, `SELECT `+courierCols+` FROM couriers WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]courier.Courier, 0)
	for rows.Next() {
		c, err := scanCourier(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *CourierRepo) GetByCode(ctx context.Context, code string) (*courier.Courier, error) {
	row := r.q.QueryRow(ctx, `SELECT `+courierCols+` FROM couriers WHERE code=$1`, code)
	return scanCourier(row)
}

const serviceCols = `id, courier_id, code, name, supports_cod, supports_pickup, supports_insurance,
	eta_min_days, eta_max_days, volumetric_divisor, reliability_score, is_active, created_at, updated_at`

func (r *CourierRepo) ListActiveServices(ctx context.Context, courierID string) ([]courier.Service, error) {
	rows, err := r.q.Query(ctx,
		`SELECT `+serviceCols+` FROM courier_services WHERE courier_id=$1 AND is_active ORDER BY code`,
		courierID)
	if err != nil {
		return nil, err
	}
	return scanServices(rows)
}

func (r *CourierRepo) ListActiveServicesAll(ctx context.Context) ([]courier.Service, error) {
	rows, err := r.q.Query(ctx,
		`SELECT `+serviceCols+` FROM courier_services s
		 JOIN couriers c ON c.id = s.courier_id
		 WHERE s.is_active AND c.is_active ORDER BY c.code, s.code`)
	if err != nil {
		return nil, err
	}
	return scanServices(rows)
}

func (r *CourierRepo) FindRates(ctx context.Context, serviceID, originZone, destinationZone string) ([]courier.Rate, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, courier_service_id, origin_zone, destination_zone,
		       min_weight, max_weight, price, effective_from, effective_to
		FROM courier_rates
		WHERE courier_service_id=$1 AND origin_zone=$2 AND destination_zone=$3
		  AND effective_from <= now()
		  AND (effective_to IS NULL OR effective_to > now())
		ORDER BY min_weight`, serviceID, originZone, destinationZone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]courier.Rate, 0)
	for rows.Next() {
		var rt courier.Rate
		if err := rows.Scan(&rt.ID, &rt.CourierServiceID, &rt.OriginZone, &rt.DestinationZone,
			&rt.MinWeight, &rt.MaxWeight, &rt.Price, &rt.EffectiveFrom, &rt.EffectiveTo); err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func (r *CourierRepo) ZoneForRegion(ctx context.Context, regionType, regionID string) (*courier.Zone, error) {
	row := r.q.QueryRow(ctx, `
		SELECT code, name, type, "values", created_at
		FROM zones
		WHERE type=$1 AND $2 = ANY("values")
		LIMIT 1`, regionType, regionID)
	var z courier.Zone
	if err := row.Scan(&z.Code, &z.Name, &z.Type, &z.Values, &z.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, courier.ErrZoneNotFound
		}
		return nil, err
	}
	return &z, nil
}

func (r *CourierRepo) ZoneForCity(ctx context.Context, cityID string) (*courier.Zone, error) {
	row := r.q.QueryRow(ctx, `
		SELECT z.code, z.name, z.type, z."values", z.created_at
		FROM zones z
		WHERE z.type = 'province'
		  AND (SELECT province_id FROM cities WHERE id = $1) = ANY(z."values")
		LIMIT 1`, cityID)
	var z courier.Zone
	if err := row.Scan(&z.Code, &z.Name, &z.Type, &z.Values, &z.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, courier.ErrZoneNotFound
		}
		return nil, err
	}
	return &z, nil
}

func scanCourier(row pgx.Row) (*courier.Courier, error) {
	var c courier.Courier
	err := row.Scan(&c.ID, &c.Code, &c.Name, &c.AdapterType, &c.IsActive, &c.LogoURL, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, courier.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanServices(rows pgx.Rows) ([]courier.Service, error) {
	defer rows.Close()
	out := make([]courier.Service, 0)
	for rows.Next() {
		var s courier.Service
		if err := rows.Scan(&s.ID, &s.CourierID, &s.Code, &s.Name, &s.SupportsCOD, &s.SupportsPickup,
			&s.SupportsInsurance, &s.EtaMinDays, &s.EtaMaxDays, &s.VolumetricDivisor,
			&s.ReliabilityScore, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
