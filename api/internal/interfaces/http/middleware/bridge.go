package middleware

import "net/http"

// ctxUserID disambungkan di sini supaya paket middleware tidak mengimpor paket
// auth (menghindari siklus impor). Paket auth memanggil RegisterUserIDResolver
// saat init untuk mengaktifkan pembatasan rate limit per pengguna.
func init() {
	ctxUserID = func(r *http.Request) string { return userIDResolver(r) }
}

// userIDResolver dapat diganti oleh paket auth; default mengembalikan "".
var userIDResolver = func(r *http.Request) string { return "" }

// RegisterUserIDResolver dipanggil paket auth (Phase 2) agar rate limiter bisa
// membatasi per-pengguna, bukan hanya per-IP.
func RegisterUserIDResolver(fn func(r *http.Request) string) {
	if fn != nil {
		userIDResolver = fn
	}
}
