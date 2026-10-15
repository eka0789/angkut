package auth

import (
	"context"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// Refresh menukar refresh token lama dengan yang baru (rotasi).
//
// Aturan keamanan yang ditegakkan di sini (ADR-007):
//  1. Token lama langsung dicabut saat rotasi — hanya boleh dipakai sekali.
//  2. Bila token yang SUDAH dirotasi dipakai lagi, itu indikasi pencurian:
//     seluruh sesi user dicabut, bukan hanya request ini ditolak.
//  3. Sesi yang dicabut / kedaluwarsa tidak dapat di-refresh.
func (s *Service) Refresh(ctx context.Context, rawToken, deviceName string, meta ContextInfo) (*AuthResult, error) {
	rawToken = trim(rawToken)
	if rawToken == "" {
		return nil, shared.New("REFRESH_TOKEN_MISSING", "Refresh token tidak disertakan.", shared.ErrUnauthorized)
	}

	hash := user.HashToken(rawToken)
	now := s.now()

	sess, err := s.deps.DB.Sessions().GetByRefreshHash(ctx, hash)
	if err != nil {
		if shared.Is(err, shared.ErrNotFound) {
			return nil, shared.New("REFRESH_TOKEN_INVALID",
				"Sesi tidak ditemukan. Silakan masuk kembali.", shared.ErrUnauthorized)
		}
		return nil, err
	}

	// Kasus 2/3: token ditemukan tetapi sudah tidak berlaku.
	if sess.RevokedAt != nil {
		return s.handlePossibleReuse(ctx, sess, meta)
	}
	if !now.Before(sess.ExpiresAt) {
		return nil, user.ErrSessionExpired
	}

	u, err := s.deps.DB.Users().GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	// Password berubah setelah sesi diterbitkan → sesi lama tidak boleh lanjut.
	if u.PasswordChangedAt != nil && sess.CreatedAt.Before(*u.PasswordChangedAt) {
		sess.Revoke(now)
		_, _ = s.deps.DB.Sessions().RevokeAll(ctx, u.ID, now)
		return nil, user.ErrSessionExpired
	}

	raw, nextHash, err := user.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}
	next := user.NewSession(sess.UserID, nextHash, deviceNameOr(sess, deviceName), sess.UserAgent, meta.IP, user.RefreshTokenTTL)
	// Sesi lanjutan mempertahankan identitas perangkat aslinya.
	next.DeviceName = deviceNameOr(sess, deviceName)

	memberships, err := s.deps.DB.Orgs().ListForUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	activeOrg, activeRole := pickActiveOrg(memberships, "")

	err = s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)
		if err := st.Sessions().Rotate(ctx, sess.ID, next); err != nil {
			return err
		}
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionTokenRefresh, meta).
			WithEntity("session", next.ID))
	})
	if err != nil {
		return nil, err
	}

	access, err := s.deps.JWT.Issue(u.ID, next.ID, u.Email, activeOrg, string(activeRole), now)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Session: IssuedSession{
			AccessToken:  access,
			RefreshToken: raw,
			ExpiresIn:    int(s.deps.JWT.TTL().Seconds()),
			SessionID:    next.ID,
		},
		User:          u,
		Organizations: memberships,
		ActiveOrgID:   activeOrg,
		ActiveRole:    activeRole,
	}, nil
}

// handlePossibleReuse menangani refresh token yang sudah tidak berlaku.
//
// Sesi dengan `replaced_by_id` berarti token ini pernah dirotasi → pemakaian
// ulang yang sah-sah saja adalah tanda pencurian: cabut SEMUA sesi user.
// Sesi yang dicabut tanpa pengganti (logout biasa) hanya ditolak.
func (s *Service) handlePossibleReuse(ctx context.Context, sess *user.Session, meta ContextInfo) (*AuthResult, error) {
	now := s.now()

	if sess.ReplacedByID != nil {
		revoked, _ := s.deps.DB.Sessions().RevokeAll(ctx, sess.UserID, now)
		_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionRefreshReuseBlocked, meta).
			WithEntity("user", sess.UserID).
			WithChanges(nil, map[string]any{
				"reused_session_id": sess.ID,
				"revoked_sessions":  revoked,
			}))
		return nil, user.ErrRefreshReuse
	}

	// Logout biasa: token sudah dicabut, cukup tolak tanpa alarm.
	return nil, shared.New("SESSION_REVOKED",
		"Sesi ini sudah berakhir. Silakan masuk kembali.", shared.ErrUnauthorized)
}

// Logout mencabut satu sesi (perangkat yang sedang aktif).
func (s *Service) Logout(ctx context.Context, userID, sessionID string, meta ContextInfo) error {
	now := s.now()
	revoked, err := s.deps.DB.Sessions().Revoke(ctx, userID, sessionID, now)
	if err != nil {
		return err
	}
	if revoked {
		_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionLogout, meta).
			WithEntity("session", sessionID))
	}
	return nil
}

// LogoutAll mencabut seluruh sesi user (semua perangkat) dan mengembalikan jumlahnya.
func (s *Service) LogoutAll(ctx context.Context, userID string, meta ContextInfo) (int, error) {
	now := s.now()
	n, err := s.deps.DB.Sessions().RevokeAll(ctx, userID, now)
	if err != nil {
		return 0, err
	}
	_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionLogoutAll, meta).
		WithEntity("user", userID).
		WithChanges(nil, map[string]any{"revoked_sessions": n}))
	return n, nil
}

// SessionView adalah baris daftar perangkat untuk UI.
type SessionView struct {
	ID         string
	Device     string
	IP         string
	UserAgent  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  time.Time
	IsCurrent  bool
}

// ListSessions mengembalikan perangkat aktif milik user. Sesi user lain tidak
// pernah terlihat karena query dibatasi user_id.
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionView, error) {
	now := s.now()
	sessions, err := s.deps.DB.Sessions().ListActiveByUser(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, SessionView{
			ID:         sess.ID,
			Device:     sess.DeviceLabel(),
			IP:         sess.IPAddress,
			UserAgent:  sess.UserAgent,
			CreatedAt:  sess.CreatedAt,
			LastUsedAt: sess.LastUsedAt,
			ExpiresAt:  sess.ExpiresAt,
			IsCurrent:  sess.ID == currentSessionID,
		})
	}
	return out, nil
}

// RevokeSession mencabut perangkat tertentu milik user yang sedang login.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string, meta ContextInfo) error {
	now := s.now()
	revoked, err := s.deps.DB.Sessions().Revoke(ctx, userID, sessionID, now)
	if err != nil {
		return err
	}
	if !revoked {
		// Tidak ada atau bukan miliknya — pesan sama agar keberadaan sesi
		// orang lain tidak dapat dipastikan.
		return shared.NotFound("SESSION_NOT_FOUND", "Sesi tidak ditemukan.")
	}
	_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionSessionRevoked, meta).
		WithEntity("session", sessionID))
	return nil
}

// ValidateAccessToken memverifikasi access token dan memastikan sesinya masih
// hidup. Ini yang membuat logout terasa instan walau access token masih berlaku.
func (s *Service) ValidateAccessToken(ctx context.Context, rawToken string) (*SessionContext, error) {
	now := s.now()
	claims, err := s.deps.JWT.Verify(rawToken, now)
	if err != nil {
		return nil, shared.New("TOKEN_INVALID", "Sesi tidak valid. Silakan masuk kembali.", shared.ErrUnauthorized)
	}

	sess, err := s.deps.DB.Sessions().GetByID(ctx, claims.SessionID)
	if err != nil {
		return nil, shared.New("TOKEN_INVALID", "Sesi tidak valid. Silakan masuk kembali.", shared.ErrUnauthorized)
	}
	if !sess.IsActive(now) || sess.UserID != claims.UserID {
		return nil, shared.New("TOKEN_REVOKED", "Sesi sudah berakhir. Silakan masuk kembali.", shared.ErrUnauthorized)
	}

	// Password berganti ⇒ token lama tidak berlaku lagi.
	u, err := s.deps.DB.Users().GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if u.PasswordChangedAt != nil && sess.CreatedAt.Before(*u.PasswordChangedAt) {
		return nil, shared.New("TOKEN_REVOKED", "Sesi sudah berakhir. Silakan masuk kembali.", shared.ErrUnauthorized)
	}
	if err := u.CanLogin(now); err != nil {
		return nil, err
	}

	_ = s.deps.DB.Sessions().TouchLastUsed(ctx, sess.ID, now)

	return &SessionContext{
		UserID:         u.ID,
		SessionID:      sess.ID,
		Email:          u.Email,
		Name:           u.Name,
		OrganizationID: claims.OrganizationID,
		RoleCode:       organization.RoleCode(claims.RoleCode),
	}, nil
}

// SessionContext adalah identitas pemanggil yang sudah terverifikasi. Middleware
// meneruskan ini ke handler; TIDAK ADA nilai di sini yang berasal dari input
// pengguna pada request yang sama.
type SessionContext struct {
	UserID         string
	SessionID      string
	Email          string
	Name           string
	OrganizationID string
	RoleCode       organization.RoleCode
}

func deviceNameOr(sess *user.Session, fallback string) string {
	if fallback != "" {
		return fallback
	}
	return sess.DeviceName
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
