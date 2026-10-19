package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	apihttp "github.com/angkut/angkut/apps/api/internal/interfaces/http"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/application/auth"
	"github.com/angkut/angkut/apps/internal/application/orderapp"
	"github.com/angkut/angkut/apps/internal/application/organizationapp"
	"github.com/angkut/angkut/apps/internal/application/rates"
	"github.com/angkut/angkut/apps/internal/application/shipmentapp"
	"github.com/angkut/angkut/apps/internal/application/trackingapp"
	"github.com/angkut/angkut/apps/internal/application/webhookapp"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/domain/courier"
	"github.com/angkut/angkut/apps/internal/domain/pricing"
	"github.com/angkut/angkut/apps/internal/domain/scoring"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/infrastructure/devmail"
	"github.com/angkut/angkut/apps/internal/infrastructure/googleoauth"
	"github.com/angkut/angkut/apps/internal/infrastructure/mockcourier"
	"github.com/angkut/angkut/apps/internal/infrastructure/tokenx"
	"github.com/angkut/angkut/apps/internal/observability"
	"github.com/angkut/angkut/apps/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// buildRouter merakit seluruh lapisan aplikasi dan mengembalikan router siap
// pakai. Semua dependensi disambungkan secara eksplisit di sini (constructor
// injection) — tidak ada service locator atau state global.
//
// Bila pool nil (mis. mode dev tanpa database), router tetap dibangun tanpa
// endpoint yang membutuhkan DB — health & metrics tetap hidup.
func buildRouter(cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool, rdb *redis.Client, metrics *observability.Registry, checkers []handler.Checker) (http.Handler, error) {
	deps := apihttp.Deps{
		Config:   cfg,
		Metrics:  metrics,
		Ready:    checkers,
		APILimit: middleware.NewMemoryLimiter(cfg.Rate.APIPerMin),
	}

	if pool == nil {
		// Tanpa DB: hanya observability yang tersedia.
		return apihttp.NewRouter(deps), nil
	}

	stores := database.NewStores(pool)
	tx := database.NewTxManager(pool)

	// ── Primitif keamanan ────────────────────────────────────────────────
	jwtIssuer, err := tokenx.New(cfg.Auth.AccessSecret, cfg.Auth.AccessTTL)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan JWT: %w", err)
	}
	mfaIssuer, err := tokenx.NewMFAIssuer(cfg.Auth.AccessSecret, 0)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan MFA: %w", err)
	}
	cipher, err := cryptox.NewCipher(cfg.Auth.SecretsKey)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan enkripsi rahasia: %w", err)
	}

	// ── Integrasi opsional ───────────────────────────────────────────────
	googleClient := googleoauth.New(
		cfg.Auth.GoogleClientID, cfg.Auth.GoogleClientSecret, cfg.Auth.GoogleRedirectURL)
	if googleClient.Enabled() {
		log.Info("google oauth aktif")
	} else {
		log.Info("google oauth nonaktif (GOOGLE_CLIENT_ID/SECRET kosong)")
	}

	// Mailer dev menulis tautan verifikasi/reset ke log. Modul Notification
	// (Phase 4) akan menggantikannya lewat port yang sama.
	mailer := devmail.New(log, cfg.API.WebURL)

	// ── Application services ─────────────────────────────────────────────
	authSvc := auth.New(auth.Deps{
		DB:     stores,
		Tx:     tx,
		Audit:  stores.Audit(),
		JWT:    jwtIssuer,
		MFA:    mfaIssuer,
		Cipher: cipher,
		Params: cryptox.Argon2Params{
			MemoryKiB:   cfg.Auth.ArgonMemoryKiB,
			Iterations:  cfg.Auth.ArgonIter,
			Parallelism: cfg.Auth.ArgonParallel,
			SaltLength:  16,
			KeyLength:   32,
		},
		TOTP:   tokenx.DefaultTOTPConfig(),
		Mailer: mailer,
		WebURL: cfg.API.WebURL,
	})

	orgSvc := organizationapp.New(organizationapp.Deps{
		DB:    stores,
		Tx:    tx,
		Audit: stores.Audit(),
	})

	// ── Handlers ─────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc, googleClient, cfg.Auth.OAuthStateSecret)
	orgHandler := handler.NewOrganizationHandler(orgSvc)
	oauthHandler := handler.NewGoogleOAuthHandler(authSvc, googleClient, cfg.Auth.OAuthStateSecret)
	regionHandler := handler.NewRegionHandler(stores.Regions())
	addressHandler := handler.NewAddressHandler(stores.Addresses(), stores.Regions())
	customerHandler := handler.NewCustomerHandler(stores.Customers())

	// Registry kurir: mock untuk semua kode seed; adapter sungguhan (Phase 4)
	// cukup ditambahkan di sini dengan kode yang sama.
	courierRepo := stores.Couriers()
	courierRegistry := courier.NewRegistry(
		mockcourier.New("jne", courierRepo),
		mockcourier.New("jnt", courierRepo),
		mockcourier.New("sicepat", courierRepo),
	)
	pricingEngine := pricing.New(pricing.Config{
		VolumetricDivisorDefault: cfg.Pricing.VolumetricDivisorDefault,
		WeightRoundUpStepKG:      cfg.Pricing.WeightRoundUpStepKG,
		MaxChargeableWeightKG:    cfg.Pricing.MaxChargeableWeightKG,
		CODFeeRateBps:            cfg.Pricing.CODFeeRateBps,
		CODFeeMin:                cfg.Pricing.CODFeeMin,
		InsuranceRateBps:         cfg.Pricing.InsuranceRateBps,
		InsuranceMinFee:          cfg.Pricing.InsuranceMinFee,
		MarkupFlat:               cfg.Pricing.MarkupFlat,
		MarkupRateBps:            cfg.Pricing.MarkupRateBps,
		PriceRoundStep:           cfg.Pricing.PriceRoundStep,
	})
	scoringEngine := scoring.New(scoring.Config{
		WeightPriceBps:       cfg.Scoring.WeightPriceBps,
		WeightSpeedBps:       cfg.Scoring.WeightSpeedBps,
		WeightReliabilityBps: cfg.Scoring.WeightReliabilityBps,
		ReasonThresholdPct:   cfg.Scoring.ReasonThresholdPct,
		HighReliabilityScore: cfg.Scoring.HighReliabilityScore,
		ConfidenceBaseBps:    cfg.Scoring.ConfidenceBaseBps,
		ConfidenceSpanBps:    cfg.Scoring.ConfidenceSpanBps,
		MaxReasons:           cfg.Scoring.MaxReasons,
	})
	ratesSvc := rates.New(rates.Deps{
		Repo:     courierRepo,
		Registry: courierRegistry,
		Engine:   pricingEngine,
		Scorer:   scoringEngine,
		Log:      log,
		Redis:    rdb,
	})
	courierHandler := handler.NewCourierHandler(ratesSvc)

	// ── Order & fulfillment (TASK-026) ───────────────────────────────────
	storeHandler := handler.NewStoreHandler(stores.StoresRepo(), stores.Warehouses())
	orderSvc := orderapp.New(orderapp.Deps{
		Tx:     tx,
		Stores: stores,
		Orders: stores.Orders(),
		Audit:  stores.Audit(),
		Log:    log,
	})
	orderHandler := handler.NewOrderHandler(orderSvc)

	// ── Shipment & label (TASK-027) ──────────────────────────────────────
	// Generator label dibentuk per-request di handler karena format (a4/thermal)
	// berbeda per permintaan — tidak ada state yang perlu dibagi.
	shipmentSvc := shipmentapp.New(shipmentapp.Deps{
		Tx:        tx,
		Stores:    stores,
		Shipments: stores.Shipments(),
		Couriers:  courierRepo,
		Registry:  courierRegistry,
		Orders:    stores.Orders(),
		Engine:    pricingEngine,
		Audit:     stores.Audit(),
		Log:       log,
	})
	shipmentHandler := handler.NewShipmentHandler(shipmentSvc)

	// ── Tracking & webhook ingress (TASK-030/031) ────────────────────────
	trackingSvc := trackingapp.New(trackingapp.Deps{
		Tx:     tx,
		Stores: stores,
		Events: stores.Tracking(),
		Audit:  stores.Audit(),
		Log:    log,
		// Notifier menerbitkan peristiwa ke Redis pub/sub (rdb nil → no-op yang
		// aman). Publish best-effort: kebenaran data tetap di database.
		Notifier: realtime.NewRedisPublisher(rdb),
	})
	trackingHandler := handler.NewTrackingHandler(trackingSvc)

	// ── Realtime hub (TASK-034) ──────────────────────────────────────────
	// Hub meneruskan pesan pub/sub ke koneksi SSE/WS lokal dalam proses ini.
	// Konteks latar proses-disengaja: hub bergantung hidup bersama server;
	// klien stream ditutup oleh ShutdownGrace (lihat main.go).
	var hub *realtime.Hub
	if rdb != nil {
		hub = realtime.NewHub(context.Background(), realtime.NewRedisHub(rdb), realtime.Patterns())
		log.Info("realtime hub aktif (redis pub/sub)")
	} else {
		hub = realtime.NewHub(context.Background(), nil, nil)
		log.Warn("redis kosong — realtime hanya snapshot awal (tanpa pembaruan hidup)")
	}
	streamHandler := handler.NewStreamHandler(trackingSvc, hub, log)

	// Rahasia signature webhook disimpan terenkripsi; cipher dibutuhkan untuk
	// membukanya saat memverifikasi — dan hanya di jalur itu.
	webhookSvc := webhookapp.New(webhookapp.Deps{
		Tx:      tx,
		Stores:  stores,
		Events:  stores.Webhooks(),
		Secrets: database.NewWebhookSecretRepo(stores.Querier(), cipher),
		Audit:   stores.Audit(),
		Log:     log,
	})
	webhookHandler := handler.NewWebhookHandler(webhookSvc)

	// Authenticator menjembatani middleware dengan application service tanpa
	// membuat paket middleware bergantung pada application.
	authenticate := func(ctx context.Context, rawToken string) (*middleware.Identity, error) {
		sc, err := authSvc.ValidateAccessToken(ctx, rawToken)
		if err != nil {
			return nil, err
		}
		return &middleware.Identity{
			UserID:         sc.UserID,
			SessionID:      sc.SessionID,
			Email:          sc.Email,
			Name:           sc.Name,
			OrganizationID: sc.OrganizationID,
			RoleCode:       sc.RoleCode,
		}, nil
	}

	deps.Authenticate = authenticate
	deps.AuthHandler = authHandler
	deps.OrganizationHandler = orgHandler
	deps.GoogleOAuthHandler = oauthHandler
	deps.RegionHandler = regionHandler
	deps.AddressHandler = addressHandler
	deps.CustomerHandler = customerHandler
	deps.CourierHandler = courierHandler
	deps.StoreHandler = storeHandler
	deps.OrderHandler = orderHandler
	deps.ShipmentHandler = shipmentHandler
	deps.TrackingHandler = trackingHandler
	deps.WebhookHandler = webhookHandler
	deps.StreamHandler = streamHandler

	return apihttp.NewRouter(deps), nil
}
