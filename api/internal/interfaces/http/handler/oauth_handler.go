package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/auth"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// oauthStateCookie adalah nama cookie penyimpan state anti-CSRF.
const oauthStateCookie = "angkut_oauth_state"

// oauthStatePath membatasi cookie hanya terkirim ke jalur OAuth.
const oauthStatePath = "/api/v1/auth/google"

// GoogleOAuthHandler menangani consent & callback Google.
//
// Keamanan alur ini bergantung pada parameter `state` yang ditandatangani server
// (HMAC) dan sekaligus disimpan di cookie httpOnly. Penyerang tidak dapat
// memalsukan callback karena ia tidak bisa menghasilkan state yang cocok dengan
// cookie milik korban.
type GoogleOAuthHandler struct {
	svc         *auth.Service
	oauth       auth.GoogleOAuth
	stateSecret string
}

// NewGoogleOAuthHandler membuat handler OAuth.
func NewGoogleOAuthHandler(svc *auth.Service, oauth auth.GoogleOAuth, stateSecret string) *GoogleOAuthHandler {
	return &GoogleOAuthHandler{svc: svc, oauth: oauth, stateSecret: stateSecret}
}

// errOAuthDisabled dikembalikan bila kredensial Google belum diisi. Lebih baik
// fitur mati dengan pesan jelas daripada menerima login yang tidak terverifikasi.
var errOAuthDisabled = shared.New(
	"OAUTH_DISABLED",
	"Masuk dengan Google belum tersedia. Gunakan email dan kata sandi.",
	shared.ErrValidation,
)

// errOAuthState menandai state yang tidak valid/kedaluwarsa.
var errOAuthState = shared.New(
	"OAUTH_STATE_INVALID",
	"Proses masuk tidak valid atau sudah kedaluwarsa. Silakan coba lagi.",
	shared.ErrValidation,
)

// errOAuthCancelled dikembalikan saat pengguna menolak di halaman consent.
var errOAuthCancelled = shared.New(
	"OAUTH_CANCELLED",
	"Masuk dengan Google dibatalkan.",
	shared.ErrValidation,
)

// Redirect menangani GET /api/v1/auth/google.
func (h *GoogleOAuthHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	if !h.oauth.Enabled() {
		response.Fail(w, errOAuthDisabled)
		return
	}

	state := h.signState(randomState())
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     oauthStatePath,
		MaxAge:   600, // 10 menit: cukup menyelesaikan consent, tidak menggantung
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.oauth.AuthCodeURL(state), http.StatusFound)
}

// Callback menangani GET /api/v1/auth/google/callback.
func (h *GoogleOAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.oauth.Enabled() {
		response.Fail(w, errOAuthDisabled)
		return
	}

	query := r.URL.Query()
	if query.Get("error") != "" {
		response.Fail(w, errOAuthCancelled)
		return
	}

	code := strings.TrimSpace(query.Get("code"))
	state := strings.TrimSpace(query.Get("state"))
	if code == "" || state == "" {
		response.Fail(w, errOAuthState)
		return
	}

	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || !hmac.Equal([]byte(cookie.Value), []byte(state)) || !h.verifyState(state) {
		response.Fail(w, errOAuthState)
		return
	}
	// State sekali pakai: hanguskan sebelum melanjutkan.
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: "", Path: oauthStatePath, MaxAge: -1,
	})

	profile, err := h.oauth.Exchange(r.Context(), code)
	if err != nil {
		// Detail vendor tidak dibocorkan ke klien; pesan umum + kode stabil.
		response.Fail(w, shared.New(
			"OAUTH_FAILED",
			"Gagal masuk dengan Google. Silakan coba lagi.",
			shared.ErrUnauthorized,
		))
		return
	}

	res, err := h.svc.OAuthLogin(r.Context(), profile, deviceNameFrom(r), auth.ContextInfo{
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
		RequestID: requestID(r),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, sessionResponse(res))
}

// ─── state ────────────────────────────────────────────────────────────────

func randomState() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// signState menempelkan tag HMAC sehingga state tidak dapat dikarang klien.
func (h *GoogleOAuthHandler) signState(state string) string {
	mac := hmac.New(sha256.New, []byte(h.stateSecret))
	mac.Write([]byte(state))
	return state + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyState memeriksa tag HMAC pada state.
func (h *GoogleOAuthHandler) verifyState(signed string) bool {
	idx := strings.LastIndexByte(signed, '.')
	if idx <= 0 {
		return false
	}
	return hmac.Equal([]byte(signed), []byte(h.signState(signed[:idx])))
}

func deviceNameFrom(r *http.Request) string {
	// FE boleh mengirim label perangkat; kalau tidak, sesi memakai user-agent.
	return strings.TrimSpace(r.URL.Query().Get("device_name"))
}
