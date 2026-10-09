package migrate_test

import (
	"testing"
	"testing/fstest"

	"github.com/angkut/angkut/apps/internal/migrate"
	"github.com/angkut/angkut/apps/migrations"
)

// Migrasi nyata harus lengkap berpasangan dan bernomor urut.
func TestEmbeddedMigrationsAreWellFormed(t *testing.T) {
	// New membutuhkan pool hanya saat dijalankan; pemuatan berkas tidak.
	// Kita uji pemuatannya lewat filesystem tertanam dengan pool nil.
	_, err := migrate.New(nil, migrations.FS)
	if err != nil {
		t.Fatalf("migrasi tertanam tidak valid: %v", err)
	}
}

func TestMigrationsRejectMissingDown(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_x.up.sql": {Data: []byte("SELECT 1;")},
	}
	if _, err := migrate.New(nil, fsys); err == nil {
		t.Fatal("migrasi tanpa file down seharusnya ditolak")
	}
}

func TestMigrationsRejectNameMismatch(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_satu.up.sql":   {Data: []byte("SELECT 1;")},
		"0001_dua.down.sql":  {Data: []byte("SELECT 1;")},
		"0002_tiga.up.sql":   {Data: []byte("SELECT 1;")},
		"0002_tiga.down.sql": {Data: []byte("SELECT 1;")},
	}
	if _, err := migrate.New(nil, fsys); err == nil {
		t.Fatal("nama up/down yang tidak konsisten seharusnya ditolak")
	}
}

func TestMigrationsSortedAndComplete(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_b.up.sql":   {Data: []byte("SELECT 1;")},
		"0002_b.down.sql": {Data: []byte("SELECT 1;")},
		"0001_a.up.sql":   {Data: []byte("SELECT 1;")},
		"0001_a.down.sql": {Data: []byte("SELECT 1;")},
		"0010_c.up.sql":   {Data: []byte("SELECT 1;")},
		"0010_c.down.sql": {Data: []byte("SELECT 1;")},
	}
	if _, err := migrate.New(nil, fsys); err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
}
