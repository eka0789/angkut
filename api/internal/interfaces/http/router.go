// Package http menyusun router dan rantai middleware. Tidak ada logika bisnis
// di paket ini — hanya registrasi route dan penyambungan handler.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/domain/organization"
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
	CustomerHandler     *handler.CustomerHandler
	CourierHandler      *handler.CourierHandler
	StoreHandler        *handler.StoreHandler
	OrderHandler        *handler.OrderHandler
	ShipmentHandler     *handler.ShipmentHandler
	TrackingHandler     *handler.TrackingHandler
	WebhookHandler      *handler.WebhookHandler
	StreamHandler       *handler.StreamHandler
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

		// ── Streaming WebSocket (/ws) — autentikasi via query token ──────
		// EventSource/WS browser tidak bisa menempel header; lihat AuthQuery.
		api.Group(func(ws chi.Router) {
			if d.StreamHandler == nil || d.Authenticate == nil {
				return
			}
			ws.Use(middleware.AuthQuery(d.Authenticate))
			ws.Use(middleware.Tenant())
			ws.Get("/ws", d.StreamHandler.OrgWS)
		})

		registerAuthRoutes(api, d)
		registerOrganizationRoutes(api, d)
		registerRegionRoutes(api, d)
		registerAddressRoutes(api, d)
		registerCustomerRoutes(api, d)
		registerCourierRoutes(api, d)
		registerStoreRoutes(api, d)
		registerOrderRoutes(api, d)
		registerShipmentRoutes(api, d)
		registerTrackingRoutes(api, d)
		registerWebhookRoutes(api, d)

		// Modul berikutnya (cod, analytics, ...) didaftarkan pada fase terkait —
		// lihat dev-docs/ai/TASKS.md.
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

// registerCustomerRoutes mendaftarkan grup /customers (auth + tenant + RBAC).
//
// Kontrak api-contract.md: baca customer.view, tulis customer.manage,
// delete admin+. Middleware dipasang per-subgroup karena rantainya beda.
func registerCustomerRoutes(api chi.Router, d Deps) {
	api.Route("/customers", func(customers chi.Router) {
		if d.CustomerHandler == nil || d.Authenticate == nil {
			return
		}
		customers.Use(middleware.Auth(d.Authenticate))
		customers.Use(middleware.Tenant())

		// Baca: customer.view.
		customers.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("customer.view"))
			read.Get("/", d.CustomerHandler.List)
			read.Route("/{id}", func(one chi.Router) {
				one.Get("/", d.CustomerHandler.Get)
				one.Get("/orders", d.CustomerHandler.ListOrders)
				one.Get("/stats", d.CustomerHandler.Stats)
			})
		})

		// Tulis: customer.manage.
		customers.Group(func(write chi.Router) {
			write.Use(middleware.RequirePermission("customer.manage"))
			write.Post("/", d.CustomerHandler.Create)
			write.Patch("/{id}", d.CustomerHandler.Update)
			write.Post("/{id}/segment", d.CustomerHandler.SetSegment)
		})

		// Hapus: admin+.
		customers.Group(func(admin chi.Router) {
			admin.Use(middleware.RequireRole(organization.RoleAdmin))
			admin.Delete("/{id}", d.CustomerHandler.Delete)
		})
	})
}

// registerCourierRoutes mendaftarkan grup /couriers (auth + tenant + RBAC).
// Semua rute TASK-022 read-only (shipment.view); konfigurasi kurir menyusul
// di Admin Panel (Phase 7).
func registerCourierRoutes(api chi.Router, d Deps) {
	api.Route("/couriers", func(couriers chi.Router) {
		if d.CourierHandler == nil || d.Authenticate == nil {
			return
		}
		couriers.Use(middleware.Auth(d.Authenticate))
		couriers.Use(middleware.Tenant())
		couriers.Use(middleware.RequirePermission("shipment.view"))
		d.CourierHandler.RegisterRoutes(couriers)
	})
}

// registerStoreRoutes mendaftarkan grup /stores dan /warehouses
// (auth + tenant + RBAC). Keduanya prasyarat order & shipment: order berasal
// dari store, shipment berangkat dari gudang (asal pengiriman).
func registerStoreRoutes(api chi.Router, d Deps) {
	api.Route("/stores", func(stores chi.Router) {
		if d.StoreHandler == nil || d.Authenticate == nil {
			return
		}
		stores.Use(middleware.Auth(d.Authenticate))
		stores.Use(middleware.Tenant())

		// Baca: member (store.view).
		stores.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("store.view"))
			read.Get("/", d.StoreHandler.ListStores)
			read.Get("/{id}", d.StoreHandler.GetStore)
		})

		// Tulis: admin+ (kontrak API: POST/PATCH/DELETE stores = admin+).
		stores.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleAdmin))
			write.Post("/", d.StoreHandler.CreateStore)
			write.Patch("/{id}", d.StoreHandler.UpdateStore)
		})
	})

	api.Route("/warehouses", func(wh chi.Router) {
		if d.StoreHandler == nil || d.Authenticate == nil {
			return
		}
		wh.Use(middleware.Auth(d.Authenticate))
		wh.Use(middleware.Tenant())

		wh.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("warehouse.view"))
			read.Get("/", d.StoreHandler.ListWarehouses)
			read.Get("/{id}", d.StoreHandler.GetWarehouse)
		})

		wh.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleAdmin))
			write.Post("/", d.StoreHandler.CreateWarehouse)
			write.Patch("/{id}", d.StoreHandler.UpdateWarehouse)
		})
	})
}

// registerOrderRoutes mendaftarkan grup /orders (auth + tenant + RBAC).
// Kontrak: baca = member (order.view), tulis = staff+ (order.manage).
func registerOrderRoutes(api chi.Router, d Deps) {
	api.Route("/orders", func(orders chi.Router) {
		if d.OrderHandler == nil || d.Authenticate == nil {
			return
		}
		orders.Use(middleware.Auth(d.Authenticate))
		orders.Use(middleware.Tenant())

		// Baca: member.
		orders.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("order.view"))
			read.Get("/", d.OrderHandler.List)
			read.Get("/summary", d.OrderHandler.Summary)
			read.Get("/{id}", d.OrderHandler.Get)
		})

		// Tulis: staff+.
		orders.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleStaff))
			write.Post("/", d.OrderHandler.Create)
			write.Patch("/{id}", d.OrderHandler.Update)
			write.Post("/{id}/cancel", d.OrderHandler.Cancel)
		})
	})
}

// registerShipmentRoutes mendaftarkan grup /shipments (auth + tenant + RBAC).
// Kontrak: baca = member (shipment.view), tulis = staff+ (shipment.manage).
//
// Mutasi POST /shipments WAJIB menyertakan Idempotency-Key (TASK-028):
// RequireIdempotency menolaknya lebih awal sehingga klien tidak bisa "lupa"
// dan membuat pengiriman ganda lewat retry.
func registerShipmentRoutes(api chi.Router, d Deps) {
	api.Route("/shipments", func(ships chi.Router) {
		if d.ShipmentHandler == nil || d.Authenticate == nil {
			return
		}
		ships.Use(middleware.Auth(d.Authenticate))
		ships.Use(middleware.Tenant())

		// Baca: member.
		ships.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("shipment.view"))
			read.Get("/", d.ShipmentHandler.List)
			read.Route("/{id}", func(one chi.Router) {
				one.Get("/", d.ShipmentHandler.Get)
				one.Get("/label", d.ShipmentHandler.Label)
				one.Get("/share-link", d.ShipmentHandler.ShareLink)
			})
		})

		// Tulis: staff+.
		ships.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleStaff))
			write.Group(func(mut chi.Router) {
				// Pembuatan pengiriman wajib idempotent.
				mut.Use(middleware.RequireIdempotency)
				mut.Post("/", d.ShipmentHandler.Create)
			})
			write.Patch("/{id}", d.ShipmentHandler.Update)
			write.Post("/{id}/cancel", d.ShipmentHandler.Cancel)
		})
	})
}

// registerTrackingRoutes mendaftarkan grup /tracking (auth + tenant + RBAC).
// Pelacakan bersifat baca bagi semua role (tracking.view adalah izin baca),
// sedangkan pencatatan event manual adalah aksi staf.
func registerTrackingRoutes(api chi.Router, d Deps) {
	api.Route("/tracking", func(tr chi.Router) {
		if d.TrackingHandler == nil || d.Authenticate == nil {
			return
		}
		tr.Use(middleware.Auth(d.Authenticate))
		tr.Use(middleware.Tenant())

		tr.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("tracking.view"))
			read.Get("/{awb}", d.TrackingHandler.ByAWB)
			read.Get("/events", d.TrackingHandler.List)
		})

		// Catatan: rute statis "/events" didaftarkan sebelum "/{awb}" pada
		// RegisterRoutes handler agar tidak tertelan sebagai resi.
		tr.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleStaff))
			write.Post("/events", d.TrackingHandler.Append)
		})

		// Stream SSE: autentikasi lewat query token (EventSource), bukan header.
		tr.Group(func(stream chi.Router) {
			stream.Use(middleware.AuthQuery(d.Authenticate))
			stream.Use(middleware.Tenant())
			stream.Use(middleware.RequirePermission("tracking.view"))
			stream.Get("/{awb}/stream", d.StreamHandler.TrackSSE)
		})
	})
}

// registerWebhookRoutes mendaftarkan grup /webhooks.
//
// Endpoint INGEST bersifat PUBLIK: pemanggilnya kurir/store, bukan pengguna
// ber-token. Keamanannya bergantung pada verifikasi HMAC di application layer,
// ditambah rate limit per IP agar tidak menjadi jalur penyalahgunaan.
// Daftar event webhook tetap privat (memuat payload pihak ketiga).
func registerWebhookRoutes(api chi.Router, d Deps) {
	api.Route("/webhooks", func(wh chi.Router) {
		if d.WebhookHandler == nil {
			return
		}

		wh.Group(func(pub chi.Router) {
			if d.APILimit != nil {
				pub.Use(middleware.RateLimit(d.APILimit, middleware.KeyByIP))
			}
			pub.Post("/{provider}", d.WebhookHandler.Ingest)
		})

		if d.Authenticate == nil {
			return
		}
		wh.Group(func(read chi.Router) {
			read.Use(middleware.Auth(d.Authenticate))
			read.Use(middleware.Tenant())
			read.Use(middleware.RequireRole(organization.RoleAdmin))
			read.Get("/events", d.WebhookHandler.List)
		})
	})
}
