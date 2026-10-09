// Package migrations menyediakan berkas SQL migrasi yang tertanam (embedded)
// ke dalam binary, sehingga runner tidak bergantung pada path relatif atau
// CLI eksternal saat dijalankan di server.
package migrations

import "embed"

// FS berisi seluruh pasangan migrasi up/down.
//
//go:embed *.sql
var FS embed.FS