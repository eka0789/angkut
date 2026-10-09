// Package validate membungkus validator agar pesan error seragam bahasa Indonesia.
package validate

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	once sync.Once
	v    *validator.Validate
)

// V mengembalikan instance validator bersama (thread-safe).
func V() *validator.Validate {
	once.Do(func() {
		v = validator.New(validator.WithRequiredStructEnabled())
		// Nama field mengikuti tag `json`, bukan nama Go — penting untuk FE.
		v.RegisterTagNameFunc(func(fld reflectField) string {
			name := fld.Tag.Get("json")
			if name == "" || name == "-" {
				return fld.Name
			}
			if i := strings.IndexByte(name, ','); i >= 0 {
				name = name[:i]
			}
			return name
		})
	})
	return v
}

// Fields memvalidasi struct dan mengembalikan map[field]pesan dalam bahasa
// Indonesia — siap dipakai sebagai `errors` pada envelope respons.
func Fields(s any) map[string]string {
	err := V().Struct(s)
	if err == nil {
		return nil
	}
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return map[string]string{"_": err.Error()}
	}
	out := make(map[string]string, len(ve))
	for _, fe := range ve {
		out[fe.Field()] = message(fe)
	}
	return out
}

func message(fe validator.FieldError) string {
	param := fe.Param()
	switch fe.Tag() {
	case "required":
		return "wajib diisi."
	case "email":
		return "format email tidak valid."
	case "min":
		return fmt.Sprintf("minimal %s.", param)
	case "max":
		return fmt.Sprintf("maksimal %s.", param)
	case "oneof":
		return fmt.Sprintf("harus salah satu dari: %s.", strings.ReplaceAll(param, " ", ", "))
	case "uuid":
		return "format ID tidak valid."
	case "numeric":
		return "harus berupa angka."
	case "gte":
		return fmt.Sprintf("harus ≥ %s.", param)
	case "lte":
		return fmt.Sprintf("harus ≤ %s.", param)
	case "url":
		return "format URL tidak valid."
	case "e164":
		return "format nomor telepon tidak valid (contoh: +628123456789)."
	default:
		return "nilai tidak valid."
	}
}
