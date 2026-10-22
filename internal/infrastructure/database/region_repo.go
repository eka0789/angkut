package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/region"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// RegionRepo membaca reference data wilayah publik.
type RegionRepo struct{ q Querier }

func (r *RegionRepo) ListProvinces(ctx context.Context) ([]region.Province, error) {
	rows, err := r.q.Query(ctx, `SELECT id, name, created_at::text FROM provinces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]region.Province, 0)
	for rows.Next() {
		var p region.Province
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *RegionRepo) ListCities(ctx context.Context, provinceID string) ([]region.City, error) {
	rows, err := r.q.Query(ctx, `SELECT id, province_id, name, type, created_at::text FROM cities WHERE province_id=$1 ORDER BY name`, provinceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]region.City, 0)
	for rows.Next() {
		var c region.City
		if err := rows.Scan(&c.ID, &c.ProvinceID, &c.Name, &c.Type, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *RegionRepo) ListDistricts(ctx context.Context, cityID string) ([]region.District, error) {
	rows, err := r.q.Query(ctx, `SELECT id, city_id, name, created_at::text FROM districts WHERE city_id=$1 ORDER BY name`, cityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]region.District, 0)
	for rows.Next() {
		var d region.District
		if err := rows.Scan(&d.ID, &d.CityID, &d.Name, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *RegionRepo) ListVillages(ctx context.Context, districtID string) ([]region.Village, error) {
	rows, err := r.q.Query(ctx, `SELECT id, district_id, name, type, created_at::text FROM villages WHERE district_id=$1 ORDER BY name`, districtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]region.Village, 0)
	for rows.Next() {
		var v region.Village
		if err := rows.Scan(&v.ID, &v.DistrictID, &v.Name, &v.Type, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *RegionRepo) ListPostalCodes(ctx context.Context, districtID string) ([]region.PostalCode, error) {
	rows, err := r.q.Query(ctx, `SELECT code, district_id, created_at::text FROM postal_codes WHERE district_id=$1 ORDER BY code`, districtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]region.PostalCode, 0)
	for rows.Next() {
		var p region.PostalCode
		if err := rows.Scan(&p.Code, &p.DistrictID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *RegionRepo) GetProvinceByID(ctx context.Context, id string) (*region.Province, error) {
	var p region.Province
	err := r.q.QueryRow(ctx, `SELECT id, name, created_at::text FROM provinces WHERE id=$1`, id).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("PROVINCE_NOT_FOUND", "Provinsi tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *RegionRepo) GetCityByID(ctx context.Context, id string) (*region.City, error) {
	var c region.City
	err := r.q.QueryRow(ctx, `SELECT id, province_id, name, type, created_at::text FROM cities WHERE id=$1`, id).Scan(&c.ID, &c.ProvinceID, &c.Name, &c.Type, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("CITY_NOT_FOUND", "Kota tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *RegionRepo) GetDistrictByID(ctx context.Context, id string) (*region.District, error) {
	var d region.District
	err := r.q.QueryRow(ctx, `SELECT id, city_id, name, created_at::text FROM districts WHERE id=$1`, id).Scan(&d.ID, &d.CityID, &d.Name, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("DISTRICT_NOT_FOUND", "Kecamatan tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *RegionRepo) GetVillageByID(ctx context.Context, id string) (*region.Village, error) {
	var v region.Village
	err := r.q.QueryRow(ctx, `SELECT id, district_id, name, type, created_at::text FROM villages WHERE id=$1`, id).Scan(&v.ID, &v.DistrictID, &v.Name, &v.Type, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("VILLAGE_NOT_FOUND", "Desa/kelurahan tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ResolvePostalCode mengubah kode pos menjadi rantai wilayah penuh lewat
// join postal_codes → districts → cities → provinces (satu query, tanpa
// N+1). Dipakai impor massal yang hanya menyediakan kolom kode pos.
func (r *RegionRepo) ResolvePostalCode(ctx context.Context, code string) (*region.PostalResolution, error) {
	var res region.PostalResolution
	err := r.q.QueryRow(ctx, `
		SELECT pc.code, d.id, c.id, p.id
		FROM postal_codes pc
		JOIN districts d ON d.id = pc.district_id
		JOIN cities    c ON c.id = d.city_id
		JOIN provinces p ON p.id = c.province_id
		WHERE pc.code = $1`, code).Scan(&res.PostalCode, &res.DistrictID, &res.CityID, &res.ProvinceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("POSTAL_CODE_NOT_FOUND", "Kode pos tidak dikenal.")
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}
