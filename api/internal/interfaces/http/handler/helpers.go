package handler

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/observability"
)

// requestID mengambil request_id dari context (diset middleware RequestID)
// agar tercatat di audit dan log tanpa handler perlu tahu detailnya.
func requestID(r *http.Request) string {
	return observability.RequestID(r.Context())
}

// clientIP mengembalikan alamat IP klien TANPA port, siap disimpan ke kolom
// `inet`. Nilai dari header X-Forwarded-For hanya dipakai sebagai metadata
// audit — TIDAK pernah untuk keputusan otorisasi atau keamanan.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Rantai proxy: entri pertama adalah klien asli.
		first := xff
		if i := strings.IndexByte(xff, ','); i > 0 {
			first = xff[:i]
		}
		if ip := normalizeIP(first); ip != "" {
			return ip
		}
	}
	return normalizeIP(r.RemoteAddr)
}

// normalizeIP membuang port dan memvalidasi bentuk alamat. Mengembalikan ""
// bila tidak dapat dipastikan — kolom audit menjadi NULL, yang jauh lebih baik
// daripada menyimpan nilai yang salah.
func normalizeIP(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	addr = strings.Trim(addr, "[]") // sebagian proxy mengirim "[::1]" tanpa port
	if net.ParseIP(addr) == nil {
		return ""
	}
	return addr
}

// unauthorized mengembalikan error standar untuk request tanpa identitas.
func unauthorized() error { return shared.ErrUnauthorized }

// ValidationError membuat typed validation error dengan detail field.
func ValidationError(fields map[string]string) error { return shared.Validation(fields) }

// nowUTC mengembalikan waktu sekarang dalam UTC. Dipisah sebagai fungsi agar
// mudah di-stub dan agar konsistensi zona waktu terlihat di satu tempat.
func nowUTC() time.Time { return time.Now().UTC() }

// parseOptionalTime mengurai waktu RFC3339 opsional. Nilai kosong atau tidak
// valid menghasilkan zero time — pemanggil (domain) yang memutuskan defaultnya,
// sehingga tidak ada waktu "diam-diam" dipakai di lapisan HTTP.
func parseOptionalTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
