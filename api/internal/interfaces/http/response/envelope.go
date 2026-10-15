// Package response menegakkan envelope respons yang sama untuk SEMUA endpoint.
// Kontrak: { success, data | error, meta }. Lihat planning/api-contract.md.
package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Envelope adalah bentuk respons tunggal.
type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
	Meta    any        `json:"meta,omitempty"`
	Message string     `json:"message,omitempty"`
}

// ErrorBody membawa kode stabil + pesan aman; tidak pernah stack trace.
type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// JSON menulis envelope dengan status HTTP tertentu.
func JSON(w http.ResponseWriter, status int, body Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("gagal menulis respons JSON", "error", err)
	}
}

// OK menulis 200 dengan data.
func OK(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data})
}

// Created menulis 201 dengan data.
func Created(w http.ResponseWriter, data any) {
	JSON(w, http.StatusCreated, Envelope{Success: true, Data: data})
}

// Accepted menulis 202 (pekerjaan async, mis. bulk import).
func Accepted(w http.ResponseWriter, data any) {
	JSON(w, http.StatusAccepted, Envelope{Success: true, Data: data})
}

// Message menulis 200 dengan pesan saja (mis. mark-as-read).
func Message(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, Envelope{Success: true, Message: msg})
}

// NoContent menulis 204 tanpa body.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// List menulis 200 dengan data array + meta paginasi.
func List(w http.ResponseWriter, data any, meta any) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data, Meta: meta})
}

// Fail memetakan typed error domain ke status HTTP + kode stabil.
// Error internal TIDAK pernah bocor ke klien.
func Fail(w http.ResponseWriter, err error) {
	status, body := MapError(err)
	JSON(w, status, Envelope{Success: false, Error: body})
}

// MapError adalah satu-satunya tempat pemetaan error → HTTP.
func MapError(err error) (int, *ErrorBody) {
	var de *shared.Error
	if errors.As(err, &de) {
		if len(de.Fields) > 0 {
			return http.StatusUnprocessableEntity, &ErrorBody{
				Code: de.Code, Message: de.Message, Fields: de.Fields,
			}
		}
		return statusForWrap(de), &ErrorBody{Code: de.Code, Message: de.Message}
	}

	switch {
	case errors.Is(err, shared.ErrNotFound):
		return http.StatusNotFound, &ErrorBody{Code: "NOT_FOUND", Message: "Data tidak ditemukan."}
	case errors.Is(err, shared.ErrConflict):
		return http.StatusConflict, &ErrorBody{Code: "CONFLICT", Message: "Terjadi konflik data."}
	case errors.Is(err, shared.ErrValidation):
		return http.StatusUnprocessableEntity, &ErrorBody{Code: "VALIDATION_ERROR", Message: "Data tidak valid."}
	case errors.Is(err, shared.ErrUnauthorized):
		return http.StatusUnauthorized, &ErrorBody{Code: "UNAUTHENTICATED", Message: "Sesi tidak valid atau sudah berakhir."}
	case errors.Is(err, shared.ErrForbidden):
		return http.StatusForbidden, &ErrorBody{Code: "FORBIDDEN", Message: "Anda tidak berwenang untuk tindakan ini."}
	case errors.Is(err, shared.ErrRateLimited):
		return http.StatusTooManyRequests, &ErrorBody{Code: "RATE_LIMITED", Message: "Terlalu banyak permintaan, coba lagi nanti."}
	default:
		// Jangan bocorkan detail internal. Pesan asli hanya masuk log.
		slog.Error("error tak tertangani", "error", err)
		return http.StatusInternalServerError, &ErrorBody{Code: "INTERNAL_ERROR", Message: "Terjadi kesalahan pada server."}
	}
}

func statusForWrap(de *shared.Error) int {
	switch {
	case errors.Is(de, shared.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(de, shared.ErrConflict):
		return http.StatusConflict
	case errors.Is(de, shared.ErrValidation):
		return http.StatusUnprocessableEntity
	case errors.Is(de, shared.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(de, shared.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(de, shared.ErrRateLimited):
		return http.StatusTooManyRequests
	default:
		return http.StatusBadRequest
	}
}
