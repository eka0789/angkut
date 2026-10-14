package database

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// Stores adalah factory repository. Semua repository yang menyentuh tabel
// terkait diambil dari sini, sehingga satu tempat menentukan querier mana yang
// dipakai (pool langsung, atau transaksi).
//
// Pola pakai:
//
//	st := database.NewStores(pool)
//	st.Users().GetByID(ctx, id)              // di luar transaksi
//	tx.InTx(ctx, func(q database.Querier) error {
//	    st := st.WithTx(q)                   // di dalam transaksi
//	    return st.Users().Create(ctx, u)
//	})
type Stores struct {
	q Querier
}

// NewStores membuat factory yang membaca/menulis langsung ke pool.
func NewStores(pool *pgxpool.Pool) *Stores { return &Stores{q: pool} }

// NewStoresWith membuat factory atas querier apa pun (dipakai test).
func NewStoresWith(q Querier) *Stores { return &Stores{q: q} }

// WithTx mengembalikan factory yang seluruh repository-nya berjalan di dalam
// transaksi tertentu. Repository yang dipakai di dalam InTx WAJIB berasal dari
// sini agar perubahan ikut ter-commit/rollback bersama.
func (s *Stores) WithTx(q Querier) *Stores { return &Stores{q: q} }

// Users mengembalikan repository pengguna.
func (s *Stores) Users() *UserRepo { return &UserRepo{q: s.q} }

// Sessions mengembalikan repository sesi perangkat.
func (s *Stores) Sessions() *SessionRepo { return &SessionRepo{q: s.q} }

// Tokens mengembalikan repository token sekali-pakai.
func (s *Stores) Tokens() *TokenRepo { return &TokenRepo{q: s.q} }

// OAuth mengembalikan repository identitas OAuth.
func (s *Stores) OAuth() *OAuthRepo { return &OAuthRepo{q: s.q} }

// Orgs mengembalikan repository organisasi & keanggotaan.
func (s *Stores) Orgs() *OrganizationRepo { return &OrganizationRepo{q: s.q} }

// Audit mengembalikan repository audit.
func (s *Stores) Audit() *AuditRepo { return &AuditRepo{q: s.q} }

// Regions mengembalikan repository referensi wilayah publik.
func (s *Stores) Regions() *RegionRepo { return &RegionRepo{q: s.q} }

// Addresses mengembalikan repository alamat tenant-scoped.
func (s *Stores) Addresses() *AddressRepo { return &AddressRepo{q: s.q} }

// Customers mengembalikan repository pelanggan tenant-scoped.
func (s *Stores) Customers() *CustomerRepo { return &CustomerRepo{q: s.q} }

// Couriers mengembalikan repository registry kurir (platform-level).
func (s *Stores) Couriers() *CourierRepo { return &CourierRepo{q: s.q} }

// Stores mengembalikan repository kanal penjualan tenant-scoped.
func (s *Stores) StoresRepo() *StoreRepo { return &StoreRepo{q: s.q} }

// Warehouses mengembalikan repository gudang tenant-scoped.
func (s *Stores) Warehouses() *WarehouseRepo { return &WarehouseRepo{q: s.q} }

// Orders mengembalikan repository order tenant-scoped.
func (s *Stores) Orders() *OrderRepo { return &OrderRepo{q: s.q} }

// Shipments mengembalikan repository pengiriman tenant-scoped.
func (s *Stores) Shipments() *ShipmentRepo { return &ShipmentRepo{q: s.q} }

// Querier mengembalikan querier aktif (pool atau tx).
func (s *Stores) Querier() Querier { return s.q }
