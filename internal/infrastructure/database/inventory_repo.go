package database

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/inventory"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/platform"
)

// ProductRepo menyimpan katalog produk & varian, tenant-scoped.
type ProductRepo struct{ q Querier }

const productCols = `id, organization_id, name, category,
	COALESCE(weight_kg::text,''), COALESCE(length_cm::text,''), COALESCE(width_cm::text,''), COALESCE(height_cm::text,''),
	is_active, COALESCE(created_by::text,''), created_at, updated_at`

const variantCols = `id, organization_id, product_id::text, sku, name,
	COALESCE(weight_kg::text,''), COALESCE(length_cm::text,''), COALESCE(width_cm::text,''), COALESCE(height_cm::text,''),
	low_stock_threshold, is_active, created_at, updated_at`

// nullableFloat menerjemahkan COALESCE(::text,”) kembali ke *float64.
func nullableFloat(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// Create menyimpan produk baru.
func (r *ProductRepo) Create(ctx context.Context, p *inventory.Product) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO products (
			id, organization_id, name, category, weight_kg, length_cm, width_cm, height_cm,
			is_active, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid,$11,$12)`,
		p.ID, p.OrganizationID, p.Name, p.Category, p.WeightKG, p.LengthCM, p.WidthCM, p.HeightCM,
		p.IsActive, p.CreatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}

// GetByID mengambil produk beserta variannya.
func (r *ProductRepo) GetByID(ctx context.Context, orgID, id string) (*inventory.Product, error) {
	row := r.q.QueryRow(ctx, `SELECT `+productCols+` FROM products WHERE organization_id=$1 AND id=$2`, orgID, id)
	p, err := scanProduct(row)
	if err != nil {
		return nil, err
	}
	variants, err := r.listVariants(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	p.Variants = variants
	return p, nil
}

func (r *ProductRepo) listVariants(ctx context.Context, orgID, productID string) ([]inventory.Variant, error) {
	rows, err := r.q.Query(ctx, `SELECT `+variantCols+` FROM product_variants
		WHERE organization_id=$1 AND product_id=$2 ORDER BY sku`, orgID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]inventory.Variant, 0, 4)
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// List mengembalikan halaman produk. Search mencocokkan nama produk ATAU SKU
// varian (produk ikut tampil bila salah satu variannya cocok).
func (r *ProductRepo) List(ctx context.Context, orgID string, f inventory.ProductFilter) ([]inventory.Product, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if s := strings.TrimSpace(f.Search); s != "" {
		where += ` AND (name ILIKE $` + itoa(n) + ` OR id IN (
			SELECT product_id FROM product_variants WHERE organization_id=$1 AND sku ILIKE $` + itoa(n) + `))`
		args = append(args, "%"+s+"%")
		n++
	}
	if c := strings.TrimSpace(f.Category); c != "" {
		where += ` AND category=$` + itoa(n)
		args = append(args, c)
		n++
	}
	if f.LowStock {
		where += ` AND id IN (
			SELECT p.id FROM products p
			JOIN product_variants v ON v.product_id = p.id AND v.organization_id = p.organization_id
			JOIN inventory i ON i.variant_id = v.id AND i.organization_id = v.organization_id
			WHERE p.organization_id = $1 AND v.is_active
			  AND (i.qty_on_hand - i.qty_reserved) <= v.low_stock_threshold)`
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM products WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args = append(args, limit, f.Offset)
	rows, err := r.q.Query(ctx, `SELECT `+productCols+` FROM products WHERE `+where+
		` ORDER BY created_at DESC LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]inventory.Product, 0, limit)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Varian dimuat sekali untuk seluruh halaman (hindari N+1).
	for i := range out {
		variants, err := r.listVariants(ctx, orgID, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Variants = variants
	}
	return out, total, nil
}

// Update menyimpan perubahan data produk.
func (r *ProductRepo) Update(ctx context.Context, p *inventory.Product) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE products SET name=$3, category=$4, weight_kg=$5, length_cm=$6, width_cm=$7, height_cm=$8
		WHERE organization_id=$1 AND id=$2`,
		p.OrganizationID, p.ID, p.Name, p.Category, p.WeightKG, p.LengthCM, p.WidthCM, p.HeightCM)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return inventory.ErrNotFound
	}
	return nil
}

// SetActive mengarsipkan/mengaktifkan produk (soft delete).
func (r *ProductRepo) SetActive(ctx context.Context, orgID, id string, active bool) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE products SET is_active=$3 WHERE organization_id=$1 AND id=$2`,
		orgID, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return inventory.ErrNotFound
	}
	return nil
}

// AddVariant menambah SKU; bentrok unik (org, sku) → ErrSKUTaken.
func (r *ProductRepo) AddVariant(ctx context.Context, v *inventory.Variant) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO product_variants (
			id, organization_id, product_id, sku, name, weight_kg, length_cm, width_cm, height_cm,
			low_stock_threshold, is_active, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		v.ID, v.OrganizationID, v.ProductID, v.SKU, v.Name, v.WeightKG, v.LengthCM, v.WidthCM, v.HeightCM,
		v.LowStockThreshold, v.IsActive, v.CreatedAt, v.UpdatedAt)
	if isUniqueViolationOn(err, "uq_variant_sku") {
		return inventory.ErrSKUTaken
	}
	return err
}

// UpdateVariant mengubah data varian (bukan saldo).
func (r *ProductRepo) UpdateVariant(ctx context.Context, v *inventory.Variant) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE product_variants SET sku=$3, name=$4, weight_kg=$5, length_cm=$6, width_cm=$7, height_cm=$8,
			low_stock_threshold=$9, is_active=$10
		WHERE organization_id=$1 AND id=$2`,
		v.OrganizationID, v.ID, v.SKU, v.Name, v.WeightKG, v.LengthCM, v.WidthCM, v.HeightCM,
		v.LowStockThreshold, v.IsActive)
	if err != nil {
		if isUniqueViolationOn(err, "uq_variant_sku") {
			return inventory.ErrSKUTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.NotFound("VARIANT_NOT_FOUND", "Varian tidak ditemukan.")
	}
	return nil
}

// VariantExists memeriksa varian milik organisasi (validasi input transfer).
func (r *ProductRepo) VariantExists(ctx context.Context, orgID, variantID string) (bool, error) {
	var ok bool
	err := r.q.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM product_variants WHERE organization_id=$1 AND id=$2::uuid)`, orgID, variantID).Scan(&ok)
	return ok, err
}

func scanProduct(row pgx.Row) (*inventory.Product, error) {
	var p inventory.Product
	var weight, length, width, height string
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Category,
		&weight, &length, &width, &height, &p.IsActive, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("PRODUCT_NOT_FOUND", "Produk tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	p.WeightKG = nullableFloat(weight)
	p.LengthCM = nullableFloat(length)
	p.WidthCM = nullableFloat(width)
	p.HeightCM = nullableFloat(height)
	return &p, nil
}

func scanVariant(rows pgx.Rows) (*inventory.Variant, error) {
	var v inventory.Variant
	var weight, length, width, height string
	err := rows.Scan(&v.ID, &v.OrganizationID, &v.ProductID, &v.SKU, &v.Name,
		&weight, &length, &width, &height, &v.LowStockThreshold, &v.IsActive, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	v.WeightKG = nullableFloat(weight)
	v.LengthCM = nullableFloat(length)
	v.WidthCM = nullableFloat(width)
	v.HeightCM = nullableFloat(height)
	return &v, nil
}

// InventoryRepo mengelola saldo stok, movement, dan transfer.
type InventoryRepo struct{ q Querier }

// Balance mengambil saldo tanpa lock (baca saja).
func (r *InventoryRepo) Balance(ctx context.Context, orgID, variantID, warehouseID string) (*inventory.Stock, error) {
	row := r.q.QueryRow(ctx, `SELECT id, organization_id, variant_id::text, warehouse_id::text, qty_on_hand, qty_reserved, version
		FROM inventory WHERE organization_id=$1 AND variant_id=$2::uuid AND warehouse_id=$3::uuid`,
		orgID, variantID, warehouseID)
	return scanStock(row)
}

// BalanceForUpdate mengambil saldo dengan FOR UPDATE — WAJIB di dalam
// transaksi; dua transfer paralel yang menyentuh gudang sama akan berurut.
func (r *InventoryRepo) BalanceForUpdate(ctx context.Context, orgID, variantID, warehouseID string) (*inventory.Stock, error) {
	row := r.q.QueryRow(ctx, `SELECT id, organization_id, variant_id::text, warehouse_id::text, qty_on_hand, qty_reserved, version
		FROM inventory WHERE organization_id=$1 AND variant_id=$2::uuid AND warehouse_id=$3::uuid FOR UPDATE`,
		orgID, variantID, warehouseID)
	return scanStock(row)
}

// UpsertBalance memastikan slot saldo ada (qty 0) lalu mengembalikannya
// terkunci FOR UPDATE.
func (r *InventoryRepo) UpsertBalance(ctx context.Context, orgID, variantID, warehouseID string) (*inventory.Stock, error) {
	if _, err := r.q.Exec(ctx, `
		INSERT INTO inventory (id, organization_id, variant_id, warehouse_id)
		VALUES ($1,$2,$3::uuid,$4::uuid)
		ON CONFLICT (organization_id, variant_id, warehouse_id) DO NOTHING`,
		platform.NewUUID(), orgID, variantID, warehouseID); err != nil {
		return nil, err
	}
	return r.BalanceForUpdate(ctx, orgID, variantID, warehouseID)
}

// SaveBalance menulis saldo baru dengan optimistic lock version; bentrok
// berarti ada penulis paralel — pemanggil WAJIB menganggapnya Conflict.
func (r *InventoryRepo) SaveBalance(ctx context.Context, s *inventory.Stock) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE inventory SET qty_on_hand=$3, qty_reserved=$4, version=version+1
		WHERE id=$1 AND version=$2`,
		s.ID, s.Version, s.QtyOnHand, s.QtyReserved)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.Conflict("INVENTORY_VERSION_CONFLICT",
			"Stok berubah di saat bersamaan; ulangi operasi.")
	}
	s.Version++
	return nil
}

// InsertMovement menulis satu jejak movement (append-only).
func (r *InventoryRepo) InsertMovement(ctx context.Context, m *inventory.Movement) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO inventory_movements (
			id, organization_id, variant_id, warehouse_id, type, qty_delta, qty_after,
			reason, transfer_id, created_by, created_at
		) VALUES ($1,$2,$3::uuid,$4::uuid,$5,$6,$7,$8,NULLIF($9,'')::uuid,NULLIF($10,'')::uuid,$11)`,
		m.ID, m.OrganizationID, m.VariantID, m.WarehouseID, string(m.Type), m.QtyDelta, m.QtyAfter,
		m.Reason, m.TransferID, m.CreatedBy, m.CreatedAt)
	return err
}

const stockRowCols = `i.id, i.organization_id, i.variant_id::text, i.warehouse_id::text,
	i.qty_on_hand, i.qty_reserved, i.version, v.sku, COALESCE(p.name,''), COALESCE(v.name,''), v.low_stock_threshold`

// ListStock mengembalikan saldo plus keterangan varian/produk.
func (r *InventoryRepo) ListStock(ctx context.Context, orgID string, f inventory.StockFilter) ([]inventory.StockRow, int, error) {
	where := `i.organization_id=$1`
	args := []any{orgID}
	n := 2
	if f.WarehouseID != "" {
		where += ` AND i.warehouse_id=$` + itoa(n) + `::uuid`
		args = append(args, f.WarehouseID)
		n++
	}
	if f.VariantID != "" {
		where += ` AND i.variant_id=$` + itoa(n) + `::uuid`
		args = append(args, f.VariantID)
		n++
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		where += ` AND (v.sku ILIKE $` + itoa(n) + ` OR p.name ILIKE $` + itoa(n) + `)`
		args = append(args, "%"+s+"%")
		n++
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM inventory i
		JOIN product_variants v ON v.id = i.variant_id
		JOIN products p ON p.id = v.product_id
		WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args = append(args, limit, f.Offset)
	rows, err := r.q.Query(ctx, `SELECT `+stockRowCols+` FROM inventory i
		JOIN product_variants v ON v.id = i.variant_id
		JOIN products p ON p.id = v.product_id
		WHERE `+where+` ORDER BY p.name, v.sku LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]inventory.StockRow, 0, limit)
	for rows.Next() {
		var sr inventory.StockRow
		if err := rows.Scan(&sr.ID, &sr.OrganizationID, &sr.VariantID, &sr.WarehouseID,
			&sr.QtyOnHand, &sr.QtyReserved, &sr.Version, &sr.SKU, &sr.ProductName, &sr.VariantName, &sr.Threshold); err != nil {
			return nil, 0, err
		}
		out = append(out, sr)
	}
	return out, total, rows.Err()
}

// ListMovements mengembalikan riwayat movement (append-only).
func (r *InventoryRepo) ListMovements(ctx context.Context, orgID string, f inventory.MovementFilter) ([]inventory.Movement, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if f.VariantID != "" {
		where += ` AND variant_id=$` + itoa(n) + `::uuid`
		args = append(args, f.VariantID)
		n++
	}
	if f.WarehouseID != "" {
		where += ` AND warehouse_id=$` + itoa(n) + `::uuid`
		args = append(args, f.WarehouseID)
		n++
	}
	if f.Type != "" {
		if !f.Type.Valid() {
			return nil, 0, shared.Validation(map[string]string{"type": "Jenis movement tidak dikenal."})
		}
		where += ` AND type=$` + itoa(n)
		args = append(args, string(f.Type))
		n++
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM inventory_movements WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args = append(args, limit, f.Offset)
	rows, err := r.q.Query(ctx, `SELECT id, organization_id, variant_id::text, warehouse_id::text, type, qty_delta, qty_after,
		COALESCE(reason,''), COALESCE(transfer_id::text,''), COALESCE(created_by::text,''), created_at
		FROM inventory_movements WHERE `+where+` ORDER BY created_at DESC, id LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]inventory.Movement, 0, limit)
	for rows.Next() {
		var m inventory.Movement
		var typ string
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.VariantID, &m.WarehouseID, &typ,
			&m.QtyDelta, &m.QtyAfter, &m.Reason, &m.TransferID, &m.CreatedBy, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		m.Type = inventory.MovementType(typ)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// LowStock mengembalikan varian dengan available ≤ threshold per gudang
// (warehouseID kosong = seluruh gudang).
func (r *InventoryRepo) LowStock(ctx context.Context, orgID, warehouseID string) ([]inventory.LowStockRow, error) {
	where := `i.organization_id = $1::uuid AND v.is_active`
	args := []any{orgID}
	if warehouseID != "" {
		where += ` AND i.warehouse_id = $2::uuid`
		args = append(args, warehouseID)
	}
	rows, err := r.q.Query(ctx, `
		SELECT v.id::text, v.sku, p.name, i.warehouse_id::text,
			(i.qty_on_hand - i.qty_reserved) AS available, v.low_stock_threshold
		FROM inventory i
		JOIN product_variants v ON v.id = i.variant_id
		JOIN products p ON p.id = v.product_id
		WHERE `+where+`
		  AND (i.qty_on_hand - i.qty_reserved) <= v.low_stock_threshold
		ORDER BY available, v.sku`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]inventory.LowStockRow, 0, 8)
	for rows.Next() {
		var lr inventory.LowStockRow
		if err := rows.Scan(&lr.VariantID, &lr.SKU, &lr.ProductName, &lr.WarehouseID,
			&lr.QtyAvailable, &lr.LowStockThreshold); err != nil {
			return nil, err
		}
		out = append(out, lr)
	}
	return out, rows.Err()
}

// CreateTransfer menyimpan transfer + item. WAJIB dipanggil di transaksi yang
// sama dengan perubahan saldo & movement.
func (r *InventoryRepo) CreateTransfer(ctx context.Context, t *inventory.StockTransfer) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO stock_transfers (id, organization_id, from_warehouse_id, to_warehouse_id, notes, created_by, created_at)
		VALUES ($1,$2,$3::uuid,$4::uuid,$5,NULLIF($6,'')::uuid,$7)`,
		t.ID, t.OrganizationID, t.FromWarehouse, t.ToWarehouse, t.Notes, t.CreatedBy, t.CreatedAt)
	if err != nil {
		if isForeignKeyViolation(err) {
			return shared.Validation(map[string]string{
				"from": "Gudang asal/tujuan tidak ditemukan pada organisasi ini.",
			})
		}
		return err
	}
	for _, it := range t.Items {
		if _, err := r.q.Exec(ctx, `
			INSERT INTO stock_transfer_items (transfer_id, variant_id, qty)
			VALUES ($1,$2::uuid,$3)`, t.ID, it.VariantID, it.Qty); err != nil {
			return err
		}
	}
	return nil
}

func scanStock(row pgx.Row) (*inventory.Stock, error) {
	var s inventory.Stock
	err := row.Scan(&s.ID, &s.OrganizationID, &s.VariantID, &s.WarehouseID,
		&s.QtyOnHand, &s.QtyReserved, &s.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("INVENTORY_NOT_FOUND", "Saldo stok tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
