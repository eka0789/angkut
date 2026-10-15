// Package organization adalah domain tenant: organisasi, keanggotaan, dan RBAC.
// Semua tabel di sini ber-scope organisasi — isolasi tenant dimulai dari sini.
package organization

import (
	"regexp"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Status organisasi.
type OrganizationStatus string

const (
	OrgActive    OrganizationStatus = "active"
	OrgSuspended OrganizationStatus = "suspended"
	OrgClosed    OrganizationStatus = "closed"
)

// MemberStatus adalah siklus hidup keanggotaan.
type MemberStatus string

const (
	MemberInvited   MemberStatus = "invited"
	MemberActive    MemberStatus = "active"
	MemberSuspended MemberStatus = "suspended"
	MemberRemoved   MemberStatus = "removed"
)

// Organization adalah tenant. Semua data bisnis menggantung di sini.
type Organization struct {
	ID           string
	Name         string
	Slug         string
	LegalName    string
	BusinessType string
	Phone        string
	Email        string
	LogoURL      string
	Timezone     string
	Currency     string
	OwnerUserID  string
	Status       OrganizationStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

// Slugify mengubah nama organisasi menjadi slug yang aman untuk URL.
// Hasilnya selalu memenuhi slugPattern, termasuk untuk nama yang seluruhnya
// berisi karakter non-ASCII (jatuh ke "org").
func Slugify(name string) string {
	var b strings.Builder
	lastDash := true // mencegah dash di awal
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) < 2 {
		return "org"
	}
	if len(slug) > 62 {
		slug = strings.Trim(slug[:62], "-")
	}
	return slug
}

// ValidateName menolak nama organisasi yang kosong atau terlalu panjang.
func ValidateName(name string) error {
	n := strings.TrimSpace(name)
	if len(n) < 2 {
		return shared.Validation(map[string]string{"name": "minimal 2 karakter."})
	}
	if len(n) > 120 {
		return shared.Validation(map[string]string{"name": "maksimal 120 karakter."})
	}
	return nil
}

// NewOrganization menyusun organisasi baru dengan pemilik tertentu.
func NewOrganization(name, ownerUserID string) *Organization {
	now := time.Now().UTC()
	display := strings.TrimSpace(name)
	return &Organization{
		ID:          platform.NewUUID(),
		Name:        display,
		Slug:        Slugify(display),
		OwnerUserID: ownerUserID,
		Timezone:    "Asia/Jakarta",
		Currency:    "IDR",
		Status:      OrgActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Member adalah keanggotaan seorang user di organisasi, membawa role-nya.
type Member struct {
	ID             string
	OrganizationID string
	UserID         string
	RoleID         string
	RoleCode       RoleCode // ikut dimuat agar otorisasi tidak perlu join tambahan
	Status         MemberStatus
	InvitedBy      string
	InvitedEmail   string
	JoinedAt       *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsActive melaporkan apakah anggota dapat beraksi.
func (m *Member) IsActive() bool { return m.Status == MemberActive }

// Activate mengaktifkan anggota setelah undangan diterima.
func (m *Member) Activate(now time.Time) {
	m.Status = MemberActive
	m.JoinedAt = &now
	m.UpdatedAt = now
}

// NewOwnerMember menyusun keanggotaan owner untuk pendiri organisasi.
func NewOwnerMember(orgID, userID string) *Member {
	now := time.Now().UTC()
	return &Member{
		ID:             platform.NewUUID(),
		OrganizationID: orgID,
		UserID:         userID,
		RoleID:         RoleOwnerID,
		RoleCode:       RoleOwner,
		Status:         MemberActive,
		JoinedAt:       &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
