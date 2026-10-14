package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angkut/angkut/apps/api/internal/application/rates"
	"github.com/angkut/angkut/apps/api/internal/domain/courier"
	"github.com/angkut/angkut/apps/api/internal/domain/pricing"
	"github.com/angkut/angkut/apps/api/internal/domain/scoring"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/go-chi/chi/v5"
)

// ── fake registry kurir ──────────────────────────────────────────────────

func testZone(code string) *courier.Zone { return &courier.Zone{Code: code, Name: code} }

type fakeCourierRepo struct{ couriers []courier.Courier }

func (f *fakeCourierRepo) ListActive(context.Context) ([]courier.Courier, error) {
	return f.couriers, nil
}

func (f *fakeCourierRepo) GetByCode(_ context.Context, code string) (*courier.Courier, error) {
	for i := range f.couriers {
		if f.couriers[i].Code == code {
			return &f.couriers[i], nil
		}
	}
	return nil, shared.NotFound("COURIER_NOT_FOUND", "Kurir tidak ditemukan.")
}

func (f *fakeCourierRepo) ListActiveServices(_ context.Context, courierID string) ([]courier.Service, error) {
	return []courier.Service{{ID: "svc-1", CourierID: courierID, Code: "REG", Name: "Reguler"}}, nil
}

func (f *fakeCourierRepo) ListActiveServicesAll(context.Context) ([]courier.Service, error) {
	return nil, nil
}

func (f *fakeCourierRepo) FindRates(context.Context, string, string, string) ([]courier.Rate, error) {
	return nil, nil
}

func (f *fakeCourierRepo) ZoneForRegion(context.Context, string, string) (*courier.Zone, error) {
	return testZone("Z1"), nil
}

func (f *fakeCourierRepo) ZoneForCity(_ context.Context, cityID string) (*courier.Zone, error) {
	if cityID == "" {
		return nil, shared.Validation(map[string]string{"city": "Kota tidak boleh kosong."})
	}
	return testZone("Z1"), nil
}

// fakeProvider mengembalikan tarif tetap per kode kurir.
type fakeProvider struct {
	code string
	rate courier.ServiceRate
}

func (p *fakeProvider) Code() string { return p.code }
func (p *fakeProvider) FetchRates(context.Context, courier.RateQuery) ([]courier.ServiceRate, error) {
	return []courier.ServiceRate{p.rate}, nil
}

// newTestHandler merakit handler dengan dua kurir: satu murah-lambat, satu
// mahal-cepat-andal.
func newTestHandler() http.Handler {
	repo := &fakeCourierRepo{couriers: []courier.Courier{
		{ID: "c1", Code: "jne", Name: "JNE", IsActive: true},
		{ID: "c2", Code: "sicepat", Name: "SiCepat", IsActive: true},
	}}
	registry := courier.NewRegistry(
		&fakeProvider{code: "jne", rate: courier.ServiceRate{
			Courier: courier.Courier{Code: "jne", Name: "JNE"},
			Service: courier.Service{
				Code: "REG", Name: "Reguler", SupportsCOD: true, SupportsPickup: true,
				EtaMinDays: 4, EtaMaxDays: 5, ReliabilityScore: 70,
			},
			Price:            10000,
			OriginZone:       "Z1",
			DestinationZone:  "Z1",
			ChargeableWeight: 1,
		}},
		&fakeProvider{code: "sicepat", rate: courier.ServiceRate{
			Courier: courier.Courier{Code: "sicepat", Name: "SiCepat"},
			Service: courier.Service{
				Code: "BEST", Name: "Best", SupportsCOD: true, SupportsPickup: true,
				EtaMinDays: 1, EtaMaxDays: 2, ReliabilityScore: 95,
			},
			Price:            20000,
			OriginZone:       "Z1",
			DestinationZone:  "Z1",
			ChargeableWeight: 1,
		}},
	)
	engine := pricing.New(pricing.Config{
		VolumetricDivisorDefault: 6000,
		WeightRoundUpStepKG:      0.1,
		MaxChargeableWeightKG:    100,
		CODFeeRateBps:            100,
		CODFeeMin:                2500,
		InsuranceRateBps:         20,
		InsuranceMinFee:          2500,
		PriceRoundStep:           100,
	})
	scorer := scoring.New(scoring.Config{
		WeightPriceBps:       4000,
		WeightSpeedBps:       2500,
		WeightReliabilityBps: 3500,
		ReasonThresholdPct:   5,
		HighReliabilityScore: 90,
		ConfidenceBaseBps:    6000,
		ConfidenceSpanBps:    3500,
		MaxReasons:           3,
	})
	svc := rates.New(rates.Deps{Repo: repo, Registry: registry, Engine: engine, Scorer: scorer})
	h := handler.NewCourierHandler(svc)

	r := chi.NewRouter()
	// Suntik identitas lewat middleware Auth dengan authenticator palsu: tes ini
	// fokus pada bentuk respons handler, bukan pada verifikasi token.
	r.Use(middleware.Auth(func(context.Context, string) (*middleware.Identity, error) {
		return &middleware.Identity{
			UserID: "u1", OrganizationID: "org-1", RoleCode: "owner",
		}, nil
	}))
	h.RegisterRoutes(r)
	return r
}

// envelope adalah bentuk respons untuk penguraian di test.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

type quotePayload struct {
	QuoteID          string  `json:"quote_id"`
	ChargeableWeight float64 `json:"chargeable_weight"`
	OriginZone       string  `json:"origin_zone"`
	DestinationZone  string  `json:"destination_zone"`
	Options          []struct {
		Courier struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"courier"`
		Service struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"service"`
		Price                int64    `json:"price"`
		CODFee               int64    `json:"cod_fee"`
		EtaMinDays           int      `json:"eta_min_days"`
		EtaMaxDays           int      `json:"eta_max_days"`
		ReliabilityScore     float64  `json:"reliability_score"`
		SupportsCOD          bool     `json:"supports_cod"`
		SupportsPickup       bool     `json:"supports_pickup"`
		EstimatedSuccessRate float64  `json:"estimated_success_rate"`
		Score                float64  `json:"score"`
		Labels               []string `json:"labels"`
		IsRecommended        bool     `json:"is_recommended"`
		Recommendation       *struct {
			Reasons    []string `json:"reasons"`
			Confidence float64  `json:"confidence"`
		} `json:"recommendation"`
		BasePrice    int64 `json:"base_price"`
		Markup       int64 `json:"markup"`
		InsuranceFee int64 `json:"insurance_fee"`
	} `json:"options"`
	SummaryLabels struct {
		Cheapest     string `json:"cheapest"`
		Fastest      string `json:"fastest"`
		MostReliable string `json:"most_reliable"`
		BestValue    string `json:"best_value"`
	} `json:"summary_labels"`
}

func doJSON(t *testing.T, r http.Handler, method, target, body string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Auth palsu membaca token apa pun; header ini hanya agar middleware lewat.
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("respons bukan JSON valid: %v\nbody: %s", err, rec.Body.String())
	}
	return rec, env
}

const validQuery = "?origin_city_id=city-a&destination_city_id=city-b&weight=1"

func TestRatesResponseMatchesContract(t *testing.T) {
	rec, env := doJSON(t, newTestHandler(), http.MethodGet, "/rates"+validQuery, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ingin 200; body: %s", rec.Code, rec.Body.String())
	}
	if !env.Success {
		t.Fatalf("success = false; error: %+v", env.Error)
	}

	var got quotePayload
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("data tidak sesuai bentuk: %v", err)
	}

	// Field tingkat quote wajib ada sesuai kontrak.
	if got.QuoteID == "" {
		t.Error("quote_id wajib ada")
	}
	if got.ChargeableWeight != 1 {
		t.Errorf("chargeable_weight = %v, ingin 1", got.ChargeableWeight)
	}
	if len(got.Options) != 2 {
		t.Fatalf("jumlah opsi = %d, ingin 2", len(got.Options))
	}

	// Label ringkasan harus menunjuk "KURIR:LAYANAN" pemenang.
	if got.SummaryLabels.Cheapest != "JNE:REG" {
		t.Errorf("cheapest = %q, ingin JNE:REG", got.SummaryLabels.Cheapest)
	}
	if got.SummaryLabels.Fastest != "SICEPAT:BEST" {
		t.Errorf("fastest = %q, ingin SICEPAT:BEST", got.SummaryLabels.Fastest)
	}

	// Setiap opsi wajib membawa field skor/label/eta/reliability sesuai kontrak.
	recommended := 0
	for _, opt := range got.Options {
		if opt.Courier.Code == "" || opt.Service.Code == "" {
			t.Error("opsi harus punya kode kurir & layanan")
		}
		if opt.EtaMinDays <= 0 || opt.EtaMaxDays <= 0 {
			t.Errorf("%s: ETA wajib terisi", opt.Courier.Code)
		}
		if opt.ReliabilityScore <= 0 {
			t.Errorf("%s: reliability_score wajib terisi", opt.Courier.Code)
		}
		if opt.Score <= 0 || opt.Score > 100 {
			t.Errorf("%s: skor %v di luar (0,100]", opt.Courier.Code, opt.Score)
		}
		if opt.EstimatedSuccessRate <= 0 || opt.EstimatedSuccessRate > 1 {
			t.Errorf("%s: estimated_success_rate %v di luar (0,1]", opt.Courier.Code, opt.EstimatedSuccessRate)
		}
		if opt.Labels == nil {
			t.Errorf("%s: labels wajib array (bukan null)", opt.Courier.Code)
		}
		if opt.Price <= 0 {
			t.Errorf("%s: price wajib > 0", opt.Courier.Code)
		}
		if opt.IsRecommended {
			recommended++
			if opt.Recommendation == nil {
				t.Error("opsi rekomendasi wajib membawa recommendation")
			} else if len(opt.Recommendation.Reasons) == 0 {
				t.Error("rekomendasi wajib punya alasan")
			}
		}
	}
	if recommended != 1 {
		t.Errorf("jumlah opsi direkomendasikan = %d, ingin 1", recommended)
	}
}

func TestRatesPreviewMatchesGet(t *testing.T) {
	h := newTestHandler()

	_, getEnv := doJSON(t, h, http.MethodGet, "/rates"+validQuery, "")
	rec, postEnv := doJSON(t, h, http.MethodPost, "/rates/preview",
		`{"origin_city_id":"city-a","destination_city_id":"city-b","weight":1}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d; body: %s", rec.Code, rec.Body.String())
	}

	var fromGet, fromPost quotePayload
	if err := json.Unmarshal(getEnv.Data, &fromGet); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(postEnv.Data, &fromPost); err != nil {
		t.Fatal(err)
	}

	// Body POST harus menghasilkan rangkaian opsi yang identik dengan GET.
	if len(fromGet.Options) != len(fromPost.Options) {
		t.Fatalf("jumlah opsi GET %d vs POST %d", len(fromGet.Options), len(fromPost.Options))
	}
	if fromGet.SummaryLabels != fromPost.SummaryLabels {
		t.Errorf("summary_labels berbeda: GET %+v vs POST %+v", fromGet.SummaryLabels, fromPost.SummaryLabels)
	}
	if fromGet.QuoteID != fromPost.QuoteID {
		t.Errorf("quote_id berbeda: GET %q vs POST %q", fromGet.QuoteID, fromPost.QuoteID)
	}
}

func TestRecommendResolvesQuoteID(t *testing.T) {
	h := newTestHandler()

	_, getEnv := doJSON(t, h, http.MethodGet, "/rates"+validQuery, "")
	var quote quotePayload
	if err := json.Unmarshal(getEnv.Data, &quote); err != nil {
		t.Fatal(err)
	}

	rec, env := doJSON(t, h, http.MethodPost, "/recommend",
		`{"quote_id":"`+quote.QuoteID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		QuoteID     string `json:"quote_id"`
		Recommended *struct {
			Courier struct {
				Code string `json:"code"`
			} `json:"courier"`
			Service struct {
				Code string `json:"code"`
			} `json:"service"`
			IsRecommended  bool `json:"is_recommended"`
			Recommendation *struct {
				Reasons    []string `json:"reasons"`
				Confidence float64  `json:"confidence"`
			} `json:"recommendation"`
		} `json:"recommended"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatal(err)
	}

	if payload.QuoteID != quote.QuoteID {
		t.Errorf("quote_id = %q, ingin %q", payload.QuoteID, quote.QuoteID)
	}
	if payload.Recommended == nil {
		t.Fatal("recommended wajib terisi untuk quote_id yang sah")
	}
	if !payload.Recommended.IsRecommended {
		t.Error("opsi yang dikembalikan harus ditandai is_recommended")
	}
	// SiCepat unggul pada ETA (1-2 hari vs 4-5) dan keandalan (95 vs 70),
	// sehingga bobot default harus memilihnya walau harganya lebih mahal.
	if payload.Recommended.Courier.Code != "sicepat" {
		t.Errorf("kurir rekomendasi = %q, ingin sicepat", payload.Recommended.Courier.Code)
	}
	if payload.Recommended.Recommendation == nil || len(payload.Recommended.Recommendation.Reasons) == 0 {
		t.Error("rekomendasi wajib disertai alasan")
	}
}

func TestRecommendRejectsForeignQuoteID(t *testing.T) {
	rec, env := doJSON(t, newTestHandler(), http.MethodPost, "/recommend",
		`{"quote_id":"bukan-token-angkut"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, ingin 422; body: %s", rec.Code, rec.Body.String())
	}
	if env.Success {
		t.Error("success harus false untuk quote_id asing")
	}
	if env.Error == nil || env.Error.Fields["quote_id"] == "" {
		t.Errorf("error harus menyebut field quote_id; dapat %+v", env.Error)
	}
}

func TestRatesValidatesWeight(t *testing.T) {
	rec, env := doJSON(t, newTestHandler(), http.MethodGet,
		"/rates?origin_city_id=city-a&destination_city_id=city-b&weight=abc", "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, ingin 422", rec.Code)
	}
	if env.Error == nil || env.Error.Fields["weight"] == "" {
		t.Errorf("error harus menyebut field weight; dapat %+v", env.Error)
	}
}

func TestRatesRequiresCODParamsWhenCOD(t *testing.T) {
	rec, env := doJSON(t, newTestHandler(), http.MethodGet,
		"/rates"+validQuery+"&is_cod=true", "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, ingin 422", rec.Code)
	}
	if env.Error == nil || env.Error.Fields["cod_amount"] == "" {
		t.Errorf("error harus menyebut field cod_amount; dapat %+v", env.Error)
	}
}
