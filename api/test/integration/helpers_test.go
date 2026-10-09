package integration

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// lookupEnv dipisah supaya helpers_test tidak mengimpor os di banyak tempat.
func lookupEnv(key string) string { return os.Getenv(key) }

// randRead dipisah agar file tes lain tetap ringkas dan mudah dibaca.
func randRead(b []byte) (int, error) { return rand.Read(b) }

// hexEncode mengubah byte menjadi string hex (untuk sufiks unik).
func hexEncode(b []byte) string { return hex.EncodeToString(b) }

// skipWithoutDB adalah helper eksplisit untuk tes yang memerlukan database.
func skipWithoutDB(t *testing.T) {
	t.Helper()
	if dsn := lookupEnv("DATABASE_URL"); dsn == "" {
		t.Skip("DATABASE_URL tidak diset — tes integrasi dilewati")
	}
}
