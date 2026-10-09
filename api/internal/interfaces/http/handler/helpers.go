package handler

import (
	"net"
	"net/http"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
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
