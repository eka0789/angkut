// Package shared berisi tipe dan aturan lintas-domain (murni, tanpa I/O).
package shared

import "errors"

// Sentinel error lintas modul. Handler memetakannya ke kode HTTP di
// interfaces/http/response/error_map.go. Jangan pernah menulis pesan
// error mentah ke response — selalu lewat pemetaan ini.
var (
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource conflict")
	ErrValidation   = errors.New("validation failed")
	ErrUnauthorized = errors.New("unauthenticated")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("rate limited")
	ErrInternal     = errors.New("internal error")
)

// Error adalah typed error yang membawa kode stabil, pesan aman untuk user,
// dan detail per-field untuk validasi. Pesan asli (internal) hanya dilog.
type Error struct {
	Code    string            // kode stabil, mis. "SHIPMENT_NOT_FOUND"
	Message string            // pesan aman untuk user
	Fields  map[string]string // detail validasi per field (opsional)
	wrap    error             // penyebab internal — TIDAK dikirim ke user
}

func (e *Error) Error() string {
	if e.wrap != nil {
		return e.Code + ": " + e.Message + ": " + e.wrap.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.wrap }

// New membuat typed error.
func New(code, message string, wrap error) *Error {
	return &Error{Code: code, Message: message, wrap: wrap}
}

// Validation membuat error validasi dengan detail field.
func Validation(fields map[string]string) *Error {
	return &Error{
		Code:    "VALIDATION_ERROR",
		Message: "Data yang dikirim tidak valid.",
		Fields:  fields,
	}
}

// NotFound membuat error 404 dengan kode spesifik modul.
func NotFound(code, message string) *Error {
	return &Error{Code: code, Message: message, wrap: ErrNotFound}
}

// Conflict membuat error 409.
func Conflict(code, message string) *Error {
	return &Error{Code: code, Message: message, wrap: ErrConflict}
}

// Is melaporkan apakah err (atau rantai penyebabnya) adalah sentinel target.
func Is(err error, target error) bool { return errors.Is(err, target) }

// As mengisi target dengan *Error pertama dalam rantai err. Mengembalikan
// false bila err bukan (atau tidak membungkus) *Error — pemanggil yang butuh
// detail field validasi memakai ini alih-alih type assertion langsung.
func As(err error, target **Error) bool { return errors.As(err, target) }
