// Package http menyusun router dan rantai middleware. Tidak ada logika bisnis
// di paket ini — hanya registrasi route dan penyambungan handler.
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/observability"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
)

// Deps adalah dependensi yang dibutuhkan router. Sebagian besar diisi pada
// fase berikutnya; Phase 1 hanya memakai health/metrics.
type Deps struct {
	Config   *config.Config
	Metrics  *observability.Registry
	Ready    []handler.Checker
	APILimit middleware.Limiter
}

// NewRouter merakit router lengkap beserta middleware global.
//
// URUTAN MIDDLEWARE DIKUNCI (lihat dev-docs/architecture/api-flow.md):
// RequestID → Recover → Logger → Metrics → SecurityHeaders → CORS.
// Middleware auth/tenant/rbac/idempotency dipasang per-group, bukan global,
// supaya endpoint publik (health, tracking publik) tidak terkena.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recover)
	r.Use(middleware.Logger)
	if d.Metrics != nil {
		r.Use(observability.MetricsMiddleware(d.Metrics))
	}
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(d.Config.API.AllowedOrigins))

	r.NotFound(handler.NotFound)
	r.MethodNotAllowed(handler.MethodNotAllowed)

	// ── Observability (tanpa auth, tanpa rate limit ketat) ────────────────
	r.Group(func(pub chi.Router) {
		pub.Get("/healthz", handler.Health)
		pub.Method(http.MethodGet, "/readyz", handler.Ready(d.Ready))
		if d.Metrics != nil && d.Config.Obs.MetricsOn {
			pub.Handle("/metrics", d.Metrics.Handler())
		}
	})

	// ── API v1 ────────────────────────────────────────────────────────────
	r.Route("/api/v1", func(api chi.Router) {
		if d.APILimit != nil {
			api.Use(middleware.RateLimit(d.APILimit, middleware.KeyByUserOrIP))
		}
		// Rate limit ketat khusus endpoint sensitif (auth, publik).
		api.Route("/auth", func(auth chi.Router) {
			if d.APILimit != nil {
				auth.Use(middleware.RateLimit(d.APILimit, middleware.KeyByIP))
			}
			// Route auth didaftarkan pada Phase 2.
		})
		api.Route("/public", func(pub chi.Router) {
			// Tracking publik didaftarkan pada Phase 3.
		})
	})

	return r
}

// pingTimeout membatasi berapa lama pengecekan readiness boleh berjalan.
var pingTimeout = 2 * time.Second