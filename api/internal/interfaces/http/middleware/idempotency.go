package middleware

import (
	"net/http"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
)

// Idempotency memastikan mutasi kritis dengan Idempotency-Key yang sama tidak
// dieksekusi dua kali. Implementasi awal: store in-memory; versi Redis akan
// menggantikan tanpa mengubah handler (kontrak Store tetap).
//
// Handler memanggil Store.Begin sebelum efek samping, lalu Complete bila sukses.
type IdempotencyStore interface {
	// Begin mencatat bahwa key ini mulai diproses. Bila key sudah pernah
	// selesai, hasil sebelumnya dikembalikan (replayed=true) dan handler harus
	// mengirim respons itu tanpa mengeksekusi ulang.
	Begin(key string, now time.Time) (replayed bool, status int, body []byte)
	// Complete menyimpan hasil akhir agar pemanggilan ulang memutar hasil sama.
	Complete(key string, status int, body []byte)
}

// RequireIdempotency menolak mutasi tanpa header Idempotency-Key sehingga
// integritas tidak bergantung pada disiplin klien.
func RequireIdempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Idempotency-Key") == "" {
			response.Fail(w, shared.Validation(map[string]string{
				"Idempotency-Key": "header wajib untuk operasi ini.",
			}))
			return
		}
		next.ServeHTTP(w, r)
	})
}