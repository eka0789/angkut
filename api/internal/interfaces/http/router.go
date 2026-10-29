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

	// PublicTrackLimit membatasi pencarian resi publik per IP. Sengaja
	// terpisah dari APILimit: resi berurutan sehingga endpoint ini adalah
	// oracle enumerasi, dan batasnya memang dibuat lebih ketat
	// (RATE_LIMIT_PUBLIC_TRACK_PER_MIN, default 30/menit).
	PublicTrackLimit middleware.Limiter

	// BulkLimit membatasi endpoint pengiriman massal PER ORGANISASI
	// (RATE_LIMIT_BULK_PER_MIN, default 5/menit — sesuai api-contract).
	BulkLimit middleware.Limiter

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
	BulkHandler         *handler.BulkHandler
	PickupHandler       *handler.PickupHandler
	InventoryHandler    *handler.InventoryHandler
	MoneyHandler        *handler.MoneyHandler
	AIHandler           *handler.AIHandler
	NotificationHandler *handler.NotificationHandler
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
		registerInventoryRoutes(api, d)
		registerMoneyRoutes(api, d)
		registerAIRoutes(api, d)
		registerNotificationRoutes(api, d)
		registerShipmentRoutes(api, d)
		registerTrackingRoutes(api, d)
		registerPickupRoutes(api, d)
		registerPublicTrackingRoutes(api, d)
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
			// Stok per gudang (TASK-038) — baca member.
			if d.InventoryHandler != nil {
				d.InventoryHandler.RegisterWarehouseStock(read, nil)
			}
		})

		wh.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleAdmin))
			write.Post("/", d.StoreHandler.CreateWarehouse)
			write.Patch("/{id}", d.StoreHandler.UpdateWarehouse)
		})

		// Transfer stok antar gudang: warehouse+ (kontrak TASK-038).
		if d.InventoryHandler != nil {
			wh.Group(func(transfer chi.Router) {
				transfer.Use(middleware.RequireRole(organization.RoleWarehouse))
				d.InventoryHandler.RegisterWarehouseStock(nil, transfer)
			})
		}
	})
}

// registerInventoryRoutes mendaftarkan grup /products dan /inventory
// (auth + tenant + RBAC). Baca katalog & saldo: member; mutasi stok dan
// produk: warehouse+ (kontrak modul 11); arsip produk: admin+.
func registerInventoryRoutes(api chi.Router, d Deps) {
	if d.InventoryHandler == nil || d.Authenticate == nil {
		return
	}
	api.Route("/products", func(pr chi.Router) {
		pr.Use(middleware.Auth(d.Authenticate))
		pr.Use(middleware.Tenant())

		// Baca: member (warehouse.view). HANYA GET — jangan mendaftar method
		// tulis di grup ini (tanpa cek role, PATCH/DELETE jadi lolos RBAC).
		pr.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("warehouse.view"))
			read.Get("/", d.InventoryHandler.ListProducts)
			read.Get("/{id}", d.InventoryHandler.GetProduct)
		})

		// Tulis: warehouse+.
		pr.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleWarehouse))
			write.Post("/", d.InventoryHandler.CreateProduct)
			write.Patch("/{id}", d.InventoryHandler.UpdateProduct)
			write.Post("/{id}/variants", d.InventoryHandler.AddVariant)
			write.Patch("/{id}/variants/{vid}", d.InventoryHandler.UpdateVariant)
		})

		// Arsip (DELETE kontrak = admin+).
		pr.Group(func(admin chi.Router) {
			admin.Use(middleware.RequireRole(organization.RoleAdmin))
			admin.Delete("/{id}", d.InventoryHandler.ArchiveProduct)
		})
	})

	api.Route("/inventory", func(inv chi.Router) {
		inv.Use(middleware.Auth(d.Authenticate))
		inv.Use(middleware.Tenant())

		inv.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("warehouse.view"))
			read.Get("/", d.InventoryHandler.ListStock)
			read.Get("/movements", d.InventoryHandler.Movements)
			read.Get("/low-stock", d.InventoryHandler.LowStock)
		})

		inv.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleWarehouse))
			write.Post("/adjust", d.InventoryHandler.Adjust)
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
			// Status job massal (rute statis "bulk" menang atas "/{id}"
			// pada tree chi).
			if d.BulkHandler != nil {
				read.Get("/bulk/{job_id}", d.BulkHandler.Status)
			}
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

			// Pengiriman massal (TASK-039): dibatasi 5 req/menit per org.
			if d.BulkHandler != nil {
				write.Group(func(bulk chi.Router) {
					limiter := d.BulkLimit
					if limiter == nil {
						limiter = d.APILimit
					}
					if limiter != nil {
						bulk.Use(middleware.RateLimit(limiter, middleware.KeyByIdentityOrg))
					}
					bulk.Post("/bulk/upload", d.BulkHandler.Upload)
					bulk.Post("/bulk/validate", d.BulkHandler.Validate)
					bulk.Group(func(sub chi.Router) {
						sub.Use(middleware.RequireIdempotency)
						sub.Post("/bulk", d.BulkHandler.Submit)
					})
				})
				// Label massal: unduh ZIP satu PDF per pengiriman.
				write.Post("/labels/bulk", d.ShipmentHandler.BulkLabels)
			}
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

		// Stream SSE didaftarkan TERPISAH dari grup ber-auth header.
		// EventSource tidak bisa mengirim header Authorization, jadi stream
		// memakai AuthQuery (header Bearer ATAU `token` di query). Bila stream
		// digabung ke grup yang sudah memakai Auth(header), middleware Auth
		// berjalan lebih dulu dan menolak koneksi dengan 401 sebelum AuthQuery
		// sempat membaca token di query.
		if d.StreamHandler != nil {
			tr.Group(func(stream chi.Router) {
				stream.Use(middleware.AuthQuery(d.Authenticate))
				stream.Use(middleware.Tenant())
				stream.Use(middleware.RequirePermission("tracking.view"))
				stream.Get("/{awb}/stream", d.StreamHandler.TrackSSE)
			})
		}

		tr.Group(func(secured chi.Router) {
			secured.Use(middleware.Auth(d.Authenticate))
			secured.Use(middleware.Tenant())

			secured.Group(func(read chi.Router) {
				read.Use(middleware.RequirePermission("tracking.view"))
				read.Get("/{awb}", d.TrackingHandler.ByAWB)
				read.Get("/events", d.TrackingHandler.List)
			})

			// Pencatatan event manual adalah aksi staf.
			// Chi memprioritaskan rute statis di atas parameter, jadi "/events"
			// tidak tertelan sebagai nilai resi.
			secured.Group(func(write chi.Router) {
				write.Use(middleware.RequireRole(organization.RoleStaff))
				write.Post("/events", d.TrackingHandler.Append)
			})
		})
	})
}

// registerPickupRoutes mendaftarkan grup /pickups (auth + tenant + RBAC).
func registerPickupRoutes(api chi.Router, d Deps) {
	api.Route("/pickups", func(pk chi.Router) {
		if d.PickupHandler == nil || d.Authenticate == nil {
			return
		}
		pk.Use(middleware.Auth(d.Authenticate))
		pk.Use(middleware.Tenant())

		pk.Group(func(read chi.Router) {
			read.Use(middleware.RequirePermission("shipment.view"))
			read.Get("/", d.PickupHandler.List)
			read.Get("/{id}", d.PickupHandler.Get)
		})

		pk.Group(func(write chi.Router) {
			// Kontrak modul: mutasi pickup = warehouse+ (owner/admin/finance/
			// warehouse), sedangkan pembacaan cukup member.
			write.Use(middleware.RequireRole(organization.RoleWarehouse))
			write.Post("/", d.PickupHandler.Create)
			write.Post("/{id}/schedule", d.PickupHandler.Schedule)
			write.Post("/{id}/complete", d.PickupHandler.Complete)
			write.Post("/{id}/cancel", d.PickupHandler.Cancel)
		})
	})
}

// registerNotificationRoutes mendaftarkan grup /notifications dan
// /notification-templates (auth + tenant). Pusat notifikasi adalah data
// PRIBADI di dalam tenant: baca tanpa permission tambahan (difilter user_id),
// template hanya admin+ (notification.manage).
func registerNotificationRoutes(api chi.Router, d Deps) {
	api.Route("/notifications", func(nc chi.Router) {
		if d.NotificationHandler == nil || d.Authenticate == nil {
			return
		}
		nc.Use(middleware.Auth(d.Authenticate))
		nc.Use(middleware.Tenant())
		d.NotificationHandler.RegisterRoutes(nc)
	})

	api.Route("/notification-templates", func(tc chi.Router) {
		if d.NotificationHandler == nil || d.Authenticate == nil {
			return
		}
		tc.Use(middleware.Auth(d.Authenticate))
		tc.Use(middleware.Tenant())
		tc.Group(func(admin chi.Router) {
			admin.Use(middleware.RequirePermission("notification.manage"))
			admin.Get("/", d.NotificationHandler.Templates)
			admin.Put("/{key}", d.NotificationHandler.UpdateTemplate)
		})
	})

	api.Route("/notification-preferences", func(pc chi.Router) {
		if d.NotificationHandler == nil || d.Authenticate == nil {
			return
		}
		pc.Use(middleware.Auth(d.Authenticate))
		pc.Use(middleware.Tenant())
		pc.Get("/", d.NotificationHandler.Preferences)
		pc.Put("/", d.NotificationHandler.UpdatePreference)
	})
}

// registerMoneyRoutes mendaftarkan grup COD, ledger & returns.
// RBAC per kontrak: COD & ledger finance+; retur member baca / staff+ buat /
// admin+ keputusan & kebijakan / warehouse+ receive & inspect / finance+ refund.
func registerMoneyRoutes(api chi.Router, d Deps) {
	if d.MoneyHandler == nil || d.Authenticate == nil {
		return
	}

	api.Route("/cod", func(c chi.Router) {
		c.Use(middleware.Auth(d.Authenticate))
		c.Use(middleware.Tenant())
		c.Use(middleware.RequirePermission("cod.view"))
		c.Group(func(write chi.Router) {
			write.Use(middleware.RequirePermission("cod.manage"))
			write.Post("/settlements", d.MoneyHandler.CreateSettlement)
			write.Post("/settlements/{id}/mark-paid", d.MoneyHandler.MarkPaid)
		})
		d.MoneyHandler.RegisterCOD(c)
	})

	api.Route("/payments", func(p chi.Router) {
		p.Use(middleware.Auth(d.Authenticate))
		p.Use(middleware.Tenant())
		p.Use(middleware.RequirePermission("cod.view"))
		p.Group(func(write chi.Router) {
			write.Use(middleware.RequirePermission("cod.settle"))
			write.Use(middleware.RequireIdempotency)
			write.Post("/", d.MoneyHandler.RecordPayment)
		})
		d.MoneyHandler.RegisterPayments(p)
	})

	api.Route("/invoices", func(iv chi.Router) {
		iv.Use(middleware.Auth(d.Authenticate))
		iv.Use(middleware.Tenant())
		iv.Use(middleware.RequirePermission("cod.view"))
		d.MoneyHandler.RegisterPayments(iv)
	})

	api.Route("/ledger", func(l chi.Router) {
		l.Use(middleware.Auth(d.Authenticate))
		l.Use(middleware.Tenant())
		l.Use(middleware.RequirePermission("cod.view"))
		d.MoneyHandler.RegisterLedger(l)
	})

	api.Route("/returns", func(rt chi.Router) {
		rt.Use(middleware.Auth(d.Authenticate))
		rt.Use(middleware.Tenant())

		rt.Group(func(member chi.Router) {
			d.MoneyHandler.RegisterReturns(member, nil, nil, nil, nil)
		})

		rt.Group(func(write chi.Router) {
			write.Use(middleware.RequireRole(organization.RoleStaff))
			d.MoneyHandler.RegisterReturns(nil, write, nil, nil, nil)
		})

		rt.Group(func(admin chi.Router) {
			admin.Use(middleware.RequirePermission("return.manage"))
			d.MoneyHandler.RegisterReturns(nil, nil, admin, nil, nil)
		})

		rt.Group(func(warehouse chi.Router) {
			warehouse.Use(middleware.RequireRole(organization.RoleWarehouse))
			d.MoneyHandler.RegisterReturns(nil, nil, nil, warehouse, nil)
		})

		rt.Group(func(finance chi.Router) {
			finance.Use(middleware.RequirePermission("cod.manage"))
			d.MoneyHandler.RegisterReturns(nil, nil, nil, nil, finance)
		})
	})

	api.Route("/return-policies", func(rp chi.Router) {
		rp.Use(middleware.Auth(d.Authenticate))
		rp.Use(middleware.Tenant())
		rp.Group(func(member chi.Router) {
			d.MoneyHandler.RegisterPolicy(member, nil)
		})
		rp.Group(func(admin chi.Router) {
			admin.Use(middleware.RequirePermission("return.manage"))
			d.MoneyHandler.RegisterPolicy(nil, admin)
		})
	})
}

// registerAIRoutes mendaftarkan grup /ai dan /analytics (auth+tenant, member).
// Aksi mutasi AI tetap butuh konfirmasi eksplisit (TASK-050).
func registerAIRoutes(api chi.Router, d Deps) {
	if d.AIHandler == nil || d.Authenticate == nil {
		return
	}
	api.Route("/ai", func(a chi.Router) {
		a.Use(middleware.Auth(d.Authenticate))
		a.Use(middleware.Tenant())
		d.AIHandler.RegisterAI(a)
	})
	api.Route("/analytics", func(a chi.Router) {
		a.Use(middleware.Auth(d.Authenticate))
		a.Use(middleware.Tenant())
		d.AIHandler.RegisterAnalytics(a)
		a.Group(func(fin chi.Router) {
			// Laporan keuangan: finance+ (cod.view).
			fin.Use(middleware.RequirePermission("cod.view"))
			fin.Get("/financial", d.MoneyHandler.Financial)
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

// registerPublicTrackingRoutes mendaftarkan grup /public (tanpa auth).
//
// Halaman pelacakan publik dipakai penerima paket yang tidak punya akun —
// tautan dibagikan lewat share-link pengiriman. Karena itu:
//   - tidak ada auth/tenant; resi unik global sehingga lookup lintas organisasi
//     aman selama payload dibatasi pada data yang tercetak di label;
//   - rate limit per IP dipasang agar resi tidak bisa dienumerasi secara massal
//     (AWB berurutan, jadi tanpa batas ini adalah oracle pencarian).
func registerPublicTrackingRoutes(api chi.Router, d Deps) {
	api.Route("/public", func(pub chi.Router) {
		if d.TrackingHandler == nil {
			return
		}
		limiter := d.PublicTrackLimit
		if limiter == nil {
			limiter = d.APILimit
		}
		if limiter != nil {
			pub.Use(middleware.RateLimit(limiter, middleware.KeyByIP))
		}
		pub.Get("/track/{awb}", d.TrackingHandler.PublicByAWB)
	})
}
