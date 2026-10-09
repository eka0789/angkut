// Package organizationapp berisi use case tenant: membuat organisasi,
// mengelola anggota, dan menegakkan hierarki role.
//
// Setiap use case menerima `actor` (anggota yang beraksi) yang berasal dari
// context autentikasi — BUKAN dari body request. Ini yang membuat otorisasi
// tidak dapat dipalsukan klien.
package organizationapp

import (
	"context"

	"github.com/angkut/angkut/apps/api/internal/domain/audit"
	"github.com/angkut/angkut/apps/api/internal/domain/organization"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
)

// Actor adalah anggota yang sedang beraksi di dalam sebuah organisasi.
type Actor struct {
	UserID         string
	OrganizationID string
	RoleCode       organization.RoleCode
	MemberID       string
}

// ContextInfo membawa metadata request untuk audit.
type ContextInfo struct {
	IP        string
	UserAgent string
	RequestID string
}

// Deps adalah dependensi service organisasi.
type Deps struct {
	DB    *database.Stores
	Tx    *database.TxManager
	Audit audit.Recorder
}

// Service mengimplementasikan use case organisasi.
type Service struct {
	deps Deps
}

// New membuat service organisasi.
func New(d Deps) *Service { return &Service{deps: d} }

// Create membuat organisasi baru; pembuatnya otomatis menjadi owner.
func (s *Service) Create(ctx context.Context, name, userID string, meta ContextInfo) (*organization.Organization, error) {
	if err := organization.ValidateName(name); err != nil {
		return nil, err
	}

	org := organization.NewOrganization(name, userID)
	owner := organization.NewOwnerMember(org.ID, userID)

	err := s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)
		if err := st.Orgs().CreateWithOwner(ctx, org, owner); err != nil {
			return err
		}
		return st.Audit().Record(ctx, audit.New(audit.ActionOrgCreated, userID, org.ID).
			WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
			WithEntity("organization", org.ID).
			WithChanges(nil, map[string]any{"name": org.Name, "slug": org.Slug}))
	})
	if err != nil {
		return nil, err
	}
	return org, nil
}

// ListForUser mengembalikan organisasi yang boleh dilihat user.
func (s *Service) ListForUser(ctx context.Context, userID string) ([]organization.MembershipInfo, error) {
	return s.deps.DB.Orgs().ListForUser(ctx, userID)
}

// requireMembership memastikan user benar-benar anggota organisasi dan
// mengembalikan role-nya. Ini gerbang tunggal yang dipakai semua use case
// berikutnya — kalau tidak lolos di sini, tidak ada data organisasi yang tersentuh.
func (s *Service) requireMembership(ctx context.Context, orgID, userID string) (*organization.Member, error) {
	m, err := s.deps.DB.Orgs().GetMember(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	switch m.Status {
	case organization.MemberActive:
		return m, nil
	case organization.MemberInvited:
		return nil, shared.New("INVITATION_NOT_ACCEPTED",
			"Anda belum menerima undangan organisasi ini.", shared.ErrForbidden)
	default:
		return nil, organization.ErrNotMember
	}
}

// Get mengembalikan detail organisasi untuk anggotanya.
func (s *Service) Get(ctx context.Context, orgID, userID string) (*organization.Organization, organization.RoleCode, error) {
	m, err := s.requireMembership(ctx, orgID, userID)
	if err != nil {
		return nil, "", err
	}
	org, err := s.deps.DB.Orgs().GetByID(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	return org, m.RoleCode, nil
}

// UpdateProfileInput adalah perubahan profil organisasi.
type UpdateProfileInput struct {
	Name         *string
	LegalName    *string
	BusinessType *string
	Phone        *string
	Email        *string
	Timezone     *string
}

// Update mengubah profil organisasi. Memerlukan izin `organization.manage`.
func (s *Service) Update(ctx context.Context, orgID, userID string, in UpdateProfileInput, meta ContextInfo) (*organization.Organization, error) {
	m, err := s.requireMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if !m.RoleCode.Can("organization.manage") {
		return nil, shared.New("FORBIDDEN",
			"Anda tidak berwenang mengubah pengaturan organisasi.", shared.ErrForbidden)
	}

	org, err := s.deps.DB.Orgs().GetByID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	before := map[string]any{"name": org.Name, "legal_name": org.LegalName, "business_type": org.BusinessType}

	if in.Name != nil {
		if err := organization.ValidateName(*in.Name); err != nil {
			return nil, err
		}
		org.Name = *in.Name
	}
	if in.LegalName != nil {
		org.LegalName = *in.LegalName
	}
	if in.BusinessType != nil {
		org.BusinessType = *in.BusinessType
	}
	if in.Phone != nil {
		org.Phone = *in.Phone
	}
	if in.Email != nil {
		org.Email = *in.Email
	}
	if in.Timezone != nil {
		org.Timezone = *in.Timezone
	}

	if err := s.deps.DB.Orgs().Update(ctx, org); err != nil {
		return nil, err
	}
	_ = s.deps.Audit.Record(ctx, audit.New(audit.ActionOrgUpdated, userID, orgID).
		WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
		WithEntity("organization", orgID).
		WithChanges(before, map[string]any{"name": org.Name, "legal_name": org.LegalName}))
	return org, nil
}
