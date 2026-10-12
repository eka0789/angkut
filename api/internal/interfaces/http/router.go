// Package http menyusun router dan rantai middleware. Tidak ada logika bisnis
// di paket ini — hanya registrasi route dan penyambungan handler.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/observability"
)

// Deps adalah dependensi yang dibutuhkan router.
type Deps struct {
	Config   *config.Config
	Metrics  *observability.Registry
	Ready    []handler.Checker
	APILimit middleware.Limiter

	// Authenticate memvalidasi access token → identitas pemanggil.
	Authenticate middleware.Authenticator

	AuthHandler         *handler.AuthHandler
	OrganizationHandler *handler.OrganizationHandler
	GoogleOAuthHandler  *handler.GoogleOAuthHandler
	RegionHandler       *handler.RegionHandler
	AddressHandler      *handler.AddressHandler
}

// NewRouter merakit router lengkap beserta middleware global.
//
// URUTAN MIDDLEWARE DIKUNCI (lihat dev-docs/architecture/api-flow.md):
// RequestID → Recover → Logger → Metrics → SecurityHeaders → CORS.
// Middleware auth/tenant/rbac dipasang PER-GROUP, bukan global, supaya endpoint
// publik (health, login, tracking publik) tidak ikut terkunci.
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

	// ── Observability (tanpa auth) ────────────────────────────────────────
	r.Group(func(pub chi.Router) {
		pub.Get("/healthz", handler.Health)
		pub.Method(http.MethodGet, "/readyz", handler.Ready(d.Ready))
		if d.Metrics != nil && d.Config.Obs.MetricsOn {
			pub.Handle("/metrics", d.Metrics.Handler())
		}
	})

	r.Route("/api/v1", func(api chi.Router) {
		if d.APILimit != nil {
			api.Use(middleware.RateLimit(d.APILimit, middleware.KeyByUserOrIP))
		}

		registerAuthRoutes(api, d)
		registerOrganizationRoutes(api, d)
		registerRegionRoutes(api, d)
		registerAddressRoutes(api, d)

		// Modul berikutnya (stores, shipments, tracking, cod, ...) didaftarkan
		// pada fase terkait — lihat dev-docs/ai/TASKS.md.
	})

	return r
}

// registerAuthRoutes mendaftarkan grup /auth.
//
// Dua sub-grup: publik (rate limit ketat per IP karena menjadi sasaran
// credential stuffing) dan privat (butuh access token).
func registerAuthRoutes(api chi.Router, d Deps) {
	api.Route("/auth", func(authGroup chi.Router) {
		if d.AuthHandler == nil {
			return
		}

		// ── Publik ────────────────────────────────────────────────────────
		authGroup.Group(func(pub chi.Router) {
			if d.APILimit != nil {
				// Endpoint kredensial: dibatasi per IP, bukan per pengguna,
				// karena penyerang belum punya identitas.
				pub.Use(middleware.RateLimit(d.APILimit, middleware.KeyByIP))
			}
			pub.Post("/register", d.AuthHandler.Register)
			pub.Post("/login", d.AuthHandler.Login)
			pub.Post("/2fa/verify", d.AuthHandler.VerifyTwoFactor)
			pub.Post("/refresh", d.AuthHandler.Refresh)
			pub.Post("/forgot-password", d.AuthHandler.ForgotPassword)
			pub.Post("/reset-password", d.AuthHandler.ResetPassword)
			pub.Post("/verify-email", d.AuthHandler.VerifyEmail)
			pub.Post("/resend-verification", d.AuthHandler.ResendVerification)

			if d.GoogleOAuthHandler != nil {
				pub.Get("/google", d.GoogleOAuthHandler.Redirect)
				pub.Get("/google/callback", d.GoogleOAuthHandler.Callback)
			}
		})

		// ── Privat ────────────────────────────────────────────────────────
		authGroup.Group(func(priv chi.Router) {
			if d.Authenticate == nil {
				return
			}
			priv.Use(middleware.Auth(d.Authenticate))

			priv.Get("/me", d.AuthHandler.Me)
			priv.Patch("/me", d.AuthHandler.UpdateProfile)
			priv.Post("/change-password", d.AuthHandler.ChangePassword)

			priv.Post("/logout", d.AuthHandler.Logout)
			priv.Post("/logout-all", d.AuthHandler.LogoutAll)

			priv.Get("/sessions", d.AuthHandler.ListSessions)
			priv.Delete("/sessions/{id}", d.AuthHandler.RevokeSession)

			priv.Get("/2fa", d.AuthHandler.TwoFactorStatus)
			priv.Post("/2fa/setup", d.AuthHandler.SetupTwoFactor)
			priv.Post("/2fa/enable", d.AuthHandler.EnableTwoFactor)
			priv.Post("/2fa/disable", d.AuthHandler.DisableTwoFactor)
		})
	})
}

// registerOrganizationRoutes mendaftarkan grup /organizations.
//
// Daftar & pembuatan organisasi hanya butuh identitas (belum ada tenant aktif),
// sedangkan operasi di dalam organisasi lewat rantai auth → tenant → rbac.
func registerOrganizationRoutes(api chi.Router, d Deps) {
	api.Route("/organizations", func(orgs chi.Router) {
		if d.OrganizationHandler == nil || d.Authenticate == nil {
			return
		}
		orgs.Use(middleware.Auth(d.Authenticate))

		// Berbasis identitas pengguna, bukan tenant aktif.
		orgs.Get("/", d.OrganizationHandler.List)
		orgs.Post("/", d.OrganizationHandler.Create)

		// Berbasis organisasi di path; keanggotaan diverifikasi service.
		orgs.Route("/{id}", func(one chi.Router) {
			one.Get("/", d.OrganizationHandler.Get)
			one.Patch("/", d.OrganizationHandler.Update)

			one.Get("/members", d.OrganizationHandler.ListMembers)
			one.Post("/members", d.OrganizationHandler.InviteMember)
			one.Patch("/members/{mid}", d.OrganizationHandler.UpdateMember)
			one.Delete("/members/{mid}", d.OrganizationHandler.RemoveMember)

			one.Get("/roles", d.OrganizationHandler.ListRoles)
			one.Get("/permissions", d.OrganizationHandler.ListPermissions)
		})
	})
}

// registerRegionRoutes mendaftarkan grup /regions (publik, rate-limited).
func registerRegionRoutes(api chi.Router, d Deps) {
	api.Route("/regions", func(regions chi.Router) {
		if d.RegionHandler == nil {
			return
		}
		if d.APILimit != nil {
			regions.Use(middleware.RateLimit(d.APILimit, middleware.KeyByIP))
		}
		d.RegionHandler.RegisterRoutes(regions)
	})
}

// registerAddressRoutes mendaftarkan grup /addresses (butuh auth + tenant).
func registerAddressRoutes(api chi.Router, d Deps) {
	api.Route("/addresses", func(addrs chi.Router) {
		if d.AddressHandler == nil || d.Authenticate == nil {
			return
		}
		addrs.Use(middleware.Auth(d.Authenticate))
		addrs.Use(middleware.Tenant())
		d.AddressHandler.RegisterRoutes(addrs)
	})
}
