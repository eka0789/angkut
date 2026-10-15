// Package store berisi entitas kanal penjualan (store) dan gudang
// (warehouse) tenant-scoped. Keduanya adalah prasyarat order (TASK-026) dan
// shipment (TASK-027): order berasal dari store, shipment berangkat dari gudang.
package store

import (
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Channel adalah kanal penjualan. Nilai HARUS sama dengan CHECK di migrasi 0014.
type Channel string

const (
	ChannelManual      Channel = "manual"
	ChannelShopify     Channel = "shopify"
	ChannelWooCommerce Channel = "woocommerce"
	ChannelTokopedia   Channel = "tokopedia"
	ChannelShopee      Channel = "shopee"
	ChannelTikTokShop  Channel = "tiktok_shop"
	ChannelCustom      Channel = "custom"
)

// AllChannels adalah kanal yang dikenal (untuk validasi & filter).
var AllChannels = []Channel{
	ChannelManual, ChannelShopify, ChannelWooCommerce,
	ChannelTokopedia, ChannelShopee, ChannelTikTokShop, ChannelCustom,
}

// Valid melaporkan apakah kanal dikenal.
func (c Channel) Valid() bool {
	for _, v := range AllChannels {
		if c == v {
			return true
		}
	}
	return false
}

// Store adalah kanal penjualan tenant. Kredensial adapter TIDAK pernah masuk
// struct ini — hanya flag HasCredentials; nilai rahasianya dibuka adapter.
type Store struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Channel        Channel   `json:"channel"`
	AdapterType    string    `json:"adapter_type"`
	HasCredentials bool      `json:"has_credentials"`
	WarehouseID    string    `json:"warehouse_id,omitempty"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate memeriksa invariant store.
func (s *Store) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(s.Name) == "" {
		fields["name"] = "Nama toko wajib diisi."
	}
	if !s.Channel.Valid() {
		fields["channel"] = "Kanal tidak dikenal."
	}
	if strings.TrimSpace(s.AdapterType) == "" {
		fields["adapter_type"] = "Tipe adapter wajib diisi."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Warehouse adalah lokasi fulfillment tenant; alamatnya menjadi origin shipment.
type Warehouse struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	AddressID      string    `json:"address_id,omitempty"`
	Capacity       *int      `json:"capacity,omitempty"`
	IsDefault      bool      `json:"is_default"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate memeriksa invariant gudang.
func (w *Warehouse) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(w.Name) == "" {
		fields["name"] = "Nama gudang wajib diisi."
	}
	if w.Capacity != nil && *w.Capacity <= 0 {
		fields["capacity"] = "Kapasitas harus lebih dari 0 bila diisi."
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// Sentinel error.
var (
	ErrNotFound              = shared.NotFound("STORE_NOT_FOUND", "Toko tidak ditemukan.")
	ErrWarehouseNotFound     = shared.NotFound("WAREHOUSE_NOT_FOUND", "Gudang tidak ditemukan.")
	ErrWarehouseDefaultTaken = shared.Conflict("WAREHOUSE_DEFAULT_TAKEN", "Sudah ada gudang default di organisasi ini. Nonaktifkan yang lama terlebih dahulu.")
)
