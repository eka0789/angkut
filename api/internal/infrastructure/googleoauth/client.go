// Package googleoauth mengimplementasikan port GoogleOAuth di atas endpoint
// OAuth2 + OpenID Connect Google.
//
// Saat kredensial belum diisi, adapter ini DISABLED dan menolak semua operasi.
// Itu disengaja: lebih baik fitur mati dengan pesan jujur daripada diam-diam
// menerima login yang tidak terverifikasi.
package googleoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/application/auth"
)

const (
	authEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenEndpoint    = "https://oauth2.googleapis.com/token"
	userInfoEndpoint = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// Client adalah adapter Google OAuth.
type Client struct {
	clientID     string
	clientSecret string
	redirectURL  string
	http         *http.Client
}

// New membuat client. Bila kredensial kosong, Enabled() bernilai false.
func New(clientID, clientSecret, redirectURL string) *Client {
	return &Client{
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		redirectURL:  strings.TrimSpace(redirectURL),
		http:         &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled melaporkan apakah Google OAuth siap dipakai.
func (c *Client) Enabled() bool {
	return c.clientID != "" && c.clientSecret != "" && c.redirectURL != ""
}

// AuthCodeURL menyusun URL consent. `state` datang dari cookie ber-signature
// yang dipasang handler — bukan dari query string klien.
func (c *Client) AuthCodeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("access_type", "online")
	q.Set("prompt", "select_account")
	return authEndpoint + "?" + q.Encode()
}

// ErrDisabled dikembalikan saat kredensial belum diisi.
var ErrDisabled = errors.New("google oauth belum dikonfigurasi")

// Exchange menukar authorization code menjadi identitas pengguna.
func (c *Client) Exchange(ctx context.Context, code string) (*auth.GoogleUser, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("authorization code kosong")
	}

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("redirect_uri", c.redirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("menukar code ke token: %w", err)
	}
	defer resp.Body.Close()

	var token struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("membaca respons token: %w", err)
	}
	if token.Error != "" || token.AccessToken == "" {
		// Pesan tidak membocorkan detail vendor ke pengguna; handler memetakannya
		// menjadi pesan umum.
		return nil, fmt.Errorf("google menolak penukaran token: %s", token.Error)
	}

	return c.fetchUserInfo(ctx, token.AccessToken)
}

func (c *Client) fetchUserInfo(ctx context.Context, accessToken string) (*auth.GoogleUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mengambil profil google: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google mengembalikan status %d", resp.StatusCode)
	}

	var profile struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("membaca profil google: %w", err)
	}
	if profile.Sub == "" {
		return nil, errors.New("profil google tidak memuat subject")
	}

	return &auth.GoogleUser{
		Subject:       profile.Sub,
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		Name:          profile.Name,
		Picture:       profile.Picture,
	}, nil
}
