package organizationapp

import (
	"time"

	"github.com/angkut/angkut/apps/internal/platform"
)

// Helper waktu & id dipisah agar mudah diganti di test dan konsisten dengan
// konvensi proyek (UUID v7 di application layer, waktu UTC).

func nowUTC() time.Time { return time.Now().UTC() }

func newID() string { return platform.NewUUID() }
