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
}

func TestProductionRequiresSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_URL", "https://api.angkut.id")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/angkut")
	t.Setenv("JWT_ACCESS_SECRET", "dev-only-change-me")

	if _, err := Load(); err == nil {
		t.Fatal("production harus menolak JWT_ACCESS_SECRET default")
	}

	t.Setenv("JWT_ACCESS_SECRET", strings.Repeat("x", 40))
	if _, err := Load(); err != nil {
		t.Fatalf("production dengan secret kuat seharusnya lolos: %v", err)
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