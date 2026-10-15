package response

import (
	"errors"
	"net/http"
	"testing"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

func TestMapErrorStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", shared.NotFound("SHIPMENT_NOT_FOUND", "Kiriman tidak ada."), http.StatusNotFound, "SHIPMENT_NOT_FOUND"},
		{"conflict", shared.Conflict("AWB_TAKEN", "AWB sudah dipakai."), http.StatusConflict, "AWB_TAKEN"},
		{"validation fields", shared.Validation(map[string]string{"weight": "wajib diisi."}), http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"sentinel notfound", shared.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"sentinel forbidden", shared.ErrForbidden, http.StatusForbidden, "FORBIDDEN"},
		{"rate limited", shared.ErrRateLimited, http.StatusTooManyRequests, "RATE_LIMITED"},
		{"unknown", errors.New("db down"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := MapError(tc.err)
			if status != tc.status {
				t.Errorf("status = %d, harap %d", status, tc.status)
			}
			if body.Code != tc.code {
				t.Errorf("code = %q, harap %q", body.Code, tc.code)
			}
			// Pesan internal tidak boleh bocor.
			if tc.name == "unknown" && body.Message == "db down" {
				t.Error("pesan internal bocor ke klien")
			}
		})
	}
}

func TestValidationCarriesFields(t *testing.T) {
	_, body := MapError(shared.Validation(map[string]string{"origin_city_id": "wajib diisi."}))
	if body.Fields["origin_city_id"] == "" {
		t.Error("detail field validasi hilang")
	}
}
