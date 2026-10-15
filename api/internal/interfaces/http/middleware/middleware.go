// Package middleware berisi rantai middleware HTTP. Urutannya DIKUNCI —
// lihat dev-docs/architecture/api-flow.md. Mengubah urutan = mengubah perilaku
// keamanan, jadi setiap perubahan harus disengaja.
package middleware

import (
	"net/http"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/observability"
	"github.com/angkut/angkut/apps/internal/platform"
)

// errInternalPanic dipakai Recover; memakai sentinel agar pemetaannya konsisten
// dan pesan panic tidak pernah bocor ke klien.
var errInternalPanic = shared.ErrInternal

// RequestID menetapkan request_id (dari header bila ada, else UUID baru) ke
// context dan header respons.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = platform.NewUUID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := observability.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recover menangkap panic agar satu request tidak menjatuhkan proses.
// Panic dicatat sebagai error; klien menerima envelope 500 yang rapi.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log := observability.WithContext(r.Context(), observability.From(r.Context()))
				log.Error("panic tertangkap", "panic", rec, "path", r.URL.Path, "method", r.Method)
				response.Fail(w, errInternalPanic)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders menambahkan header keamanan dasar (defense-in-depth;
// TLS & WAF ditangani Nginx/Cloudflare di depan).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=()")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		if r.URL.Path != "/metrics" {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}
