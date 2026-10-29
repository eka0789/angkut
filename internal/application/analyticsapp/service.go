// Package analyticsapp menghitung KPI & laporan dari data operasional
// (TASK-047/052/055). Semua agregasi di SQL dengan filter tenant — tanpa
// tabel rollup dulu (data masih kecil; rollup menyusul bila perlu).
package analyticsapp

import (
	"context"
	"time"

	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// Deps dependensi service analytics.
type Deps struct {
	Stores *database.Stores
}

// Service service analytics.
type Service struct{ stores *database.Stores }

// New membuat service analytics.
func New(d Deps) *Service { return &Service{stores: d.Stores} }

// KPI adalah angka utama dashboard (kontrak /analytics/dashboard).
type KPI struct {
	OrdersToday     int64   `json:"orders_today"`
	ShipmentsToday  int64   `json:"shipments_today"`
	InTransit       int64   `json:"in_transit"`
	Delivered       int64   `json:"delivered"`
	Delayed         int64   `json:"delayed"`
	Failed          int64   `json:"failed"`
	ReturnsOpen     int64   `json:"returns"`
	CODPending      int64   `json:"cod_pending"`
	CODBalance      int64   `json:"cod_balance"`
	ShippingCost    int64   `json:"shipping_cost"`
	AvgDeliveryDays float64 `json:"avg_delivery_days"`
}

// ChartPoint adalah satu titik deret waktu.
type ChartPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// Dashboard adalah payload /analytics/dashboard.
type Dashboard struct {
	KPI      KPI           `json:"kpi"`
	Orders   []ChartPoint  `json:"orders_over_time"`
	Cost     []ChartPoint  `json:"shipping_cost"`
	Couriers []CourierPerf `json:"courier_performance"`
}

// CourierPerf adalah performa satu kurir.
type CourierPerf struct {
	Courier     string  `json:"courier"`
	Total       int64   `json:"total"`
	Delivered   int64   `json:"delivered"`
	Failed      int64   `json:"failed"`
	SuccessRate float64 `json:"success_rate"`
	AvgDays     float64 `json:"avg_delivery_days"`
}

// Dashboard menghitung KPI + chart 30 hari + performa kurir untuk tenant.
func (s *Service) Dashboard(ctx context.Context, orgID string) (*Dashboard, error) {
	d := &Dashboard{}
	st := s.stores
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -30).Format("2006-01-02")

	err := st.Querier().QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM orders WHERE organization_id=$1 AND created_at::date=$2),
			(SELECT count(*) FROM shipments WHERE organization_id=$1 AND created_at::date=$2),
			(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status IN ('picked_up','in_transit','out_for_delivery')),
			(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status='delivered'),
			(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status='in_transit' AND created_at < now() - interval '3 days'),
			(SELECT count(*) FROM shipments WHERE organization_id=$1 AND status='failed'),
			(SELECT count(*) FROM returns WHERE organization_id=$1 AND status NOT IN ('refunded','rejected')),
			(SELECT count(*) FROM cod_transactions WHERE organization_id=$1 AND status IN ('pending','collected')),
			(SELECT COALESCE(sum(amount),0) FROM cod_transactions WHERE organization_id=$1 AND status IN ('pending','collected')),
			(SELECT COALESCE(sum(shipping_price),0) FROM shipments WHERE organization_id=$1 AND created_at >= $3),
			(SELECT COALESCE(avg(EXTRACT(EPOCH FROM (delivered_at - created_at)))/86400.0, 0) FROM shipments WHERE organization_id=$1 AND delivered_at IS NOT NULL)`,
		orgID, today, since).Scan(
		&d.KPI.OrdersToday, &d.KPI.ShipmentsToday, &d.KPI.InTransit, &d.KPI.Delivered,
		&d.KPI.Delayed, &d.KPI.Failed, &d.KPI.ReturnsOpen, &d.KPI.CODPending,
		&d.KPI.CODBalance, &d.KPI.ShippingCost, &d.KPI.AvgDeliveryDays)
	if err != nil {
		return nil, err
	}

	// Chart order & biaya 30 hari.
	if err := s.chart(ctx, orgID, &d.Orders, `
		SELECT created_at::date, count(*) FROM orders
		WHERE organization_id=$1 AND created_at >= $2 GROUP BY 1 ORDER BY 1`); err != nil {
		return nil, err
	}
	if err := s.chart(ctx, orgID, &d.Cost, `
		SELECT created_at::date, COALESCE(sum(shipping_price),0) FROM shipments
		WHERE organization_id=$1 AND created_at >= $2 GROUP BY 1 ORDER BY 1`); err != nil {
		return nil, err
	}

	// Performa kurir.
	rows, err := st.Querier().Query(ctx, `
		SELECT courier_code, count(*),
			count(*) FILTER (WHERE status='delivered'),
			count(*) FILTER (WHERE status='failed'),
			COALESCE(avg(EXTRACT(EPOCH FROM (delivered_at - created_at))) FILTER (WHERE delivered_at IS NOT NULL)/86400.0, 0)
		FROM shipments WHERE organization_id=$1 AND created_at >= $2
		GROUP BY courier_code ORDER BY count(*) DESC`, orgID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cp CourierPerf
		if err := rows.Scan(&cp.Courier, &cp.Total, &cp.Delivered, &cp.Failed, &cp.AvgDays); err != nil {
			return nil, err
		}
		if cp.Total > 0 {
			cp.SuccessRate = float64(cp.Delivered) / float64(cp.Total) * 100
		}
		d.Couriers = append(d.Couriers, cp)
	}
	return d, rows.Err()
}

func (s *Service) chart(ctx context.Context, orgID string, out *[]ChartPoint, sql string) error {
	rows, err := s.stores.Querier().Query(ctx, sql, orgID, time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02"))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p ChartPoint
		var d time.Time
		if err := rows.Scan(&d, &p.Value); err != nil {
			return err
		}
		p.Date = d.Format("2006-01-02")
		*out = append(*out, p)
	}
	return rows.Err()
}

// HealthFactor satu komponen skor.
type HealthFactor struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"` // 0..100
	Weight float64 `json:"weight"`
}

// HealthScore adalah skor kesehatan logistik (TASK-055, heuristik).
type HealthScore struct {
	Score           float64        `json:"score"`
	Factors         []HealthFactor `json:"factors"`
	Recommendations []string       `json:"recommendations"`
}

// HealthScore menghitung 0–100 dari 4 faktor: success rate, kecepatan,
// retur, COD collection. Bobot: 40/25/15/20.
func (s *Service) HealthScore(ctx context.Context, orgID string) (*HealthScore, error) {
	var total, delivered, failed, returnsN int64
	var avgDays float64
	var codPending, codCollected int64
	err := s.stores.Querier().QueryRow(ctx, `
		SELECT count(*),
			count(*) FILTER (WHERE status='delivered'),
			count(*) FILTER (WHERE status='failed'),
			COALESCE(avg(EXTRACT(EPOCH FROM (delivered_at - created_at))) FILTER (WHERE delivered_at IS NOT NULL), 0) / 86400.0
		FROM shipments WHERE organization_id=$1`, orgID).
		Scan(&total, &delivered, &failed, &avgDays)
	if err != nil {
		return nil, err
	}
	if err := s.stores.Querier().QueryRow(ctx, `
		SELECT count(*) FROM returns WHERE organization_id=$1 AND status NOT IN ('refunded','rejected')`,
		orgID).Scan(&returnsN); err != nil {
		return nil, err
	}
	if err := s.stores.Querier().QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status='pending'),
		       count(*) FILTER (WHERE status IN ('collected','settling'))
		FROM cod_transactions WHERE organization_id=$1`, orgID).
		Scan(&codPending, &codCollected); err != nil {
		return nil, err
	}

	hs := &HealthScore{Factors: []HealthFactor{
		{Name: "delivery_success", Weight: 0.40},
		{Name: "speed", Weight: 0.25},
		{Name: "returns", Weight: 0.15},
		{Name: "cod_collection", Weight: 0.20},
	}}
	f := hs.Factors
	if total > 0 {
		f[0].Score = float64(delivered) / float64(total) * 100
	} else {
		f[0].Score = 100
	}
	// Kecepatan: 2 hari = 100, tiap hari ekstra −12.5 (batas 0).
	f[1].Score = max(0, 100-(avgDays-2)*12.5)
	// Retur: 0 retur = 100; tiap retur aktif −10.
	if total > 0 {
		f[2].Score = max(0, 100-float64(returnsN)/float64(total)*1000)
	} else {
		f[2].Score = 100
	}
	// COD: pending menumpuk = kurang sehat.
	if codPending+codCollected > 0 {
		f[3].Score = float64(codCollected) / float64(codPending+codCollected) * 100
	} else {
		f[3].Score = 100
	}
	hs.Score = f[0].Score*f[0].Weight + f[1].Score*f[1].Weight +
		f[2].Score*f[2].Weight + f[3].Score*f[3].Weight

	if f[0].Score < 85 {
		hs.Recommendations = append(hs.Recommendations,
			"Tingkat delivery success di bawah 85% — evaluasi layanan kurir dengan success rate terendah.")
	}
	if f[1].Score < 80 {
		hs.Recommendations = append(hs.Recommendations,
			"Rata-rata pengiriman melebihi 2 hari — pertimbangkan layanan lebih cepat untuk rute utama.")
	}
	if f[2].Score < 90 {
		hs.Recommendations = append(hs.Recommendations,
			"Retur aktif tinggi — periksa kemasan & akurasi deskripsi produk.")
	}
	if f[3].Score < 100 {
		hs.Recommendations = append(hs.Recommendations,
			"Buat settlement COD rutin agar uang di kurir tidak menumpuk.")
	}
	if len(hs.Recommendations) == 0 {
		hs.Recommendations = append(hs.Recommendations, "Operasi logistik sehat — pertahankan.")
	}
	return hs, nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
