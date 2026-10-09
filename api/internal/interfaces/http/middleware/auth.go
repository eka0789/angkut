// Package middleware — autentikasi, tenant, dan RBAC.
//
// PRINSIP YANG TIDAK DINEGOSIASIKAN (ADR-008):
// organization_id HANYA berasal dari token yang sudah diverifikasi. Tidak ada
// jalur di mana nilai itu diambil dari body, query, atau header yang dikirim
// klien. Kalau prinsip ini bocor, isolasi tenant bocor.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/domain/organization"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/observability"
)

// Authenticator adalah fungsi yang dipakai middleware untuk memvalidasi token.
// Dipisah sebagai fungsi agar paket ini tidak mengimpor application (menghindari
// impor melingkar) sekaligus mudah diganti di test.
type Authenticator func(ctx context.Context, rawToken string) (*Identity, error)

// Identity adalah pemanggil yang sudah terverifikasi.
type Identity struct {
	UserID         string
	SessionID      string
	Email          string
	Name           string
	OrganizationID string
	RoleCode       organization.RoleCode
}

// ctxKey menjaga kunci context tetap privat ke paket ini.
type ctxKey struct{ name string }

var identityKey = ctxKey{"auth_identity"}

// Auth menolak request tanpa access token yang sah.
//
// Token dibaca dari header Authorization: Bearer <token>. Sesi diperiksa ke
// penyimpanan sehingga logout berlaku segera, bukan menunggu token kedaluwarsa.
func Auth(authenticate Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r)
			if raw == "" {
				response.Fail(w, shared.ErrUnauthorized)
				return
			}
			identity, err := authenticate(r.Context(), raw)
			if err != nil {
				response.Fail(w, err)
				return
			}
			ctx := withIdentity(r.Context(), identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth mencoba membaca identitas bila ada, tetapi tidak menolak request
// anonim. Dipakai endpoint publik yang menampilkan lebih banyak untuk pengguna
// yang sudah login (mis. tracking publik milik sendiri).
func OptionalAuth(authenticate Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if raw := bearerToken(r); raw != "" {
				if identity, err := authenticate(r.Context(), raw); err == nil {
					r = r.WithContext(withIdentity(r.Context(), identity))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Tenant menetapkan organisasi aktif ke context.
//
// Isi token dibandingkan dengan header `X-Organization-Id` bila klien mengirimnya:
// perbedaan berarti klien meminta organisasi yang tidak sesuai dengan token-nya,
// dan itu ditolak — bukan diam-diam dipakai.
func Tenant() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := IdentityFrom(r.Context())
			if identity == nil {
				response.Fail(w, shared.ErrUnauthorized)
				return
			}

			requested := strings.TrimSpace(r.Header.Get("X-Organization-Id"))
			if requested != "" && requested != identity.OrganizationID {
				response.Fail(w, shared.New(
					"ORGANIZATION_MISMATCH",
					"Organisasi yang diminta tidak sesuai dengan sesi Anda.",
					shared.ErrForbidden,
				))
				return
			}

			if identity.OrganizationID == "" {
				// Pengguna belum punya organisasi (mis. baru mendaftar lalu
				// organisasinya dihapus). Endpoint yang butuh tenant harus gagal jelas.
				response.Fail(w, shared.New(
					"NO_ACTIVE_ORGANIZATION",
					"Anda belum memiliki organisasi aktif. Buat atau pilih organisasi terlebih dahulu.",
					shared.ErrForbidden,
				))
				return
			}

			ctx := observability.WithOrganizationID(r.Context(), identity.OrganizationID)
			ctx = observability.WithUserID(ctx, identity.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission menolak aksi bila role pemanggil tidak memiliki izin.
// Pemeriksaan dilakukan di server — menyembunyikan tombol di UI bukan otorisasi.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := IdentityFrom(r.Context())
			if identity == nil {
				response.Fail(w, shared.ErrUnauthorized)
				return
			}
			if !identity.RoleCode.Can(permission) {
				response.Fail(w, shared.New(
					"FORBIDDEN",
					"Anda tidak berwenang untuk tindakan ini.",
					shared.ErrForbidden,
				))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole menolak aksi bila role pemanggil di bawah salah satu role
// yang diizinkan (berdasarkan hierarki level).
func RequireRole(allowed ...organization.RoleCode) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := IdentityFrom(r.Context())
			if identity == nil {
				response.Fail(w, shared.ErrUnauthorized)
				return
			}
			for _, role := range allowed {
				if identity.RoleCode == role || identity.RoleCode.CanManage(role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.Fail(w, shared.New(
				"FORBIDDEN",
				"Anda tidak berwenang untuk tindakan ini.",
				shared.ErrForbidden,
			))
		})
	}
}

// ─── Context plumbing ─────────────────────────────────────────────────────

func withIdentity(ctx context.Context, id *Identity) context.Context {
	ctx = context.WithValue(ctx, identityKey, id)
	ctx = observability.WithUserID(ctx, id.UserID)
	return ctx
}

// IdentityFrom mengambil identitas pemanggil dari context. Nilai nil berarti
// request belum melewati middleware Auth.
func IdentityFrom(ctx context.Context) *Identity {
	if ctx == nil {
		return nil
	}
	if id, ok := ctx.Value(identityKey).(*Identity); ok {
		return id
	}
	return nil
}

// OrgIDFromContext mengembalikan organisasi aktif — satu-satunya sumber yang
// boleh dipakai repository untuk memfilter data tenant.
func OrgIDFromContext(ctx context.Context) string {
	if id := IdentityFrom(ctx); id != nil {
		return id.OrganizationID
	}
	return ""
}

// UserIDFromContext mengembalikan id pengguna terautentikasi.
func UserIDFromContext(ctx context.Context) string {
	if id := IdentityFrom(ctx); id != nil {
		return id.UserID
	}
	return ""
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
