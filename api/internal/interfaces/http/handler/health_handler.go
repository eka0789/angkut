// Package handler berisi handler HTTP tipis: decode → service → encode.
// TIDAK ada logika bisnis di sini.
package handler

import (
	"net/http"
	"time"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
)

// Health melaporkan proses hidup (liveness). Tidak menyentuh dependensi agar
// sinyal liveness tidak ikut mati saat DB bermasalah.
func Health(w http.ResponseWriter, r *http.Request) {
	response.OK(w, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// Checker melaporkan status satu dependensi.
type Checker interface {
	Name() string
	Check(r *http.Request) error
}

// Ready melaporkan kesiapan menerima trafik (readiness). Bila ada dependensi
// wajib yang gagal, kembalikan 503 sehingga load balancer berhenti mengirim.
func Ready(checkers []Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := make(map[string]string, len(checkers))
		ready := true
		for _, c := range checkers {
			err := c.Check(r)
			if err != nil {
				results[c.Name()] = "down"
				ready = false
			} else {
				results[c.Name()] = "up"
			}
		}
		if !ready {
			response.JSON(w, http.StatusServiceUnavailable, response.Envelope{
				Success: false,
				Error:   &response.ErrorBody{Code: "NOT_READY", Message: "Dependensi belum siap."},
				Meta:    results,
			})
			return
		}
		response.JSON(w, http.StatusOK, response.Envelope{Success: true, Meta: results})
	}
}

// NotFound dipasang sebagai handler fallback router.
func NotFound(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusNotFound, response.Envelope{
		Success: false,
		Error:   &response.ErrorBody{Code: "ROUTE_NOT_FOUND", Message: "Endpoint tidak ditemukan."},
	})
}

// MethodNotAllowed dipasang sebagai handler fallback metode.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusMethodNotAllowed, response.Envelope{
		Success: false,
		Error:   &response.ErrorBody{Code: "METHOD_NOT_ALLOWED", Message: "Metode HTTP tidak didukung untuk endpoint ini."},
	})
}