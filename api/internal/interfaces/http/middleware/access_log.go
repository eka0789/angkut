package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/observability"
)

// statusRecorder merekam status & ukuran respons untuk logging akses.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap menyingkap pembungkus ini ke ResponseWriter asli.
//
// Pembungkus hanya menyalin metode `ResponseWriter` (Write/WriteHeader), jadi
// `http.Flusher` dan `http.Hijacker` HILANG dari tipe di tangan handler —
// streaming SSE dan WebSocket akan ditolak "does not implement". Ekspresi
// Unwrap ini membuat `http.ResponseController` dan pemakai yang berjalan lewat
// rantai unwrap (mis. websocket.Accept) tetap menemukan kemampuan tersebut.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Logger mencatat satu baris akses per request (JSON, dengan korelasi context).
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		log := observability.WithContext(r.Context(), observability.From(r.Context()))
		log.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", clientIP(r),
		)
	})
}

// clientIP mengambil IP klien dengan menghormati X-Forwarded-For dari proxy
// tepercaya (Nginx). Nilai ini TIDAK dipakai untuk otorisasi.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		return host[:i]
	}
	return host
}
