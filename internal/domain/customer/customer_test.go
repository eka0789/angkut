package customer_test

import (
	"testing"

	"github.com/angkut/angkut/apps/internal/domain/customer"
)

// Aturan segmentasi otomatis — urutan pemeriksaan dari risiko ke loyalitas.
func TestAutoSegment(t *testing.T) {
	codOK := 0.95
	codBad := 0.5

	tests := []struct {
		name string
		m    customer.Metrics
		want customer.Segment
	}{
		{"tanpa riwayat → new", customer.Metrics{}, customer.SegmentNew},
		{"order 1 → new", customer.Metrics{OrdersCount: 1, TotalSpend: 1_000_000}, customer.SegmentNew},
		{"order 3 → regular", customer.Metrics{OrdersCount: 3, TotalSpend: 500_000}, customer.SegmentRegular},
		{"spend tinggi → vip", customer.Metrics{OrdersCount: 5, TotalSpend: 60_000_000}, customer.SegmentVIP},
		{"order banyak → vip", customer.Metrics{OrdersCount: 30}, customer.SegmentVIP},
		{"retur tinggi → high_risk", customer.Metrics{OrdersCount: 10, ReturnRate: 0.5, TotalSpend: 60_000_000}, customer.SegmentHighRisk},
		{"cod gagal sering → cod_risk", customer.Metrics{OrdersCount: 10, CODSuccessRate: &codBad}, customer.SegmentCODRisk},
		{"cod sukses tinggi → regular", customer.Metrics{OrdersCount: 10, CODSuccessRate: &codOK, TotalSpend: 500_000}, customer.SegmentRegular},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := customer.AutoSegment(tt.m); got != tt.want {
				t.Fatalf("AutoSegment(%+v) = %s, want %s", tt.m, got, tt.want)
			}
		})
	}
}

func TestSegmentValid(t *testing.T) {
	for _, s := range customer.AllSegments {
		if !s.Valid() {
			t.Fatalf("segmen bawaan %s harus valid", s)
		}
	}
	if customer.Segment("unknown").Valid() {
		t.Fatal("segmen tidak dikenal harus tidak valid")
	}
}

func TestCustomerValidate(t *testing.T) {
	c := customer.Customer{Name: "Budi", Phone: "0812"}
	if err := c.Validate(); err != nil {
		t.Fatalf("customer valid harus lolos: %v", err)
	}

	bad := customer.Customer{Phone: "0812"}
	err := bad.Validate()
	if err == nil {
		t.Fatal("customer tanpa nama harus gagal validasi")
	}
}

func TestFromMetrics(t *testing.T) {
	cod := 0.8
	m := customer.Metrics{OrdersCount: 4, TotalSpend: 200_000, AOV: 50_000, ReturnRate: 0.1, CODSuccessRate: &cod, PreferredCourier: "jne"}
	s := customer.FromMetrics(m)
	if s.OrdersCount != 4 || s.TotalSpend != 200_000 || s.AOV != 50_000 || s.PreferredCourier != "jne" {
		t.Fatalf("FromMetrics tidak meneruskan metrik: %+v", s)
	}
	if s.CODSuccessRate == nil || *s.CODSuccessRate != 0.8 {
		t.Fatalf("cod_success_rate salah: %+v", s.CODSuccessRate)
	}
}
