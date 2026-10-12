package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/angkut/angkut/apps/api/internal/application/auth"
	"github.com/angkut/angkut/apps/api/internal/application/organizationapp"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/devmail"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/googleoauth"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/tokenx"
	apihttp "github.com/angkut/angkut/apps/api/internal/interfaces/http"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/config"
	"github.com/angkut/angkut/apps/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildRouter merakit seluruh lapisan aplikasi dan mengembalikan router siap
// pakai. Semua dependensi disambungkan secara eksplisit di sini (constructor
// injection) — tidak ada service locator atau state global.
//
// Bila pool nil (mis. mode dev tanpa database), router tetap dibangun tanpa
// endpoint yang membutuhkan DB — health & metrics tetap hidup.
func buildRouter(cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool, metrics *observability.Registry, checkers []handler.Checker) (http.Handler, error) {
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

	return apihttp.NewRouter(deps), nil
}
