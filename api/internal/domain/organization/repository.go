package organization

import (
	"context"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Repository adalah port penyimpanan organisasi & keanggotaan.
//
// ATURAN ISOLASI (ADR-008): SEMUA method yang menyentuh data organisasi WAJIB
// memfilter `organization_id`. Repository tidak pernah menerima tenant dari
// input pengguna — pemanggil mengambilnya dari context autentikasi.
type Repository interface {
	// Create membuat organisasi sekaligus keanggotaan owner-nya dalam SATU
	// transaksi. Organisasi tanpa owner adalah data yatim — dilarang.
	CreateWithOwner(ctx context.Context, org *Organization, owner *Member) error

	// GetByID mengambil organisasi; hanya mengembalikan yang belum dihapus.
	GetByID(ctx context.Context, id string) (*Organization, error)

	// Update menyimpan perubahan profil organisasi.
	Update(ctx context.Context, org *Organization) error

	// ListForUser mengembalikan organisasi tempat user menjadi anggota aktif,
	// beserta role efektifnya.
	ListForUser(ctx context.Context, userID string) ([]MembershipInfo, error)

	// GetMember mengambil keanggotaan user di organisasi tertentu.
	GetMember(ctx context.Context, orgID, userID string) (*Member, error)

	// GetMemberByID mengambil keanggotaan berdasarkan id (untuk kelola anggota).
	GetMemberByID(ctx context.Context, orgID, memberID string) (*Member, error)

	// ListMembers mengembalikan seluruh anggota organisasi.
	ListMembers(ctx context.Context, orgID string) ([]Member, error)

	// CreateMember menambahkan anggota (undangan atau langsung aktif).
	CreateMember(ctx context.Context, m *Member) error

	// UpdateMember menyimpan perubahan role/status anggota.
	UpdateMember(ctx context.Context, m *Member) error

	// RemoveMember menandai anggota keluar (status removed, bukan hapus baris,
	// agar jejak audit tetap utuh).
	RemoveMember(ctx context.Context, orgID, memberID string, now time.Time) error

	// CountActiveOwners menghitung owner aktif — dipakai menegakkan invariant
	// "organisasi selalu punya minimal satu owner".
	CountActiveOwners(ctx context.Context, orgID string) (int, error)

	// FindUserByEmail mencari anggota yang sudah punya akun, untuk undangan.
	FindUserByEmail(ctx context.Context, email string) (userID string, name string, err error)
}

// MembershipInfo adalah organisasi + role efektif user di dalamnya.
type MembershipInfo struct {
	Organization Organization
	RoleCode     RoleCode
	MemberID     string
}

// ErrOrganizationNotFound dipakai saat organisasi tidak ada atau sudah dihapus.
var ErrOrganizationNotFound = shared.NotFound(
	"ORGANIZATION_NOT_FOUND", "Organisasi tidak ditemukan.",
)

// ErrNotMember dikembalikan bila user bukan anggota organisasi tersebut.
// Pesan sengaja sama dengan "tidak ditemukan" agar penyerang tidak dapat
// memastikan keberadaan organisasi milik orang lain.
var ErrNotMember = shared.NotFound(
	"ORGANIZATION_NOT_FOUND", "Organisasi tidak ditemukan.",
)

// ErrMemberNotFound dipakai saat mengelola anggota.
var ErrMemberNotFound = shared.NotFound(
	"MEMBER_NOT_FOUND", "Anggota tidak ditemukan.",
)

// ErrAlreadyMember dikembalikan saat menambah anggota yang sudah ada.
var ErrAlreadyMember = shared.Conflict(
	"ALREADY_MEMBER", "Pengguna ini sudah menjadi anggota organisasi.",
)

// ErrEmailTakenInOrg dipakai saat mengundang email yang sudah terdaftar/terundang.
var ErrEmailTakenInOrg = shared.Conflict(
	"ALREADY_INVITED", "Email ini sudah diundang atau sudah menjadi anggota.",
)

// ErrInvalidRole dikembalikan untuk kode role yang tidak dikenal.
var ErrInvalidRole = shared.Validation(map[string]string{
	"role": "peran tidak dikenal.",
})
