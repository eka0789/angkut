package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/platform"
)

// CORS membatasi asal permintaan browser ke daftar putih. Preflight ditangani
// tanpa meneruskan ke router.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allow[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allow[strings.ToLower(origin)]; ok {
					h := w.Header()
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Vary", "Origin")
					h.Set("Access-Control-Allow-Credentials", "true")
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers",
						"Authorization, Content-Type, Idempotency-Key, X-Request-ID, X-Api-Key")
					h.Set("Access-Control-Max-Age", "600")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter membatasi laju request. Implementasi awal: token bucket in-memory
// per kunci (IP atau identitas). Redis-backed akan menggantikan ini saat
// multi-node; interface-nya sudah disiapkan lewat Limiter.
type Limiter interface {
	// Allow melaporkan apakah permintaan dengan kunci ini diizinkan,
	// beserta sisa kuota dan waktu tunggu bila ditolak.
	Allow(key string, now time.Time) (ok bool, remaining int, retryAfter time.Duration)
}

// RateLimit membungkus Limiter menjadi middleware. keyFn menentukan kunci
// pembatasan (mis. IP untuk endpoint publik, user/org untuk endpoint auth).
func RateLimit(limiter Limiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key == "" {
				key = clientIP(r)
			}
			ok, remaining, retryAfter := limiter.Allow(key, time.Now())
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !ok {
				secs := int(retryAfter.Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				response.Fail(w, shared.ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// KeyByIP membatasi per alamat IP (endpoint publik).
func KeyByIP(r *http.Request) string { return "ip:" + clientIP(r) }

// KeyByUserOrIP membatasi per pengguna terautentikasi, else per IP.
// Identitas diambil dari context yang diset middleware auth.
func KeyByUserOrIP(r *http.Request) string {
	if uid := userIDFromContext(r); uid != "" {
		return "user:" + uid
	}
	return "ip:" + clientIP(r)
}

// userIDFromContext membaca user_id dari context tanpa mengimpor paket auth
// (menghindari siklus impor). Kunci context sama dengan yang diset auth.
func userIDFromContext(r *http.Request) string { return ctxUserID(r) }

// ctxUserID disediakan di bridge.go untuk memisahkan detail context key.
var ctxUserID = func(r *http.Request) string { return "" }

// EnsureUUIDBaru dipakai test untuk memastikan generator UUID stabil.
func EnsureUUIDBaru() string { return platform.NewUUID() }