package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// OrganizationRepo adalah implementasi organization.Repository.
//
// ATURAN ISOLASI (ADR-008): setiap query di file ini menyaring kolom yang
// mengikat data ke organisasi/user. Tidak ada method yang mengembalikan baris
// milik organisasi lain, bahkan bila id-nya diketahui.
type OrganizationRepo struct {
	q Querier
}

// NewOrganizationRepo membuat repository organisasi.
func NewOrganizationRepo(q Querier) *OrganizationRepo { return &OrganizationRepo{q: q} }

// WithTx menjalankan repository di transaksi tertentu.
func (r *OrganizationRepo) WithTx(q Querier) *OrganizationRepo {
	return &OrganizationRepo{q: q}
}

const orgColumns = `
	id, name, slug, legal_name, business_type, phone, email, logo_url,
	timezone, currency, owner_user_id, status, created_at, updated_at, deleted_at`

// CreateWithOwner membuat organisasi + keanggotaan owner dalam SATU transaksi.
// Organisasi tanpa owner adalah data yatim — kombinasi ini tidak boleh setengah jadi.
func (r *OrganizationRepo) CreateWithOwner(ctx context.Context, org *organization.Organization, owner *organization.Member) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO organizations (
			id, name, slug, legal_name, business_type, phone, email, logo_url,
			timezone, currency, owner_user_id, status, created_at, updated_at
		) VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),
		          $9,$10,$11,$12,$13,$14)`,
		org.ID, org.Name, org.Slug, org.LegalName, org.BusinessType, org.Phone, org.Email, org.LogoURL,
		org.Timezone, org.Currency, org.OwnerUserID, string(org.Status), org.CreatedAt, org.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			// Slug bertabrakan: pengguna cukup mengganti nama; ini bukan kegagalan fatal.
			return shared.Conflict("SLUG_TAKEN", "Nama organisasi sudah dipakai. Coba nama lain.")
		}
		return err
	}

	_, err = r.q.Exec(ctx, `
		INSERT INTO organization_members (
			id, organization_id, user_id, role_id, status, joined_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		owner.ID, owner.OrganizationID, owner.UserID, owner.RoleID, string(owner.Status),
		owner.JoinedAt, owner.CreatedAt, owner.UpdatedAt,
	)
	return err
}

// GetByID mengambil organisasi yang belum dihapus.
func (r *OrganizationRepo) GetByID(ctx context.Context, id string) (*organization.Organization, error) {
	row := r.q.QueryRow(ctx, `SELECT `+orgColumns+` FROM organizations WHERE id=$1 AND deleted_at IS NULL`, id)
	return scanOrg(row)
}

// Update menyimpan perubahan profil organisasi.
func (r *OrganizationRepo) Update(ctx context.Context, org *organization.Organization) error {
	_, err := r.q.Exec(ctx, `
		UPDATE organizations SET
			name=$2, legal_name=NULLIF($3,''), business_type=NULLIF($4,''),
			phone=NULLIF($5,''), email=NULLIF($6,''), logo_url=NULLIF($7,''),
			timezone=$8, currency=$9
		WHERE id=$1 AND deleted_at IS NULL`,
		org.ID, org.Name, org.LegalName, org.BusinessType, org.Phone, org.Email, org.LogoURL,
		org.Timezone, org.Currency)
	return err
}

// ListForUser mengembalikan organisasi tempat user aktif menjadi anggota,
// beserta role efektifnya. Ini satu-satunya jalan daftar organisasi user —
// tidak ada endpoint yang membolehkan melihat organisasi orang lain.
func (r *OrganizationRepo) ListForUser(ctx context.Context, userID string) ([]organization.MembershipInfo, error) {
	rows, err := r.q.Query(ctx, `
		SELECT o.`+"id"+`, o.name, o.slug, o.legal_name, o.business_type, o.phone, o.email, o.logo_url,
		       o.timezone, o.currency, o.owner_user_id, o.status, o.created_at, o.updated_at, o.deleted_at,
		       m.id, ro.code
		  FROM organization_members m
		  JOIN organizations o ON o.id = m.organization_id
		  JOIN roles ro ON ro.id = m.role_id
		 WHERE m.user_id = $1
		   AND m.status = 'active'
		   AND o.deleted_at IS NULL
		 ORDER BY o.created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]organization.MembershipInfo, 0, 4)
	for rows.Next() {
		var info organization.MembershipInfo
		var memberID, roleCode string
		var legal, businessType, phone, email, logo *string

		err := rows.Scan(
			&info.Organization.ID, &info.Organization.Name, &info.Organization.Slug,
			&legal, &businessType, &phone, &email, &logo,
			&info.Organization.Timezone, &info.Organization.Currency,
			&info.Organization.OwnerUserID, &info.Organization.Status,
			&info.Organization.CreatedAt, &info.Organization.UpdatedAt, &info.Organization.DeletedAt,
			&memberID, &roleCode,
		)
		if err != nil {
			return nil, err
		}
		info.Organization.LegalName = deref(legal)
		info.Organization.BusinessType = deref(businessType)
		info.Organization.Phone = deref(phone)
		info.Organization.Email = deref(email)
		info.Organization.LogoURL = deref(logo)
		info.MemberID = memberID
		info.RoleCode = organization.RoleCode(roleCode)
		out = append(out, info)
	}
	return out, rows.Err()
}

// GetMember mengambil keanggotaan user pada organisasi tertentu.
func (r *OrganizationRepo) GetMember(ctx context.Context, orgID, userID string) (*organization.Member, error) {
	row := r.q.QueryRow(ctx, `
		SELECT m.id, m.organization_id, m.user_id, m.role_id, ro.code, m.status,
		       COALESCE(m.invited_by::text,''), COALESCE(m.invited_email,''),
		       m.joined_at, m.created_at, m.updated_at
		  FROM organization_members m
		  JOIN roles ro ON ro.id = m.role_id
		 WHERE m.organization_id = $1 AND m.user_id = $2`, orgID, userID)
	return scanMember(row)
}

// GetMemberByID mengambil keanggotaan berdasarkan id, dibatasi organisasi.
func (r *OrganizationRepo) GetMemberByID(ctx context.Context, orgID, memberID string) (*organization.Member, error) {
	row := r.q.QueryRow(ctx, `
		SELECT m.id, m.organization_id, m.user_id, m.role_id, ro.code, m.status,
		       COALESCE(m.invited_by::text,''), COALESCE(m.invited_email,''),
		       m.joined_at, m.created_at, m.updated_at
		  FROM organization_members m
		  JOIN roles ro ON ro.id = m.role_id
		 WHERE m.organization_id = $1 AND m.id = $2`, orgID, memberID)
	m, err := scanMember(row)
	if err != nil {
		if shared.Is(err, shared.ErrNotFound) {
			return nil, organization.ErrMemberNotFound
		}
		return nil, err
	}
	return m, nil
}

// ListMembers mengembalikan seluruh anggota organisasi (termasuk yang diundang),
// diurutkan dari role tertinggi.
func (r *OrganizationRepo) ListMembers(ctx context.Context, orgID string) ([]organization.Member, error) {
	rows, err := r.q.Query(ctx, `
		SELECT m.id, m.organization_id, m.user_id, m.role_id, ro.code, m.status,
		       COALESCE(m.invited_by::text,''), COALESCE(m.invited_email,''),
		       m.joined_at, m.created_at, m.updated_at
		  FROM organization_members m
		  JOIN roles ro ON ro.id = m.role_id
		 WHERE m.organization_id = $1
		 ORDER BY ro.level DESC, m.created_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []organization.Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// CreateMember menambahkan anggota baru.
func (r *OrganizationRepo) CreateMember(ctx context.Context, m *organization.Member) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO organization_members (
			id, organization_id, user_id, role_id, status, invited_by, invited_email,
			joined_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,''),$8,$9,$10)`,
		m.ID, m.OrganizationID, m.UserID, m.RoleID, string(m.Status),
		m.InvitedBy, m.InvitedEmail, m.JoinedAt, m.CreatedAt, m.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return organization.ErrAlreadyMember
		}
		return err
	}
	return nil
}

// UpdateMember menyimpan perubahan role/status anggota.
func (r *OrganizationRepo) UpdateMember(ctx context.Context, m *organization.Member) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE organization_members SET role_id=$3, status=$4
		 WHERE organization_id=$1 AND id=$2`,
		m.OrganizationID, m.ID, m.RoleID, string(m.Status))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return organization.ErrMemberNotFound
	}
	return nil
}

// RemoveMember menandai anggota keluar. Baris TIDAK dihapus agar jejak audit
// dan riwayat tetap utuh.
func (r *OrganizationRepo) RemoveMember(ctx context.Context, orgID, memberID string, now time.Time) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE organization_members SET status='removed'
		 WHERE organization_id=$1 AND id=$2 AND status <> 'removed'`, orgID, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return organization.ErrMemberNotFound
	}
	return nil
}

// CountActiveOwners menghitung owner aktif — dasar penegakan invariant
// "organisasi selalu punya minimal satu owner".
func (r *OrganizationRepo) CountActiveOwners(ctx context.Context, orgID string) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `
		SELECT count(*) FROM organization_members m
		  JOIN roles ro ON ro.id = m.role_id
		 WHERE m.organization_id=$1 AND m.status='active' AND ro.code='owner'`, orgID).Scan(&n)
	return n, err
}

// FindUserByEmail mencari akun yang sudah ada (untuk undangan langsung aktif).
func (r *OrganizationRepo) FindUserByEmail(ctx context.Context, email string) (string, string, error) {
	var id, name string
	err := r.q.QueryRow(ctx,
		`SELECT id, name FROM users WHERE lower(email)=lower($1) AND deleted_at IS NULL`, email).Scan(&id, &name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", shared.ErrNotFound
		}
		return "", "", err
	}
	return id, name, nil
}

func scanOrg(row pgx.Row) (*organization.Organization, error) {
	var o organization.Organization
	var legal, businessType, phone, email, logo *string
	var status string

	err := row.Scan(
		&o.ID, &o.Name, &o.Slug, &legal, &businessType, &phone, &email, &logo,
		&o.Timezone, &o.Currency, &o.OwnerUserID, &status,
		&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, organization.ErrOrganizationNotFound
		}
		return nil, err
	}
	o.LegalName = deref(legal)
	o.BusinessType = deref(businessType)
	o.Phone = deref(phone)
	o.Email = deref(email)
	o.LogoURL = deref(logo)
	o.Status = organization.OrganizationStatus(status)
	return &o, nil
}

func scanMember(row pgx.Row) (*organization.Member, error) {
	var m organization.Member
	var status, roleCode string
	var joinedAt *time.Time

	err := row.Scan(
		&m.ID, &m.OrganizationID, &m.UserID, &m.RoleID, &roleCode, &status,
		&m.InvitedBy, &m.InvitedEmail, &joinedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Sengaja memakai pesan "organisasi tidak ditemukan" untuk konteks
			// resolusi tenant agar keberadaan organisasi tidak bocor.
			return nil, organization.ErrNotMember
		}
		return nil, err
	}
	m.RoleCode = organization.RoleCode(roleCode)
	m.Status = organization.MemberStatus(status)
	m.JoinedAt = joinedAt
	return &m, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
