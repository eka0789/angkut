package address

import "context"

// Repository mendefinisikan operasi CRUD alamat tenant-scoped.
// PRINSIP ISOLASI (ADR-008): semua query wajib filter organization_id dari context.
type Repository interface {
	Create(ctx context.Context, a *Address) error
	GetByID(ctx context.Context, orgID, id string) (*Address, error)
	Update(ctx context.Context, a *Address) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, status Status) ([]Address, error)
	Search(ctx context.Context, orgID, query string, limit int) ([]Address, error)

	// UnsetDefaultsExcept menghapus flag is_default dari semua alamat org kecuali satu.
	// Dipakai saat menetapkan alamat baru sebagai default.
	UnsetDefaultsExcept(ctx context.Context, orgID, exceptID string) error
}
