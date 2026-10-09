package observability

import (
	"net/http"
	"time"
)

// MetricsMiddleware mencatat jumlah request per metode+status ke registry.
// (Versi ringkas; saat OTel aktif, span menggantikan sebagian peran ini.)
func MetricsMiddleware(reg *Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRec{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			reg.Inc("angkut_http_requests_total")
			reg.Inc("angkut_http_requests_" + r.Method + "_" + itoa(status) + "_total")
			reg.Set("angkut_http_last_request_duration_ms", float64(time.Since(start).Milliseconds()))
		})
	}
}

type statusRec struct {
	http.ResponseWriter
	status int
}

func (r *statusRec) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRec) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
