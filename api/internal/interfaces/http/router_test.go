package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "github.com/angkut/angkut/apps/api/internal/interfaces/http"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/observability"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return apihttp.NewRouter(apihttp.Deps{
		Config:   cfg,
		Metrics:  observability.NewRegistry(),
		Ready:    nil,
		APILimit: middleware.NewMemoryLimiter(120),
	})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, harap 200", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID harus diset oleh middleware RequestID")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security header X-Content-Type-Options hilang")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if body["success"] != true {
		t.Errorf("envelope success = %v", body["success"])
	}
}

func TestReadyzWithoutDeps(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, harap 200 saat tanpa dependensi wajib", rec.Code)
	}
}

func TestNotFoundEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tidak-ada", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, harap 404", rec.Code)
	}
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if body.Success || body.Error.Code != "ROUTE_NOT_FOUND" {
		t.Errorf("envelope 404 salah: %+v", body)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	router := newTestRouter(t)

	// Registry baru dibuat per router, jadi hasilkan satu request dulu agar
	// counter sudah terisi saat /metrics dibaca.
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, harap 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "angkut_http_requests_total") {
		t.Error("metrik request tidak terisi setelah request lain")
	}
}

func TestCORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, harap 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allow-origin = %q", got)
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "https://jahat.example")
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("origin tak dikenal tidak boleh diizinkan, dapat %q", got)
	}
}

func TestRecoverReturnsCleanEnvelope(t *testing.T) {
	panicHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	h := middleware.RequestID(middleware.Recover(panicHandler))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, harap 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("detail panic bocor ke klien")
	}
}
