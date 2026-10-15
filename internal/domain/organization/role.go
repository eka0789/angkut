package organization

import (
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// RoleCode adalah kode role yang dikenal sistem. Nilainya HARUS sama dengan
// seed di migrasi 0003 — perbandingan dilakukan lewat konstanta ini, bukan
// string literal yang tersebar di handler.
type RoleCode string

const (
	RoleOwner     RoleCode = "owner"
	RoleAdmin     RoleCode = "admin"
	RoleFinance   RoleCode = "finance"
	RoleWarehouse RoleCode = "warehouse"
	RoleStaff     RoleCode = "staff"
	RoleViewer    RoleCode = "viewer"
)

// RoleID adalah UUID role hasil seed migrasi 0003. Dipakai saat membuat
// keanggotaan owner tanpa perlu query tambahan.
const (
	RoleOwnerID     = "01920000-0000-7000-8000-000000000001"
	RoleAdminID     = "01920000-0000-7000-8000-000000000002"
	RoleFinanceID   = "01920000-0000-7000-8000-000000000003"
	RoleWarehouseID = "01920000-0000-7000-8000-000000000004"
	RoleStaffID     = "01920000-0000-7000-8000-000000000005"
	RoleViewerID    = "01920000-0000-7000-8000-000000000006"
)

// Level mengembalikan peringkat kekuasaan role. Dipakai untuk aturan
// "tidak boleh mengubah/menghapus pemegang role yang lebih tinggi".
func (r RoleCode) Level() int {
	switch r {
	case RoleOwner:
		return 100
	case RoleAdmin:
		return 90
	case RoleFinance:
		return 70
	case RoleWarehouse:
		return 60
	case RoleStaff:
		return 50
	case RoleViewer:
		return 10
	default:
		return 0
	}
}

// Valid melaporkan apakah kode role dikenal.
func (r RoleCode) Valid() bool { return r.Level() > 0 }

// AllRoles mengembalikan daftar role dari yang paling berkuasa.
func AllRoles() []RoleCode {
	return []RoleCode{RoleOwner, RoleAdmin, RoleFinance, RoleWarehouse, RoleStaff, RoleViewer}
}

// CanManage melaporkan apakah actor boleh mengelola target, berdasarkan
// hierarki. Aturan inti RBAC organisasi:
//   - tidak boleh mengelola anggota dengan level >= level sendiri,
//   - owner tidak dapat diturunkan/dihapus oleh siapa pun selain owner lain.
func (r RoleCode) CanManage(target RoleCode) bool {
	if !r.Valid() || !target.Valid() {
		return false
	}
	return r.Level() > target.Level()
}

// ErrLastOwner melindungi invariant "organisasi selalu punya owner".
var ErrLastOwner = shared.Conflict(
	"LAST_OWNER",
	"Organisasi harus memiliki minimal satu owner. Ubah owner lain terlebih dahulu.",
)

// ErrCannotManageRole dikembalikan saat actor mencoba mengelola anggota dengan
// level setara atau lebih tinggi.
var ErrCannotManageRole = shared.New(
	"CANNOT_MANAGE_ROLE",
	"Anda tidak dapat mengelola anggota dengan peran setara atau lebih tinggi dari Anda.",
	shared.ErrForbidden,
)

// Permission adalah izin granular (mis. "shipment.create").
type Permission struct {
	ID          string
	Code        string
	Module      string
	Description string
}

// PermissionMatrix memetakan role ke izin. Satu tempat, bukan tersebar.
//
// Catatan: middleware tetap memeriksa ini di server; FE hanya memakainya untuk
// menyembunyikan UI. Server selalu otoritatif.
var PermissionMatrix = map[RoleCode][]string{
	// Owner: seluruh izin modul.
	RoleOwner: {"*"},

	RoleAdmin: {
		"organization.manage", "member.manage", "role.view",
		"shipment.manage", "order.manage", "customer.manage",
		"product.manage", "inventory.manage", "warehouse.manage",
		"store.manage", "return.manage", "notification.manage",
		"automation.manage", "analytics.view", "cod.view",
		"ai.use", "apikey.manage",
		"store.view", "warehouse.view",
	},

	RoleFinance: {
		"cod.view", "cod.settle", "cod.export",
		"payment.manage", "invoice.view", "analytics.view",
		"shipment.view", "order.view", "return.refund", "ai.use",
		"store.view", "warehouse.view",
	},

	RoleWarehouse: {
		"inventory.manage", "product.manage", "warehouse.manage",
		"shipment.manage", "shipment.pickup", "return.receive", "return.inspect",
		"order.view", "shipment.view", "ai.use",
		"store.view", "warehouse.view",
	},

	RoleStaff: {
		"shipment.manage", "shipment.view", "order.manage", "order.view",
		"customer.manage", "customer.view", "tracking.view",
		"analytics.view", "ai.use",
		"store.view", "warehouse.view",
	},

	RoleViewer: {
		"shipment.view", "order.view", "customer.view", "analytics.view",
		"inventory.view", "cod.view", "tracking.view",
		"store.view", "warehouse.view",
	},
}

// Can melaporkan apakah role memiliki izin tertentu.
func (r RoleCode) Can(permission string) bool {
	perms, ok := PermissionMatrix[r]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == "*" || p == permission {
			return true
		}
	}
	return false
}

// PermissionsFor mengembalikan daftar izin efektif sebuah role.
func PermissionsFor(r RoleCode) []string {
	if perms, ok := PermissionMatrix[r]; ok {
		return perms
	}
	return nil
}
