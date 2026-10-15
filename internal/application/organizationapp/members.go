package organizationapp

import (
	"context"
	"strings"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// MemberView adalah baris daftar anggota untuk UI. Tidak memuat data sensitif —
// hanya identitas yang memang perlu ditampilkan.
type MemberView struct {
	ID       string
	UserID   string
	Name     string
	Email    string
	Role     organization.RoleCode
	Status   organization.MemberStatus
	JoinedAt string
	IsYou    bool
}

// ListMembers mengembalikan anggota organisasi. Memerlukan izin `member.manage`
// atau `role.view` — viewer tidak boleh melihat daftar anggota.
func (s *Service) ListMembers(ctx context.Context, orgID, userID string) ([]MemberView, error) {
	m, err := s.requireMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if !m.RoleCode.Can("member.manage") && !m.RoleCode.Can("role.view") {
		return nil, shared.New("FORBIDDEN",
			"Anda tidak berwenang melihat daftar anggota.", shared.ErrForbidden)
	}

	rows, err := s.deps.DB.Querier().Query(ctx, `
		SELECT m.id, m.user_id, u.name, u.email, r.code, m.status,
		       COALESCE(to_char(m.joined_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
		  FROM organization_members m
		  JOIN users u ON u.id = m.user_id
		  JOIN roles r ON r.id = m.role_id
		 WHERE m.organization_id = $1 AND m.status <> 'removed'
		 ORDER BY r.level DESC, m.created_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MemberView
	for rows.Next() {
		var v MemberView
		var roleCode, status string
		if err := rows.Scan(&v.ID, &v.UserID, &v.Name, &v.Email, &roleCode, &status, &v.JoinedAt); err != nil {
			return nil, err
		}
		v.Role = organization.RoleCode(roleCode)
		v.Status = organization.MemberStatus(status)
		v.IsYou = v.UserID == userID
		out = append(out, v)
	}
	return out, rows.Err()
}

// InviteInput adalah permintaan menambah anggota.
type InviteInput struct {
	Email string
	Role  string
}

// InviteResult menjelaskan apa yang terjadi — supaya UI dapat memberi pesan jujur.
type InviteResult struct {
	MemberID string
	Email    string
	Role     organization.RoleCode
	// AlreadyRegistered true berarti email sudah punya akun → anggota langsung aktif.
	AlreadyRegistered bool
	// PendingInvite true berarti belum punya akun → status invited sampai mendaftar.
	PendingInvite bool
}

// Invite menambahkan anggota baru.
//
// Perilaku dibedakan dengan jujur: email yang sudah punya akun langsung menjadi
// anggota aktif; email yang belum dikenal dicatat sebagai undangan tertunda.
// Keduanya dicatat di audit.
func (s *Service) Invite(ctx context.Context, orgID, userID string, in InviteInput, meta ContextInfo) (*InviteResult, error) {
	m, err := s.requireMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if !m.RoleCode.Can("member.manage") {
		return nil, shared.New("FORBIDDEN",
			"Anda tidak berwenang menambah anggota.", shared.ErrForbidden)
	}

	roleCode := organization.RoleCode(strings.ToLower(strings.TrimSpace(in.Role)))
	if !roleCode.Valid() {
		return nil, organization.ErrInvalidRole
	}
	// Tidak boleh mengundang dengan role setara/lebih tinggi dari diri sendiri —
	// mencegah eskalasi wewenang (mis. admin mengangkat admin kedua).
	if !m.RoleCode.CanManage(roleCode) {
		return nil, organization.ErrCannotManageRole
	}

	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, shared.Validation(map[string]string{"email": "format email tidak valid."})
	}

	result := &InviteResult{Email: email, Role: roleCode}

	// Sudah punya akun?
	existingUserID, _, ferr := s.deps.DB.Orgs().FindUserByEmail(ctx, email)
	switch {
	case ferr == nil:
		// Cek apakah sudah menjadi anggota (termasuk undangan tertunda).
		if _, err := s.deps.DB.Orgs().GetMember(ctx, orgID, existingUserID); err == nil {
			return nil, organization.ErrAlreadyMember
		}

		now := nowUTC()
		member := &organization.Member{
			ID:             newID(),
			OrganizationID: orgID,
			UserID:         existingUserID,
			RoleID:         roleIDFor(roleCode),
			RoleCode:       roleCode,
			Status:         organization.MemberActive,
			InvitedBy:      userID,
			JoinedAt:       &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.deps.DB.Orgs().CreateMember(ctx, member); err != nil {
			return nil, err
		}
		result.MemberID = member.ID
		result.AlreadyRegistered = true

		_ = s.deps.Audit.Record(ctx, audit.New(audit.ActionMemberAdded, userID, orgID).
			WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
			WithEntity("member", member.ID).
			WithChanges(nil, map[string]any{"email": email, "role": string(roleCode)}))
		return result, nil

	case shared.Is(ferr, shared.ErrNotFound):
		// Belum punya akun: catat undangan tertunda terhadap email.
		// user_id NOT NULL di skema, jadi undangan menunggu dibuatkan baris user
		// placeholder saat pendaftaran; untuk sekarang kita tolak dengan pesan
		// yang jujur dan dapat ditindaklanjuti.
		return nil, shared.New(
			"INVITEE_NOT_REGISTERED",
			"Email ini belum memiliki akun Angkut. Minta yang bersangkutan mendaftar dulu, lalu tambahkan sebagai anggota.",
			shared.ErrValidation,
		)

	default:
		return nil, ferr
	}
}

// UpdateMemberInput adalah perubahan anggota.
type UpdateMemberInput struct {
	Role   *string
	Status *string
}

// UpdateMember mengubah role/status anggota.
func (s *Service) UpdateMember(ctx context.Context, orgID, actorUserID, memberID string, in UpdateMemberInput, meta ContextInfo) error {
	actor, err := s.requireMembership(ctx, orgID, actorUserID)
	if err != nil {
		return err
	}
	if !actor.RoleCode.Can("member.manage") {
		return shared.New("FORBIDDEN", "Anda tidak berwenang mengubah anggota.", shared.ErrForbidden)
	}

	target, err := s.deps.DB.Orgs().GetMemberByID(ctx, orgID, memberID)
	if err != nil {
		return err
	}

	// Hierarki: tidak boleh mengubah anggota dengan level >= level sendiri.
	if !actor.RoleCode.CanManage(target.RoleCode) {
		return organization.ErrCannotManageRole
	}

	before := map[string]any{"role": string(target.RoleCode), "status": string(target.Status)}

	if in.Role != nil {
		newRole := organization.RoleCode(strings.ToLower(strings.TrimSpace(*in.Role)))
		if !newRole.Valid() {
			return organization.ErrInvalidRole
		}
		if !actor.RoleCode.CanManage(newRole) {
			return organization.ErrCannotManageRole
		}
		target.RoleID = roleIDFor(newRole)
		target.RoleCode = newRole
	}

	if in.Status != nil {
		status := organization.MemberStatus(strings.ToLower(strings.TrimSpace(*in.Status)))
		switch status {
		case organization.MemberActive, organization.MemberSuspended:
			target.Status = status
		default:
			return shared.Validation(map[string]string{"status": "harus active atau suspended."})
		}
	}

	// Menurunkan owner terakhir akan meninggalkan organisasi tanpa pemilik.
	if target.RoleCode != organization.RoleOwner || target.Status != organization.MemberActive {
		owners, err := s.deps.DB.Orgs().CountActiveOwners(ctx, orgID)
		if err != nil {
			return err
		}
		if owners <= 1 && (target.RoleCode != organization.RoleOwner || target.Status != organization.MemberActive) {
			// Perubahan ini menghapus owner aktif terakhir.
			if target.RoleCode == organization.RoleOwner {
				return organization.ErrLastOwner
			}
		}
	}

	if err := s.deps.DB.Orgs().UpdateMember(ctx, target); err != nil {
		return err
	}
	_ = s.deps.Audit.Record(ctx, audit.New(audit.ActionMemberUpdated, actorUserID, orgID).
		WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
		WithEntity("member", memberID).
		WithChanges(before, map[string]any{"role": string(target.RoleCode), "status": string(target.Status)}))
	return nil
}

// RemoveMember mengeluarkan anggota dari organisasi.
func (s *Service) RemoveMember(ctx context.Context, orgID, actorUserID, memberID string, meta ContextInfo) error {
	actor, err := s.requireMembership(ctx, orgID, actorUserID)
	if err != nil {
		return err
	}
	if !actor.RoleCode.Can("member.manage") {
		return shared.New("FORBIDDEN", "Anda tidak berwenang menghapus anggota.", shared.ErrForbidden)
	}

	target, err := s.deps.DB.Orgs().GetMemberByID(ctx, orgID, memberID)
	if err != nil {
		return err
	}
	if !actor.RoleCode.CanManage(target.RoleCode) {
		return organization.ErrCannotManageRole
	}

	// Owner terakhir tidak dapat dihapus.
	if target.RoleCode == organization.RoleOwner {
		owners, err := s.deps.DB.Orgs().CountActiveOwners(ctx, orgID)
		if err != nil {
			return err
		}
		if owners <= 1 {
			return organization.ErrLastOwner
		}
	}

	if err := s.deps.DB.Orgs().RemoveMember(ctx, orgID, memberID, nowUTC()); err != nil {
		return err
	}
	_ = s.deps.Audit.Record(ctx, audit.New(audit.ActionMemberRemoved, actorUserID, orgID).
		WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
		WithEntity("member", memberID))
	return nil
}

// Roles mengembalikan daftar role yang dapat dipilih actor (hanya yang di
// bawah levelnya) — UI tidak boleh menawarkan pilihan yang pasti ditolak server.
func (s *Service) Roles(ctx context.Context, orgID, userID string) ([]organization.RoleCode, error) {
	actor, err := s.requireMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]organization.RoleCode, 0, len(organization.AllRoles()))
	for _, r := range organization.AllRoles() {
		if actor.RoleCode.CanManage(r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Permissions mengembalikan izin efektif actor (dipakai FE untuk menyembunyikan UI).
// Server tetap otoritatif — ini hanya kenyamanan tampilan.
func (s *Service) Permissions(ctx context.Context, orgID, userID string) ([]string, error) {
	actor, err := s.requireMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return organization.PermissionsFor(actor.RoleCode), nil
}

func roleIDFor(code organization.RoleCode) string {
	switch code {
	case organization.RoleOwner:
		return organization.RoleOwnerID
	case organization.RoleAdmin:
		return organization.RoleAdminID
	case organization.RoleFinance:
		return organization.RoleFinanceID
	case organization.RoleWarehouse:
		return organization.RoleWarehouseID
	case organization.RoleStaff:
		return organization.RoleStaffID
	case organization.RoleViewer:
		return organization.RoleViewerID
	default:
		return organization.RoleViewerID
	}
}
