// Package inventoryapp berisi logika aplikasi stok (TASK-037/038): katalog
// produk/varian, penyesuaian saldo berjejak, reservasi, laporan low-stock,
// dan transfer antar gudang yang atomik.
//
// Aturan yang dipegang:
//   - satu-satunya angka yang boleh dijanjikan adalah available =
//     on_hand − reserved;
//   - setiap perubahan saldo menulis movement (append-only) DI DALAM
//     transaksi yang sama — tidak ada saldo berubah tanpa jejak;
//   - transfer yang gagal di salah satu item ditolak SELURUHNYA (atomik);
//   - optimistic lock version: bentrok penulis paralel = 409, bukan retry
//     diam-diam (aturan CODING_RULES).
package inventoryapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/inventory"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps adalah dependensi service inventory.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Service adalah service aplikasi inventory.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	audit  audit.Recorder
	log    *slog.Logger
}

// New membuat service inventory.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, audit: d.Audit, log: log}
}

// ── Produk & varian ─────────────────────────────────────────────────────

// ProductInput adalah permintaan membuat/mengubah produk.
type ProductInput struct {
	Name     string
	Category string
	WeightKG *float64
	LengthCM *float64
	WidthCM  *float64
	HeightCM *float64
	Actor    string
}

// CreateProduct membuat produk (tanpa varian; varian ditambah terpisah).
func (s *Service) CreateProduct(ctx context.Context, orgID, requestID string, in ProductInput) (*inventory.Product, error) {
	p := &inventory.Product{
		ID: platform.NewUUID(), OrganizationID: orgID,
		Name: strings.TrimSpace(in.Name), Category: strings.TrimSpace(in.Category),
		WeightKG: in.WeightKG, LengthCM: in.LengthCM, WidthCM: in.WidthCM, HeightCM: in.HeightCM,
		IsActive: true, CreatedBy: in.Actor,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.stores.Products().Create(ctx, p); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionProductCreated, p.ID)
	return p, nil
}

// GetProduct mengambil produk beserta variannya.
func (s *Service) GetProduct(ctx context.Context, orgID, id string) (*inventory.Product, error) {
	return s.stores.Products().GetByID(ctx, orgID, id)
}

// ListFilter adalah filter daftar produk dari endpoint.
type ListFilter struct {
	Search   string
	Category string
	LowStock bool
	Limit    int
	Offset   int
}

// ListProducts mengembalikan halaman produk.
func (s *Service) ListProducts(ctx context.Context, orgID string, f ListFilter) ([]inventory.Product, int, error) {
	return s.stores.Products().List(ctx, orgID, inventory.ProductFilter{
		Search: f.Search, Category: f.Category, LowStock: f.LowStock,
		Limit: f.Limit, Offset: f.Offset,
	})
}

// UpdateProduct mengubah data produk.
func (s *Service) UpdateProduct(ctx context.Context, orgID, id, requestID string, in ProductInput) (*inventory.Product, error) {
	p, err := s.stores.Products().GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	p.Name = strings.TrimSpace(in.Name)
	p.Category = strings.TrimSpace(in.Category)
	p.WeightKG, p.LengthCM, p.WidthCM, p.HeightCM = in.WeightKG, in.LengthCM, in.WidthCM, in.HeightCM
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.stores.Products().Update(ctx, p); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionProductUpdated, p.ID)
	return p, nil
}

// ArchiveProduct mengarsipkan produk (soft delete; DELETE endpoint).
func (s *Service) ArchiveProduct(ctx context.Context, orgID, id, actor, requestID string) error {
	if err := s.stores.Products().SetActive(ctx, orgID, id, false); err != nil {
		return err
	}
	s.recordAudit(ctx, orgID, actor, requestID, audit.ActionProductArchived, id)
	return nil
}

// VariantInput adalah permintaan menambah/mengubah varian.
type VariantInput struct {
	SKU               string
	Name              string
	WeightKG          *float64
	LengthCM          *float64
	WidthCM           *float64
	HeightCM          *float64
	LowStockThreshold int
	Actor             string
}

// AddVariant menambah SKU pada produk.
func (s *Service) AddVariant(ctx context.Context, orgID, productID, requestID string, in VariantInput) (*inventory.Variant, error) {
	// Produk harus ada & milik org (FK tidak cukup untuk pesan yang jelas).
	if _, err := s.stores.Products().GetByID(ctx, orgID, productID); err != nil {
		return nil, err
	}
	v := &inventory.Variant{
		ID: platform.NewUUID(), OrganizationID: orgID, ProductID: productID,
		SKU: strings.TrimSpace(in.SKU), Name: strings.TrimSpace(in.Name),
		WeightKG: in.WeightKG, LengthCM: in.LengthCM, WidthCM: in.WidthCM, HeightCM: in.HeightCM,
		LowStockThreshold: in.LowStockThreshold, IsActive: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if v.LowStockThreshold == 0 {
		v.LowStockThreshold = 5 // default bernaung di domain; 0 = "low pada 0"
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if err := s.stores.Products().AddVariant(ctx, v); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionProductVariantAdded, v.ID)
	return v, nil
}

// UpdateVariant mengubah data varian (bukan saldo — saldo hanya lewat
// adjust/transfer/reserve agar semua perubahan berjejak).
func (s *Service) UpdateVariant(ctx context.Context, orgID, variantID, requestID string, in VariantInput) (*inventory.Variant, error) {
	// Repo UpdateVariant memfilter organization_id — varian org lain tidak
	// akan pernah tersentuh (0 baris → NotFound).
	v := &inventory.Variant{
		ID: variantID, OrganizationID: orgID,
		SKU: strings.TrimSpace(in.SKU), Name: strings.TrimSpace(in.Name),
		WeightKG: in.WeightKG, LengthCM: in.LengthCM, WidthCM: in.WidthCM, HeightCM: in.HeightCM,
		LowStockThreshold: in.LowStockThreshold, IsActive: true,
		UpdatedAt: time.Now().UTC(),
	}
	if v.LowStockThreshold == 0 {
		v.LowStockThreshold = 5
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if err := s.stores.Products().UpdateVariant(ctx, v); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionProductVariantUpdated, v.ID)
	return v, nil
}

// ── Saldo & movement ────────────────────────────────────────────────────

// AdjustInput adalah permintaan penyesuaian saldo (bisa + atau −).
type AdjustInput struct {
	VariantID   string
	WarehouseID string
	QtyDelta    int
	Reason      string
	Actor       string
}

// Adjust menyesuaikan saldo on_hand dan menulis movement. QtyDelta negatif
// tidak boleh membuat saldo di bawah reserved (pesanan yang sudah direservasi
// tidak boleh terkuras oleh koreksi).
func (s *Service) Adjust(ctx context.Context, orgID, requestID string, in AdjustInput) (*inventory.Stock, error) {
	if in.QtyDelta == 0 {
		return nil, shared.Validation(map[string]string{"qty_delta": "Penyesuaian tidak boleh 0."})
	}
	if err := validateIDs(orgID, in.VariantID, in.WarehouseID); err != nil {
		return nil, err
	}
	var out *inventory.Stock
	err := s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)
		bal, err := st.Inventory().UpsertBalance(ctx, orgID, in.VariantID, in.WarehouseID)
		if err != nil {
			return err
		}
		newOnHand := bal.QtyOnHand + in.QtyDelta
		if newOnHand < 0 {
			return shared.Validation(map[string]string{
				"qty_delta": "Penyesuaian membuat saldo negatif.",
			})
		}
		if newOnHand < bal.QtyReserved {
			return shared.Validation(map[string]string{
				"qty_delta": "Saldo tidak boleh turun di bawah jumlah yang sudah direservasi.",
			})
		}
		bal.QtyOnHand = newOnHand
		if err := st.Inventory().SaveBalance(ctx, bal); err != nil {
			return err
		}
		if err := st.Inventory().InsertMovement(ctx, &inventory.Movement{
			ID: platform.NewUUID(), OrganizationID: orgID,
			VariantID: in.VariantID, WarehouseID: in.WarehouseID,
			Type: inventory.MovementAdjust, QtyDelta: in.QtyDelta, QtyAfter: bal.QtyOnHand,
			Reason: strings.TrimSpace(in.Reason), CreatedBy: in.Actor,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		out = bal
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionInventoryAdjusted, in.VariantID)
	s.log.Info("saldo stok disesuaikan", "org", orgID, "variant", in.VariantID,
		"warehouse", in.WarehouseID, "delta", in.QtyDelta, "after", out.QtyOnHand)
	return out, nil
}

// ListStock mengembalikan saldo + keterangan varian/produk.
func (s *Service) ListStock(ctx context.Context, orgID string, f StockFilter) ([]inventory.StockRow, int, error) {
	return s.stores.Inventory().ListStock(ctx, orgID, inventory.StockFilter{
		WarehouseID: f.WarehouseID, VariantID: f.VariantID, Search: f.Search,
		Limit: f.Limit, Offset: f.Offset,
	})
}

// MovementFilter adalah filter riwayat movement dari endpoint.
type MovementFilter struct {
	VariantID   string
	WarehouseID string
	Type        string
	Limit       int
	Offset      int
}

// Movements mengembalikan riwayat movement.
func (s *Service) Movements(ctx context.Context, orgID string, f MovementFilter) ([]inventory.Movement, int, error) {
	mf := inventory.MovementFilter{
		VariantID: f.VariantID, WarehouseID: f.WarehouseID,
		Limit: f.Limit, Offset: f.Offset,
	}
	if t := strings.TrimSpace(f.Type); t != "" {
		mt := inventory.MovementType(t)
		if !mt.Valid() {
			return nil, 0, shared.Validation(map[string]string{
				"type": "Jenis movement harus adjust, transfer_out, transfer_in, reserve, atau release.",
			})
		}
		mf.Type = mt
	}
	return s.stores.Inventory().ListMovements(ctx, orgID, mf)
}

// LowStock mengembalikan laporan low-stock.
func (s *Service) LowStock(ctx context.Context, orgID, warehouseID string) ([]inventory.LowStockRow, error) {
	return s.stores.Inventory().LowStock(ctx, orgID, strings.TrimSpace(warehouseID))
}

// ── Reservasi (port internal; dipakai modul shipment nanti) ─────────────

// Reserve memesan stok untuk keperluan tertentu (mis. pengiriman): on_hand
// tetap, reserved naik, available turun.
func (s *Service) Reserve(ctx context.Context, orgID, variantID, warehouseID string, qty int, reason, actor string) error {
	return s.mutateReserved(ctx, orgID, variantID, warehouseID, qty, reason, actor, true)
}

// Release melepaskan reservasi.
func (s *Service) Release(ctx context.Context, orgID, variantID, warehouseID string, qty int, reason, actor string) error {
	return s.mutateReserved(ctx, orgID, variantID, warehouseID, qty, reason, actor, false)
}

func (s *Service) mutateReserved(ctx context.Context, orgID, variantID, warehouseID string, qty int, reason, actor string, reserve bool) error {
	if qty <= 0 {
		return shared.Validation(map[string]string{"qty": "Jumlah harus lebih dari 0."})
	}
	mt := inventory.MovementRelease
	if reserve {
		mt = inventory.MovementReserve
	}
	return s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)
		bal, err := st.Inventory().UpsertBalance(ctx, orgID, variantID, warehouseID)
		if err != nil {
			return err
		}
		delta := -qty
		if reserve {
			delta = qty
		}
		newReserved := bal.QtyReserved + delta
		if newReserved < 0 {
			return shared.Validation(map[string]string{"qty": "Reservasi yang dilepas melebihi yang ada."})
		}
		if newReserved > bal.QtyOnHand {
			return shared.Conflict("INVENTORY_INSUFFICIENT",
				"Stok tersedia tidak cukup untuk reservasi ini.")
		}
		bal.QtyReserved = newReserved
		if err := st.Inventory().SaveBalance(ctx, bal); err != nil {
			return err
		}
		return st.Inventory().InsertMovement(ctx, &inventory.Movement{
			ID: platform.NewUUID(), OrganizationID: orgID,
			VariantID: variantID, WarehouseID: warehouseID,
			Type: mt, QtyDelta: delta, QtyAfter: bal.QtyReserved,
			Reason: reason, CreatedBy: actor, CreatedAt: time.Now().UTC(),
		})
	})
}

// ── Transfer antar gudang (TASK-038) ────────────────────────────────────

// TransferInput adalah permintaan transfer stok.
type TransferInput struct {
	FromWarehouse string
	ToWarehouse   string
	Notes         string
	Items         []inventory.TransferItem
	Actor         string
}

// Transfer memindahkan stok antar dua gudang organisasi secara atomik:
// saldo asal −, saldo tujuan +, dua movement tercatat, transfer tersimpan.
// Satu item gagal (saldo kurang / varian bukan milik org) = seluruh transfer
// dibatalkan — tidak ada transfer setengah jalan.
func (s *Service) Transfer(ctx context.Context, orgID, requestID string, in TransferInput) (*inventory.StockTransfer, error) {
	// Dedup + validasi bentuk sebelum transaksi (aturan bulkapp: input tak
	// dipercaya; varian duplikat membuat perbandingan saldo keliru).
	items, err := normalizeTransferItems(in.Items)
	if err != nil {
		return nil, err
	}
	in.Items = items
	in.FromWarehouse = strings.TrimSpace(in.FromWarehouse)
	in.ToWarehouse = strings.TrimSpace(in.ToWarehouse)
	if err := inventory.ValidateTransfer(inventory.TransferInput{
		OrganizationID: orgID, FromWarehouse: in.FromWarehouse,
		ToWarehouse: in.ToWarehouse, Items: in.Items,
	}); err != nil {
		return nil, err
	}

	// Semua varian harus milik organisasi — dicek sekali di luar transaksi
	// agar pesan 422 jelas (di dalam tx, guard tenant tetap dijalankan lagi).
	for _, it := range in.Items {
		ok, err := s.stores.Products().VariantExists(ctx, orgID, it.VariantID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, shared.Validation(map[string]string{
				"items": "Ada varian yang tidak ditemukan pada organisasi ini.",
			})
		}
	}

	t := &inventory.StockTransfer{
		ID: platform.NewUUID(), OrganizationID: orgID,
		FromWarehouse: in.FromWarehouse, ToWarehouse: in.ToWarehouse,
		Notes: strings.TrimSpace(in.Notes), CreatedBy: in.Actor,
		Items: in.Items, CreatedAt: time.Now().UTC(),
	}
	now := time.Now().UTC()

	err = s.tx.InTx(ctx, func(q database.Querier) error {
		st := s.stores.WithTx(q)

		// Kunci kedua saldo sebelum mengubah apa pun (urut id agar dua
		// transfer paralel arah berlawanan tidak deadlock).
		src := make([]*inventory.Stock, 0, len(in.Items))
		dst := make([]*inventory.Stock, 0, len(in.Items))
		for _, it := range in.Items {
			sb, err := st.Inventory().UpsertBalance(ctx, orgID, it.VariantID, in.FromWarehouse)
			if err != nil {
				return err
			}
			db, err := st.Inventory().UpsertBalance(ctx, orgID, it.VariantID, in.ToWarehouse)
			if err != nil {
				return err
			}
			src = append(src, sb)
			dst = append(dst, db)
		}

		// Validasi ketersediaan SEMUA item sebelum menulis (atomik).
		for i, it := range in.Items {
			if avail := src[i].QtyOnHand - src[i].QtyReserved; avail < it.Qty {
				return shared.Validation(map[string]string{
					"items": "Stok tersedia di gudang asal tidak cukup untuk " + it.VariantID + ".",
				})
			}
		}

		// Tulis: sumber keluar, tujuan masuk, dua movement per item.
		for i, it := range in.Items {
			src[i].QtyOnHand -= it.Qty
			if err := st.Inventory().SaveBalance(ctx, src[i]); err != nil {
				return err
			}
			if err := st.Inventory().InsertMovement(ctx, &inventory.Movement{
				ID: platform.NewUUID(), OrganizationID: orgID,
				VariantID: it.VariantID, WarehouseID: in.FromWarehouse,
				Type: inventory.MovementTransferOut, QtyDelta: -it.Qty, QtyAfter: src[i].QtyOnHand,
				Reason: "transfer " + t.ID, TransferID: t.ID,
				CreatedBy: in.Actor, CreatedAt: now,
			}); err != nil {
				return err
			}

			dst[i].QtyOnHand += it.Qty
			if err := st.Inventory().SaveBalance(ctx, dst[i]); err != nil {
				return err
			}
			if err := st.Inventory().InsertMovement(ctx, &inventory.Movement{
				ID: platform.NewUUID(), OrganizationID: orgID,
				VariantID: it.VariantID, WarehouseID: in.ToWarehouse,
				Type: inventory.MovementTransferIn, QtyDelta: it.Qty, QtyAfter: dst[i].QtyOnHand,
				Reason: "transfer " + t.ID, TransferID: t.ID,
				CreatedBy: in.Actor, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return st.Inventory().CreateTransfer(ctx, t)
	})
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, in.Actor, requestID, audit.ActionStockTransferred, t.ID)
	s.log.Info("transfer stok selesai", "org", orgID, "transfer", t.ID,
		"from", in.FromWarehouse, "to", in.ToWarehouse, "items", len(in.Items))
	return t, nil
}

// normalizeTransferItems membuang varian kosong, memvalidasi UUID, dan
// menghapus duplikat (duplikat membuat pemeriksaan saldo keliru).
func normalizeTransferItems(raw []inventory.TransferItem) ([]inventory.TransferItem, error) {
	out := make([]inventory.TransferItem, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, it := range raw {
		id := strings.TrimSpace(it.VariantID)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, shared.Validation(map[string]string{
				"items": "Daftar varian memuat nilai yang bukan UUID.",
			})
		}
		if seen[id] {
			return nil, shared.Validation(map[string]string{
				"items": "Ada varian duplikat pada daftar transfer.",
			})
		}
		seen[id] = true
		out = append(out, inventory.TransferItem{VariantID: id, Qty: it.Qty})
	}
	return out, nil
}

// validateIDs memastikan bentuk UUID sebelum menyentuh query (cegah 22P02).
func validateIDs(ids ...string) error {
	for _, id := range ids {
		if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
			return shared.Validation(map[string]string{
				"id": "Identitas yang dikirim bukan UUID yang valid.",
			})
		}
	}
	return nil
}

// recordAudit menyimpan jejak audit inventory; kegagalan dicatat, tidak
// menggagalkan operasi bisnis.
func (s *Service) recordAudit(ctx context.Context, orgID, actorUserID, requestID, action, entityID string) {
	if s.audit == nil {
		return
	}
	e := audit.New(action, actorUserID, orgID).WithEntity("inventory", entityID)
	e.RequestID = requestID
	if err := s.audit.Record(ctx, e); err != nil {
		s.log.Warn("audit inventory gagal", "action", action, "error", err)
	}
}

// StockFilter adalah filter stok dari endpoint.
type StockFilter struct {
	WarehouseID string
	VariantID   string
	Search      string
	Limit       int
	Offset      int
}
