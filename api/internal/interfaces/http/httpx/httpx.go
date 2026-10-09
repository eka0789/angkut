// Package httpx berisi helper kecil untuk handler tipis (decode, param, dsb).
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/platform/validate"
)

// maxBody membatasi ukuran body agar tidak ada request raksasa.
const maxBody = 4 << 20 // 4 MiB

// Decode membaca JSON body, menolak field tak dikenal, lalu memvalidasi tag.
// Mengembalikan false bila sudah menulis respons error.
func Decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var dst T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&dst); err != nil {
		if errors.Is(err, io.EOF) {
			response.Fail(w, shared.Validation(map[string]string{"body": "body kosong."}))
			return dst, false
		}
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syn), errors.As(err, &typ):
			response.Fail(w, shared.Validation(map[string]string{"body": "format JSON tidak valid."}))
		default:
			response.Fail(w, shared.Validation(map[string]string{"body": "body tidak dapat dibaca."}))
		}
		return dst, false
	}
	if fields := validate.Fields(dst); fields != nil {
		response.Fail(w, shared.Validation(fields))
		return dst, false
	}
	return dst, true
}

// QueryInt membaca query integer dengan default dan batas.
func QueryInt(r *http.Request, key string, def, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

// QueryBool membaca flag boolean query.
func QueryBool(r *http.Request, key string, def bool) bool {
	raw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	switch raw {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return def
	}
}

// QueryStr mengembalikan nilai query yang sudah di-trim.
func QueryStr(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

// PathParam mengambil path parameter router (Chi) secara aman.
func PathParam(r *http.Request, key string) string { return chiURLParam(r, key) }

// Pagination membaca parameter paginasi standar dari query.
func Pagination(r *http.Request) shared.Pagination {
	p := shared.Pagination{
		Limit:  QueryInt(r, "limit", shared.DefaultLimit, 1, shared.MaxLimit),
		Cursor: QueryStr(r, "cursor"),
		Page:   QueryInt(r, "page", 0, 0, 0),
	}
	p.Normalize()
	return p
}

// IdempotencyKey mengambil header Idempotency-Key (wajib untuk mutasi kritis).
func IdempotencyKey(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}