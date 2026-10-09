package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDevDefaults(t *testing.T) {
	// Lingkungan test tidak boleh bergantung pada .env nyata.
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://angkut:angkut@localhost:5432/angkut?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() gagal di dev: %v", err)
	}
	if c.API.Port != "8080" {
		t.Errorf("port default = %q, harap 8080", c.API.Port)
	}
	if c.Auth.AccessTTL != 15*time.Minute {
		t.Errorf("access TTL default = %v, harap 15m", c.Auth.AccessTTL)
	}
	// Vendor default harus mock agar platform jalan tanpa kredensial.
	if c.Vendor.CourierAdapter != "mock" || c.Vendor.AIProvider != "mock" {
		t.Errorf("vendor default harus mock, dapat %+v", c.Vendor)
	}
	// Google OAuth kosong = fitur mati dengan pesan jujur, bukan gagal start.
	if c.Auth.GoogleClientID != "" {
		t.Error("Google client ID harus kosong secara default")
	}
}

// validKey adalah kunci 32-byte base64 yang sah (bukan nilai dev).
const validKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

// productionConfig menyiapkan lingkungan production yang VALID, sehingga tiap
// tes cukup mengubah satu variabel dan memastikan validasi menangkapnya.
func productionConfig(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_URL", "https://api.angkut.id")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/angkut?sslmode=disable")
	t.Setenv("JWT_ACCESS_SECRET", strings.Repeat("j", 40))
	t.Setenv("SECRETS_ENCRYPTION_KEY", validKey)
	t.Setenv("OAUTH_STATE_SECRET", strings.Repeat("o", 40))
}

func TestProductionAcceptsProperlyConfiguredEnvironment(t *testing.T) {
	productionConfig(t)
	if _, err := Load(); err != nil {
		t.Fatalf("production yang lengkap seharusnya lolos: %v", err)
	}
}

func TestProductionRejectsDefaultJWTSecret(t *testing.T) {
	productionConfig(t)
	t.Setenv("JWT_ACCESS_SECRET", "dev-only-change-me-32-chars-minimum!!")
	if _, err := Load(); err == nil {
		t.Fatal("production harus menolak JWT_ACCESS_SECRET default dev")
	}
}

func TestProductionRejectsDevEncryptionKey(t *testing.T) {
	productionConfig(t)
	t.Setenv("SECRETS_ENCRYPTION_KEY", defaultDevSecretsKey)
	if _, err := Load(); err == nil {
		t.Fatal("production harus menolak kunci enkripsi default dev")
	}
}

func TestProductionRejectsDevOAuthStateSecret(t *testing.T) {
	productionConfig(t)
	t.Setenv("OAUTH_STATE_SECRET", "dev-oauth-state-secret-change-me-32ch")
	if _, err := Load(); err == nil {
		t.Fatal("production harus menolak OAUTH_STATE_SECRET default dev")
	}
}

// Pemisahan kunci: kunci enkripsi rahasia dan kunci penandatanganan token tidak
// boleh sama — kebocoran salah satunya tidak boleh melemahkan yang lain.
func TestProductionRequiresSeparateKeys(t *testing.T) {
	productionConfig(t)
	shared := strings.Repeat("x", 40)
	t.Setenv("JWT_ACCESS_SECRET", shared)
	t.Setenv("SECRETS_ENCRYPTION_KEY", shared)
	if _, err := Load(); err == nil {
		t.Fatal("kunci enkripsi harus berbeda dari JWT_ACCESS_SECRET")
	}
}

func TestProductionRequiresHTTPS(t *testing.T) {
	productionConfig(t)
	t.Setenv("API_URL", "http://api.angkut.id")
	if _, err := Load(); err == nil {
		t.Fatal("API_URL http harus ditolak di production")
	}
}

func TestProductionRequiresDatabaseURL(t *testing.T) {
	productionConfig(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("DATABASE_URL kosong harus ditolak di production")
	}
}

func TestInvalidDSNRejected(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "mysql://nope")
	if _, err := Load(); err == nil {
		t.Fatal("DSN non-postgres harus ditolak")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("X_INT", "42")
	t.Setenv("X_BOOL", "yes")
	t.Setenv("X_DUR", "90s")

	if got := envInt("X_INT", 1); got != 42 {
		t.Errorf("envInt = %d", got)
	}
	if !envBool("X_BOOL", false) {
		t.Error("envBool gagal membaca 'yes'")
	}
	if got := envDuration("X_DUR", time.Second); got != 90*time.Second {
		t.Errorf("envDuration = %v", got)
	}
	if got := env("X_MISSING", "fallback"); got != "fallback" {
		t.Errorf("env fallback = %q", got)
	}
}
