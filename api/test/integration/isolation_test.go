// Package integration menguji jaminan isolasi tenant terhadap database nyata.
//
// Ini GATE Phase 2: "0 temuan cross-tenant". Uji di sini sengaja menyerang
// batas antar-organisasi dari sudut yang berbeda — akses langsung ke repository,
// bukan lewat UI — karena kebocoran tenant paling sering terjadi di query layer.
//
// Tes dilewati bila DATABASE_URL tidak diset (mis. `go test ./...` tanpa DB).
// CI menyediakan postgres sebagai service sehingga tes ini selalu berjalan di sana.
package integration

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/api/internal/application/organizationapp"
	"github.com/angkut/angkut/apps/api/internal/domain/organization"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/domain/user"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
)

// fixture menyiapkan dua organisasi terpisah, masing-masing dengan satu owner.
// Dua tenant — bukan satu — adalah syarat minimum untuk membuktikan isolasi.
type fixture struct {
	pool   *pgxpool.Pool
	stores *database.Stores
	tx     *database.TxManager

	userA, userB string
	orgA, orgB   string
	memberA      string
}

func setup(t *testing.T) *fixture {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL tidak diset — tes isolasi tenant dilewati")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("membuka pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	t.Cleanup(pool.Close)

	stores := database.NewStores(pool)
	f := &fixture{pool: pool, stores: stores, tx: database.NewTxManager(pool)}

	// Data unik per run supaya tes dapat dijalankan berulang pada DB yang sama.
	suffix := randomSuffix(t)

	f.userA = createUser(t, stores, "tenant-a-"+suffix+"@example.com", "Pemilik A")
	f.userB = createUser(t, stores, "tenant-b-"+suffix+"@example.com", "Pemilik B")

	f.orgA, f.memberA = createOrgWithOwner(t, f, f.userA, "Org A "+suffix)
	f.orgB, _ = createOrgWithOwner(t, f, f.userB, "Org B "+suffix)

	return f
}

func createUser(t *testing.T, stores *database.Stores, email, name string) string {
	t.Helper()
	u := user.NewUser(email, name)
	u.PasswordHash = "x" // tidak dipakai di tes ini
	if err := stores.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("membuat user %s: %v", email, err)
	}
	return u.ID
}

func createOrgWithOwner(t *testing.T, f *fixture, ownerUserID, name string) (orgID, memberID string) {
	t.Helper()
	org := organization.NewOrganization(name, ownerUserID)
	owner := organization.NewOwnerMember(org.ID, ownerUserID)
	ctx := context.Background()

	err := f.tx.InTx(ctx, func(q database.Querier) error {
		return f.stores.WithTx(q).Orgs().CreateWithOwner(ctx, org, owner)
	})
	if err != nil {
		t.Fatalf("membuat organisasi %s: %v", name, err)
	}
	return org.ID, owner.ID
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := randRead(b); err != nil {
		t.Fatalf("membaca acak: %v", err)
	}
	return hexEncode(b)
}

// ─── Tes isolasi ──────────────────────────────────────────────────────────

// User dari tenant B tidak boleh melihat organisasi tenant A, walau tahu ID-nya.
func TestListForUserNeverLeaksOtherTenants(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	listA, err := f.stores.Orgs().ListForUser(ctx, f.userA)
	if err != nil {
		t.Fatalf("ListForUser(A): %v", err)
	}
	for _, m := range listA {
		if m.Organization.ID == f.orgB {
			t.Fatal("KEBOCORAN TENANT: user A melihat organisasi milik user B")
		}
	}

	listB, err := f.stores.Orgs().ListForUser(ctx, f.userB)
	if err != nil {
		t.Fatalf("ListForUser(B): %v", err)
	}
	for _, m := range listB {
		if m.Organization.ID == f.orgA {
			t.Fatal("KEBOCORAN TENANT: user B melihat organisasi milik user A")
		}
	}
}

// Anggota tenant lain tidak boleh dianggap sebagai anggota, walau ID organisasi
// dan ID user-nya sama-sama diketahui.
func TestGetMemberRejectsForeignUser(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// User B bukan anggota org A.
	if _, err := f.stores.Orgs().GetMember(ctx, f.orgA, f.userB); err == nil {
		t.Fatal("KEBOCORAN TENANT: user B dianggap anggota org A")
	} else if !shared.Is(err, shared.ErrNotFound) {
		t.Fatalf("error harus NotFound agar keberadaan organisasi tidak bocor, dapat: %v", err)
	}

	// Pasangan yang benar tetap bekerja.
	if _, err := f.stores.Orgs().GetMember(ctx, f.orgA, f.userA); err != nil {
		t.Fatalf("user A harus dikenali sebagai anggota org A: %v", err)
	}
}

// Daftar anggota hanya memuat anggota organisasi itu.
func TestListMembersScopedToOrganization(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	members, err := f.stores.Orgs().ListMembers(ctx, f.orgA)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	for _, m := range members {
		if m.OrganizationID != f.orgA {
			t.Fatalf("KEBOCORAN TENANT: anggota org lain muncul di daftar org A (%s)", m.OrganizationID)
		}
		if m.UserID == f.userB {
			t.Fatal("KEBOCORAN TENANT: user B muncul di daftar anggota org A")
		}
	}
}

// Service menolak aktor dari tenant lain, bahkan ketika ia tahu ID organisasi
// target dan ID anggota target.
func TestServiceRejectsCrossTenantAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := organizationapp.New(organizationapp.Deps{
		DB: f.stores, Tx: f.tx, Audit: f.stores.Audit(),
	})

	// Baca organisasi orang lain.
	if _, _, err := svc.Get(ctx, f.orgA, f.userB); err == nil {
		t.Fatal("KEBOCORAN TENANT: user B dapat membaca org A lewat service")
	}

	// Baca daftar anggota organisasi orang lain.
	if _, err := svc.ListMembers(ctx, f.orgA, f.userB); err == nil {
		t.Fatal("KEBOCORAN TENANT: user B dapat membaca anggota org A")
	}

	// Ubah organisasi orang lain.
	name := "Direbut"
	if _, err := svc.Update(ctx, f.orgA, f.userB,
		organizationapp.UpdateProfileInput{Name: &name}, organizationapp.ContextInfo{}); err == nil {
		t.Fatal("KEBOCORAN TENANT: user B dapat mengubah org A")
	}

	// Hapus anggota organisasi orang lain.
	if err := svc.RemoveMember(ctx, f.orgA, f.userB, f.memberA, organizationapp.ContextInfo{}); err == nil {
		t.Fatal("KEBOCORAN TENANT: user B dapat menghapus anggota org A")
	}

	// Tambah anggota ke organisasi orang lain.
	if _, err := svc.Invite(ctx, f.orgA, f.userB,
		organizationapp.InviteInput{Email: "penyusup@example.com", Role: "admin"},
		organizationapp.ContextInfo{}); err == nil {
		t.Fatal("KEBOCORAN TENANT: user B dapat menambah anggota ke org A")
	}

	// Pastikan org A tidak berubah setelah semua percobaan di atas.
	org, err := f.stores.Orgs().GetByID(ctx, f.orgA)
	if err != nil {
		t.Fatalf("GetByID(orgA): %v", err)
	}
	if org.Name == "Direbut" {
		t.Fatal("KEBOCORAN TENANT: nama org A berhasil diubah pihak luar")
	}
}

// Endpoint berbasis id tidak boleh menjadi oracle keberadaan data orang lain:
// jawaban untuk "bukan milik saya" harus sama dengan "tidak ada".
func TestErrorDoesNotRevealExistence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := organizationapp.New(organizationapp.Deps{
		DB: f.stores, Tx: f.tx, Audit: f.stores.Audit(),
	})

	_, _, errForeign := svc.Get(ctx, f.orgA, f.userB)

	missing := "01a11ea1-0000-7000-8000-00000000dead"
	_, _, errMissing := svc.Get(ctx, missing, f.userA)

	if errForeign == nil || errMissing == nil {
		t.Fatal("kedua kasus seharusnya gagal")
	}

	var ef, em *shared.Error
	if !asShared(errForeign, &ef) || !asShared(errMissing, &em) {
		t.Fatalf("error harus typed error: %v / %v", errForeign, errMissing)
	}
	if ef.Code != em.Code || ef.Message != em.Message {
		t.Fatalf("kode/pesan berbeda (bocor keberadaan): luar=%s/%s tidak-ada=%s/%s",
			ef.Code, ef.Message, em.Code, em.Message)
	}
}

// Invariant: organisasi tidak boleh kehilangan owner aktif terakhirnya.
func TestLastOwnerIsProtected(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := organizationapp.New(organizationapp.Deps{
		DB: f.stores, Tx: f.tx, Audit: f.stores.Audit(),
	})

	owners, err := f.stores.Orgs().CountActiveOwners(ctx, f.orgA)
	if err != nil {
		t.Fatalf("CountActiveOwners: %v", err)
	}
	if owners != 1 {
		t.Fatalf("fixture harus punya tepat 1 owner, dapat %d", owners)
	}

	// Owner tidak dapat menghapus dirinya sendiri bila ia satu-satunya owner.
	if err := svc.RemoveMember(ctx, f.orgA, f.userA, f.memberA, organizationapp.ContextInfo{}); err == nil {
		t.Fatal("owner terakhir seharusnya tidak dapat dihapus")
	}

	// Owner juga tidak dapat menurunkan dirinya sendiri menjadi viewer.
	viewer := "viewer"
	if err := svc.UpdateMember(ctx, f.orgA, f.userA, f.memberA,
		organizationapp.UpdateMemberInput{Role: &viewer}, organizationapp.ContextInfo{}); err == nil {
		t.Fatal("owner terakhir seharusnya tidak dapat diturunkan")
	}

	if n, _ := f.stores.Orgs().CountActiveOwners(ctx, f.orgA); n != 1 {
		t.Fatalf("jumlah owner berubah menjadi %d", n)
	}
}

// Role setara/lebih tinggi tidak dapat dikelola — mencegah eskalasi wewenang.
func TestCannotManageEqualOrHigherRole(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := organizationapp.New(organizationapp.Deps{
		DB: f.stores, Tx: f.tx, Audit: f.stores.Audit(),
	})

	// Tambahkan admin ke org A.
	res, err := svc.Invite(ctx, f.orgA, f.userA,
		organizationapp.InviteInput{Email: emailOf(t, f.stores, f.userB), Role: "admin"},
		organizationapp.ContextInfo{})
	if err != nil {
		t.Fatalf("Invite admin: %v", err)
	}

	// Admin tidak boleh mengangkat admin lain (setara).
	if _, err := svc.Invite(ctx, f.orgA, f.userB,
		organizationapp.InviteInput{Email: emailOf(t, f.stores, f.userA), Role: "admin"},
		organizationapp.ContextInfo{}); err == nil {
		t.Fatal("admin seharusnya tidak dapat mengangkat role setara")
	}

	// Admin tidak boleh menurunkan owner.
	viewer := "viewer"
	if err := svc.UpdateMember(ctx, f.orgA, f.userB, f.memberA,
		organizationapp.UpdateMemberInput{Role: &viewer}, organizationapp.ContextInfo{}); err == nil {
		t.Fatal("admin seharusnya tidak dapat menurunkan owner")
	}

	_ = res
}

// ─── helper ───────────────────────────────────────────────────────────────

func emailOf(t *testing.T, stores *database.Stores, userID string) string {
	t.Helper()
	u, err := stores.Users().GetByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetByID(%s): %v", userID, err)
	}
	return u.Email
}

func asShared(err error, target **shared.Error) bool {
	if e, ok := err.(*shared.Error); ok {
		*target = e
		return true
	}
	return false
}
