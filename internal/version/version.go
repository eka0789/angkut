// Versi aplikasi. Nilai disuntik saat build:
//
//	go build -ldflags "-X 'github.com/angkut/angkut/apps/internal/version.Version=1.0.0'"
package version

// Nilai default saat dijalankan dari source.
var (
	// Version adalah versi semantik aplikasi (lihat dev-docs/ai/VERSION.md).
	Version = "0.0.0-dev"
	// Commit adalah hash git saat build.
	Commit = "none"
	// BuildTime adalah waktu build dalam RFC3339.
	BuildTime = "unknown"
)

// String merangkum versi untuk log startup.
func String() string {
	return Version + " (" + Commit + ", " + BuildTime + ")"
}