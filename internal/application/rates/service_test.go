package rates

import (
	"testing"

	"github.com/angkut/angkut/apps/internal/domain/courier"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

func TestQuoteIDRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   courier.RateQuery
	}{
		{
			name: "tanpa dimensi",
			in: courier.RateQuery{
				OriginCityID:      "city-origin",
				DestinationCityID: "city-dest",
				ActualWeightKG:    1.5,
			},
		},
		{
			name: "dengan dimensi & COD & asuransi",
			in: courier.RateQuery{
				OriginCityID:      "city-origin",
				DestinationCityID: "city-dest",
				ActualWeightKG:    3.25,
				Dimensions:        &shared.Dimensions{Length: 30, Width: 20, Height: 10},
				IsCOD:             true,
				CODAmount:         250000,
				Insured:           true,
				DeclaredValue:     1000000,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := encodeQuoteID(tc.in)
			got, err := decodeQuoteID(id)
			if err != nil {
				t.Fatalf("decodeQuoteID: %v", err)
			}
			if got.OriginCityID != tc.in.OriginCityID ||
				got.DestinationCityID != tc.in.DestinationCityID ||
				got.ActualWeightKG != tc.in.ActualWeightKG ||
				got.IsCOD != tc.in.IsCOD ||
				got.CODAmount != tc.in.CODAmount ||
				got.Insured != tc.in.Insured ||
				got.DeclaredValue != tc.in.DeclaredValue {
				t.Errorf("hasil decode berbeda:\n dapat %+v\n ingin %+v", got, tc.in)
			}
			if (got.Dimensions == nil) != (tc.in.Dimensions == nil) {
				t.Fatalf("keberadaan dimensi berbeda: dapat %+v", got.Dimensions)
			}
			if tc.in.Dimensions != nil && *got.Dimensions != *tc.in.Dimensions {
				t.Errorf("dimensi berbeda: dapat %+v ingin %+v", *got.Dimensions, *tc.in.Dimensions)
			}
		})
	}
}

func TestDecodeQuoteIDRejectsForeignToken(t *testing.T) {
	if _, err := decodeQuoteID("bukan-token-angkut"); err == nil {
		t.Fatal("token tanpa prefix harus ditolak")
	}
	if _, err := decodeQuoteID(quoteIDPrefix + "!!!bukan-base64!!!"); err == nil {
		t.Fatal("payload rusak harus ditolak")
	}
	// Token valid secara bentuk tetapi parameter kosong → gagal validasi domain.
	if _, err := decodeQuoteID(quoteIDPrefix + "e30"); err == nil {
		t.Fatal("token dengan parameter kosong harus ditolak validasi")
	}
}

func TestCacheKeyDistinguishesMaterialParams(t *testing.T) {
	base := courier.RateQuery{
		OriginCityID:      "a",
		DestinationCityID: "b",
		ActualWeightKG:    1,
	}
	withCOD := base
	withCOD.IsCOD = true
	withCOD.CODAmount = 50000

	if cacheKeyForRateQuery(base) == cacheKeyForRateQuery(withCOD) {
		t.Error("kunci cache harus berbeda antara non-COD dan COD")
	}

	withDims := base
	withDims.Dimensions = &shared.Dimensions{Length: 10, Width: 10, Height: 10}
	if cacheKeyForRateQuery(base) == cacheKeyForRateQuery(withDims) {
		t.Error("kunci cache harus berbeda ketika dimensi ikut dihitung")
	}

	// Query identik harus menghasilkan kunci identik (deterministik).
	if cacheKeyForRateQuery(base) != cacheKeyForRateQuery(base) {
		t.Error("kunci cache harus deterministik")
	}
}
