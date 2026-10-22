// Package config memuat & memvalidasi konfigurasi dari environment.
// Tidak ada credential default produksi di sini; dev default hanya agar
// `make api` langsung bisa jalan.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config adalah konfigurasi lengkap proses API/worker.
type Config struct {
	Env      string
	Name     string
	Timezone string
	LogLevel string

	API     APIConfig
	DB      DBConfig
	RDB     RedisConfig
	MQ      RabbitConfig
	S3      S3Config
	Mail    MailConfig
	Auth    AuthConfig
	Vendor  VendorConfig
	Obs     ObservabilityConfig
	Rate    RateLimitConfig
	Pricing PricingConfig
	Scoring ScoringConfig
	Bulk    BulkConfig
}

type APIConfig struct {
	Port           string
	APIURL         string
	WebURL         string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	ShutdownGrace  time.Duration
	AllowedOrigins []string
}

type DBConfig struct {
	URL      string
	MaxConns int32
	MinConns int32
}

type RedisConfig struct{ URL string }

type RabbitConfig struct {
	URL     string
	Enabled bool
}

type S3Config struct {
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	Region       string
	UsePathStyle bool
}

type MailConfig struct {
	SMTPURL string
	From    string
	Enabled bool
}

type AuthConfig struct {
	AccessSecret   string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	ArgonMemoryKiB uint32
	ArgonIter      uint32
	ArgonParallel  uint8
	// SecretsKey mengenkripsi rahasia at-rest (TOTP secret per user).
	// Wajib 32 byte base64. Tidak boleh sama dengan AccessSecret.
	SecretsKey string
	// Google OAuth (kosong = fitur mati dengan pesan jujur, bukan gagal diam).
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	// OAuthStateSecret menandatangani parameter state OAuth (anti-CSRF).
	OAuthStateSecret string
}

// VendorConfig menentukan adapter mana yang aktif. Nilai kosong/mock berarti
// platform berjalan penuh dengan adapter tiruan — tidak ada vendor wajib.
type VendorConfig struct {
	CourierAdapter string
	StoreAdapter   string
	PaymentAdapter string
	AIProvider     string
	WhatsAppAdap   string
}

type ObservabilityConfig struct {
	OTELEnabled  bool
	OTELEndpoint string
	MetricsOn    bool
}

type RateLimitConfig struct {
	AuthPerMin        int
	APIPerMin         int
	PublicTrackPerMin int
	// BulkPerMin membatasi endpoint pengiriman massal PER ORGANISASI:
	// kontrak mengizinkan 5 submit bulk/menit (api-contract, baris rate limit).
	BulkPerMin int
}

// BulkConfig membatasi unggahan pengiriman massal (TASK-039). Semua angka
// datang dari env dengan default bernama — tanpa literal di kode bisnis.
type BulkConfig struct {
	MaxRows      int     // maksimum baris per unggahan/job
	MaxWeightKG  float64 // maksimum berat per baris
	ChunkSize    int     // ukuran batch progres saat memproses baris
	MaxUploadMiB int     // maksimum ukuran file CSV unggahan
}

// validate menolak parameter bulk yang tidak masuk akal sebelum server menyala.
func (b BulkConfig) validate() error {
	var problems []string
	if b.MaxRows <= 0 {
		problems = append(problems, "BULK_MAX_ROWS harus > 0")
	}
	if b.MaxWeightKG <= 0 {
		problems = append(problems, "BULK_MAX_WEIGHT_KG harus > 0")
	}
	if b.ChunkSize <= 0 {
		problems = append(problems, "BULK_CHUNK_SIZE harus > 0")
	}
	if b.MaxUploadMiB <= 0 {
		problems = append(problems, "BULK_MAX_UPLOAD_MIB harus > 0")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// PricingConfig memuat seluruh parameter pricing engine (TASK-023).
// Semua angka berasal dari env dengan default yang diberi nama — logika bisnis
// tidak boleh memuat literal harga/berat. Rate dalam basis points: 10000 = 100%.
type PricingConfig struct {
	VolumetricDivisorDefault int     // pembagi volumetrik default (cm³/kg)
	WeightRoundUpStepKG      float64 // chargeable weight dibulatkan KE ATAS ke kelipatan ini
	MaxChargeableWeightKG    float64 // batas sanity berat chargeable
	CODFeeRateBps            int64   // fee COD = persentase nilai COD
	CODFeeMin                int64   // fee COD minimum (IDR)
	InsuranceRateBps         int64   // biaya asuransi = persentase nilai barang
	InsuranceMinFee          int64   // asuransi minimum (IDR)
	MarkupFlat               int64   // markup platform tetap per shipment (IDR)
	MarkupRateBps            int64   // markup platform dari harga dasar kurir
	PriceRoundStep           int64   // pembulatan harga akhir ke kelipatan ini (IDR)
}

// validate menolak parameter pricing yang tidak masuk akal sebelum server
// menyala — salah konfigurasi harga lebih mahal daripada gagal cepat.
func (p PricingConfig) validate() error {
	var problems []string
	if p.VolumetricDivisorDefault <= 0 {
		problems = append(problems, "PRICING_VOLUMETRIC_DIVISOR harus > 0")
	}
	if p.WeightRoundUpStepKG <= 0 {
		problems = append(problems, "PRICING_WEIGHT_ROUND_STEP_KG harus > 0")
	}
	if p.MaxChargeableWeightKG <= 0 {
		problems = append(problems, "PRICING_MAX_CHARGEABLE_KG harus > 0")
	}
	if p.CODFeeRateBps < 0 || p.CODFeeRateBps > 10000 {
		problems = append(problems, "PRICING_COD_FEE_RATE_BPS harus 0..10000 (basis points)")
	}
	if p.InsuranceRateBps < 0 || p.InsuranceRateBps > 10000 {
		problems = append(problems, "PRICING_INSURANCE_RATE_BPS harus 0..10000 (basis points)")
	}
	if p.MarkupRateBps < 0 || p.MarkupRateBps > 10000 {
		problems = append(problems, "PRICING_MARKUP_RATE_BPS harus 0..10000 (basis points)")
	}
	if p.CODFeeMin < 0 || p.InsuranceMinFee < 0 || p.MarkupFlat < 0 {
		problems = append(problems, "Nilai minimum/markup tidak boleh negatif")
	}
	if p.PriceRoundStep <= 0 {
		problems = append(problems, "PRICING_PRICE_ROUND_STEP harus > 0")
	}
	if len(problems) > 0 {
		return fmt.Errorf("pricing: %s", strings.Join(problems, "; "))
	}
	return nil
}

// totalBpsWeight adalah jumlah bobot scoring yang wajib 10000 (100%).
const totalBpsWeight = 10000

// ScoringConfig memuat parameter engine scoring & rekomendasi (TASK-025).
// Bobot kriteria configurable lewat env — mengubah bobot TIDAK boleh menuntut
// perubahan kode. Ketiga bobot harus berjumlah 10000 basis points.
type ScoringConfig struct {
	WeightPriceBps       int64 // bobot harga
	WeightSpeedBps       int64 // bobot kecepatan (ETA)
	WeightReliabilityBps int64 // bobot keandalan kurir

	// ReasonThresholdPct adalah beda minimum (persen) agar alasan ditulis.
	ReasonThresholdPct int
	// HighReliabilityScore adalah ambang reliability_score (0..100) untuk
	// menyebut opsi berperforma historis tinggi.
	HighReliabilityScore float64
	// ConfidenceBaseBps + ConfidenceSpanBps × margin skor = keyakinan (0..10000).
	ConfidenceBaseBps int64
	ConfidenceSpanBps int64
	// MaxReasons membatasi jumlah alasan rekomendasi.
	MaxReasons int
}

// validate menolak bobot scoring yang tidak masuk akal sebelum server menyala.
func (s ScoringConfig) validate() error {
	var problems []string
	if s.WeightPriceBps < 0 || s.WeightSpeedBps < 0 || s.WeightReliabilityBps < 0 {
		problems = append(problems, "SCORING_WEIGHT_* tidak boleh negatif")
	}
	if sum := s.WeightPriceBps + s.WeightSpeedBps + s.WeightReliabilityBps; sum != totalBpsWeight {
		problems = append(problems, fmt.Sprintf(
			"SCORING_WEIGHT_PRICE/SPEED/RELIABILITY_BPS harus berjumlah %d (saat ini %d)",
			totalBpsWeight, sum))
	}
	if s.HighReliabilityScore <= 0 || s.HighReliabilityScore > 100 {
		problems = append(problems, "SCORING_HIGH_RELIABILITY_SCORE harus 0..100")
	}
	if s.ReasonThresholdPct < 0 {
		problems = append(problems, "SCORING_REASON_THRESHOLD_PCT tidak boleh negatif")
	}
	if s.ConfidenceBaseBps < 0 || s.ConfidenceBaseBps > totalBpsWeight {
		problems = append(problems, "SCORING_CONFIDENCE_BASE_BPS harus 0..10000")
	}
	if s.ConfidenceSpanBps < 0 || s.ConfidenceSpanBps > totalBpsWeight {
		problems = append(problems, "SCORING_CONFIDENCE_SPAN_BPS harus 0..10000")
	}
	if s.ConfidenceBaseBps+s.ConfidenceSpanBps > totalBpsWeight {
		problems = append(problems, "SCORING_CONFIDENCE_BASE_BPS + SPAN_BPS tidak boleh melebihi 10000")
	}
	if s.MaxReasons <= 0 {
		problems = append(problems, "SCORING_MAX_REASONS harus > 0")
	}
	if len(problems) > 0 {
		return fmt.Errorf("scoring: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Nilai default khusus pengembangan. Validasi production membandingkan
// terhadap konstanta ini (bukan literal terpisah), sehingga default dan
// penolakannya tidak mungkin lepas sinkron.
const (
	defaultDevAccessSecret = "dev-only-change-me-32-chars-minimum!!"
	defaultDevSecretsKey   = "ZGV2LW9ubHktZW5jcnlwdGlvbi1rZXktMzJieXRlcyE="
	defaultDevOAuthState   = "dev-oauth-state-secret-change-me-32ch"
)

// IsProduction memengaruhi default keamanan (cookie, verbose error).
func (c *Config) IsProduction() bool { return c.Env == "production" }

// IsDevelopment untuk perilaku dev (log debug, mock vendor).
func (c *Config) IsDevelopment() bool { return c.Env == "development" || c.Env == "" }

// Load membaca environment dan memvalidasi hal-hal kritis.
func Load() (*Config, error) {
	c := &Config{
		Env:      env("APP_ENV", "development"),
		Name:     env("APP_NAME", "angkut"),
		Timezone: env("APP_TIMEZONE", "Asia/Jakarta"),
		LogLevel: env("LOG_LEVEL", "info"),
		API: APIConfig{
			Port:           env("API_PORT", "8080"),
			APIURL:         env("API_URL", "http://localhost:8080"),
			WebURL:         env("WEB_URL", "http://localhost:3000"),
			ReadTimeout:    15 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    60 * time.Second,
			ShutdownGrace:  20 * time.Second,
			AllowedOrigins: splitCSV(env("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		},
		DB: DBConfig{
			URL:      env("DATABASE_URL", ""),
			MaxConns: int32(envInt("DB_MAX_CONNS", 20)),
			MinConns: int32(envInt("DB_MIN_CONNS", 2)),
		},
		RDB: RedisConfig{URL: env("REDIS_URL", "redis://localhost:6379/0")},
		MQ: RabbitConfig{
			URL:     env("RABBITMQ_URL", ""),
			Enabled: envBool("RABBITMQ_ENABLED", false),
		},
		S3: S3Config{
			Endpoint:     env("S3_ENDPOINT", ""),
			Bucket:       env("S3_BUCKET", "angkut"),
			AccessKey:    env("S3_ACCESS_KEY", ""),
			SecretKey:    env("S3_SECRET_KEY", ""),
			Region:       env("S3_REGION", "idn"),
			UsePathStyle: envBool("S3_USE_PATH_STYLE", true),
		},
		Mail: MailConfig{
			SMTPURL: env("SMTP_URL", ""),
			From:    env("MAIL_FROM", "no-reply@angkut.local"),
			Enabled: envBool("MAIL_ENABLED", false),
		},
		Auth: AuthConfig{
			AccessSecret:       env("JWT_ACCESS_SECRET", defaultDevAccessSecret),
			AccessTTL:          envDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:         envDuration("REFRESH_TOKEN_TTL", 720*time.Hour),
			ArgonMemoryKiB:     uint32(envInt("ARGON2_MEMORY_KIB", 65536)),
			ArgonIter:          uint32(envInt("ARGON2_ITERATIONS", 3)),
			ArgonParallel:      uint8(envInt("ARGON2_PARALLELISM", 2)),
			SecretsKey:         env("SECRETS_ENCRYPTION_KEY", defaultDevSecretsKey),
			GoogleClientID:     env("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
			GoogleRedirectURL:  env("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
			OAuthStateSecret:   env("OAUTH_STATE_SECRET", defaultDevOAuthState),
		},
		Vendor: VendorConfig{
			CourierAdapter: env("COURIER_ADAPTER", "mock"),
			StoreAdapter:   env("STORE_ADAPTER", "mock"),
			PaymentAdapter: env("PAYMENT_ADAPTER", "mock"),
			AIProvider:     env("AI_PROVIDER", "mock"),
			WhatsAppAdap:   env("WHATSAPP_ADAPTER", "mock"),
		},
		Obs: ObservabilityConfig{
			OTELEnabled:  envBool("OTEL_ENABLED", false),
			OTELEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
			MetricsOn:    envBool("METRICS_ENABLED", true),
		},
		Rate: RateLimitConfig{
			AuthPerMin:        envInt("RATE_LIMIT_AUTH_PER_MIN", 10),
			APIPerMin:         envInt("RATE_LIMIT_API_PER_MIN", 120),
			PublicTrackPerMin: envInt("RATE_LIMIT_PUBLIC_TRACK_PER_MIN", 30),
			BulkPerMin:        envInt("RATE_LIMIT_BULK_PER_MIN", 5),
		},
		Bulk: BulkConfig{
			MaxRows:      envInt("BULK_MAX_ROWS", 2000),
			MaxWeightKG:  envFloat("BULK_MAX_WEIGHT_KG", 100),
			ChunkSize:    envInt("BULK_CHUNK_SIZE", 100),
			MaxUploadMiB: envInt("BULK_MAX_UPLOAD_MIB", 4),
		},
		Pricing: PricingConfig{
			VolumetricDivisorDefault: envInt("PRICING_VOLUMETRIC_DIVISOR", 6000),
			WeightRoundUpStepKG:      envFloat("PRICING_WEIGHT_ROUND_STEP_KG", 0.1),
			MaxChargeableWeightKG:    envFloat("PRICING_MAX_CHARGEABLE_KG", 100),
			CODFeeRateBps:            int64(envInt("PRICING_COD_FEE_RATE_BPS", 100)),
			CODFeeMin:                int64(envInt("PRICING_COD_FEE_MIN", 2500)),
			InsuranceRateBps:         int64(envInt("PRICING_INSURANCE_RATE_BPS", 20)),
			InsuranceMinFee:          int64(envInt("PRICING_INSURANCE_MIN_FEE", 2500)),
			MarkupFlat:               int64(envInt("PRICING_MARKUP_FLAT", 0)),
			MarkupRateBps:            int64(envInt("PRICING_MARKUP_RATE_BPS", 0)),
			PriceRoundStep:           int64(envInt("PRICING_PRICE_ROUND_STEP", 100)),
		},
		Scoring: ScoringConfig{
			// Default: keandalan sedikit lebih diutamakan daripada harga,
			// karena ongkir salah pilih lebih mahal daripada selisih kecil.
			WeightPriceBps:       int64(envInt("SCORING_WEIGHT_PRICE_BPS", 4000)),
			WeightSpeedBps:       int64(envInt("SCORING_WEIGHT_SPEED_BPS", 2500)),
			WeightReliabilityBps: int64(envInt("SCORING_WEIGHT_RELIABILITY_BPS", 3500)),

			ReasonThresholdPct:   envInt("SCORING_REASON_THRESHOLD_PCT", 5),
			HighReliabilityScore: envFloat("SCORING_HIGH_RELIABILITY_SCORE", 90),
			ConfidenceBaseBps:    int64(envInt("SCORING_CONFIDENCE_BASE_BPS", 6000)),
			ConfidenceSpanBps:    int64(envInt("SCORING_CONFIDENCE_SPAN_BPS", 3500)),
			MaxReasons:           envInt("SCORING_MAX_REASONS", 3),
		},
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate menolak kombinasi berbahaya SEBELUM server menyala.
func (c *Config) Validate() error {
	var problems []string

	if c.IsProduction() {
		if c.Auth.AccessSecret == "" || c.Auth.AccessSecret == defaultDevAccessSecret {
			problems = append(problems, "JWT_ACCESS_SECRET wajib diisi dengan nilai kuat di production")
		}
		if c.DB.URL == "" {
			problems = append(problems, "DATABASE_URL wajib diisi di production")
		}
		if strings.HasPrefix(c.API.APIURL, "http://") {
			problems = append(problems, "API_URL harus https di production")
		}
		if c.Auth.SecretsKey == defaultDevSecretsKey {
			problems = append(problems, "SECRETS_ENCRYPTION_KEY wajib diisi dengan kunci 32-byte base64 di production")
		}
		if c.Auth.OAuthStateSecret == defaultDevOAuthState {
			problems = append(problems, "OAUTH_STATE_SECRET wajib diubah di production")
		}
		if c.Auth.SecretsKey == c.Auth.AccessSecret {
			problems = append(problems, "SECRETS_ENCRYPTION_KEY harus berbeda dari JWT_ACCESS_SECRET (pemisahan kunci)")
		}
	}
	if c.DB.URL != "" && !strings.HasPrefix(c.DB.URL, "postgres") {
		problems = append(problems, "DATABASE_URL harus berupa DSN postgres")
	}
	if c.RDB.URL != "" && !strings.HasPrefix(c.RDB.URL, "redis") {
		problems = append(problems, "REDIS_URL harus berupa DSN redis")
	}
	if err := c.Pricing.validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if err := c.Scoring.validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if err := c.Bulk.validate(); err != nil {
		problems = append(problems, err.Error())
	}

	if len(problems) > 0 {
		return fmt.Errorf("konfigurasi tidak valid:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

// ─── helper env ──────────────────────────────────────────────────────────

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			return d
		}
	}
	return def
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
