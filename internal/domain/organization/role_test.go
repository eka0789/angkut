package organization

import "testing"

// Hierarki role adalah fondasi otorisasi. Kalau tabel ini salah, admin bisa
// mengangkat dirinya sendiri menjadi owner atau menurunkan atasannya.
func TestRoleHierarchy(t *testing.T) {
	cases := []struct {
		actor     RoleCode
		target    RoleCode
		canManage bool
	}{
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleOwner, false}, // owner tidak mengelola owner lain
		{RoleAdmin, RoleStaff, true},
		{RoleAdmin, RoleAdmin, false}, // setara tidak boleh
		{RoleAdmin, RoleOwner, false}, // tidak boleh menurunkan owner
		{RoleStaff, RoleViewer, true},
		{RoleViewer, RoleStaff, false},     // pengamat tidak mengelola apa pun
		{RoleFinance, RoleWarehouse, true}, // 70 > 60
		{RoleWarehouse, RoleFinance, false},
	}
	for _, tc := range cases {
		got := tc.actor.CanManage(tc.target)
		if got != tc.canManage {
			t.Errorf("%s.CanManage(%s) = %v, harap %v", tc.actor, tc.target, got, tc.canManage)
		}
	}
}

func TestRoleLevelOrdering(t *testing.T) {
	roles := AllRoles()
	for i := 1; i < len(roles); i++ {
		if roles[i-1].Level() <= roles[i].Level() {
			t.Errorf("urutan role salah: %s (%d) harus lebih tinggi dari %s (%d)",
				roles[i-1], roles[i-1].Level(), roles[i], roles[i].Level())
		}
	}
}

func TestInvalidRoleHasNoPower(t *testing.T) {
	var unknown RoleCode = "superadmin"
	if unknown.Valid() {
		t.Error("role tak dikenal seharusnya tidak valid")
	}
	if unknown.CanManage(RoleViewer) {
		t.Error("role tak dikenal tidak boleh dapat mengelola apa pun")
	}
	if unknown.Can("shipment.manage") {
		t.Error("role tak dikenal tidak boleh punya izin")
	}
}

func TestPermissionMatrix(t *testing.T) {
	// Owner memegang seluruh izin.
	if !RoleOwner.Can("apa pun") {
		t.Error("owner harus lolos untuk izin apa pun")
	}
	// Viewer hanya boleh membaca.
	if RoleViewer.Can("shipment.manage") {
		t.Error("viewer tidak boleh mengelola pengiriman")
	}
	if !RoleViewer.Can("shipment.view") {
		t.Error("viewer harus boleh melihat pengiriman")
	}
	// Finance mengurus uang, bukan stok.
	if !RoleFinance.Can("cod.settle") {
		t.Error("finance harus boleh menyelesaikan COD")
	}
	if RoleFinance.Can("inventory.manage") {
		t.Error("finance tidak boleh mengelola stok")
	}
	// Warehouse mengurus stok, bukan uang.
	if !RoleWarehouse.Can("inventory.manage") {
		t.Error("warehouse harus boleh mengelola stok")
	}
	if RoleWarehouse.Can("cod.settle") {
		t.Error("warehouse tidak boleh menyelesaikan COD")
	}
}

func TestEveryRoleHasPermissions(t *testing.T) {
	for _, r := range AllRoles() {
		if len(PermissionsFor(r)) == 0 {
			t.Errorf("role %s tidak punya izin — kemungkinan lupa diisi di matrix", r)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Toko Budi":        "toko-budi",
		"UD Sumber Rejeki": "ud-sumber-rejeki",
		"  Spasi  Ganda  ": "spasi-ganda",
		"Toko & Budi!!!":   "toko-budi",
		"Toko_Budi.Online": "toko-budi-online",
		"123":              "123",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, harap %q", in, got, want)
		}
	}
}

func TestSlugifyAlwaysValid(t *testing.T) {
	// Nama non-ASCII tidak boleh menghasilkan slug kosong atau tidak valid.
	// String non-ASCII di bawah ("日本") sengaja dipakai sebagai contoh input
	// yang seluruhnya bukan huruf Latin — pengguna dapat menempelkan teks apa
	// pun ke kolom nama, dan slug tetap harus valid.
	for _, in := range []string{"", "!!!", "日本", "a", "---"} {
		got := Slugify(in)
		if !slugPattern.MatchString(got) {
			t.Errorf("Slugify(%q) = %q tidak memenuhi pola slug", in, got)
		}
	}
}

func TestValidateOrganizationName(t *testing.T) {
	if err := ValidateName("Toko Budi"); err != nil {
		t.Errorf("nama wajar ditolak: %v", err)
	}
	if err := ValidateName("x"); err == nil {
		t.Error("nama terlalu pendek seharusnya ditolak")
	}
	if err := ValidateName(""); err == nil {
		t.Error("nama kosong seharusnya ditolak")
	}
}
