package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// chiURLParam dipisah agar paket httpx tetap ringkas; Chi aman dipanggil
// walau route tidak punya param (mengembalikan string kosong).
func chiURLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
