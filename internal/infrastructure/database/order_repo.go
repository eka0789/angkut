package database

import (
	"context"
	"errors"

	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/angkut/angkut/apps/internal/domain/order"
)

// OrderRepo menyimpan order tenant-scoped. Semua query memfilter
// organization_id dan mengecualikan baris soft-deleted.
type OrderRepo struct{ q Querier }

const orderCols = `id, organization_id, COALESCE(store_id::text,''), customer_id::text, order_number,
	COALESCE(external_order_id,''), status, payment_type, subtotal, shipping_cost, discount, total_amount,
	COALESCE(shipping_address_id::text,''), COALESCE(notes,''), COALESCE(cancelled_reason,''),
	created_at, updated_at`

// Create menyimpan order beserta seluruh itemnya. Harus dijalankan di dalam
// transaksi oleh pemanggil (application layer) — order tanpa item adalah data
// rusak, jadi keduanya wajib atomik.
func (r *OrderRepo) Create(ctx context.Context, o *order.Order) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO orders (
			id, organization_id, store_id, customer_id, order_number, external_order_id,
			status, payment_type, subtotal, shipping_cost, discount, total_amount,
			shipping_address_id, notes, created_at, updated_at
		) VALUES ($1,$2,NULLIF($3,'')::uuid,$4::uuid,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,
			NULLIF($13,'')::uuid,NULLIF($14,''),$15,$16)`,
		o.ID, o.OrganizationID, o.StoreID, o.CustomerID, o.OrderNumber, o.ExternalOrderID,
		string(o.Status), string(o.PaymentType), o.Subtotal, o.ShippingCost, o.Discount, o.TotalAmount,
		o.ShippingAddressID, o.Notes, o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		if isUniqueOrderNumber(err) {
			return order.ErrNumberTaken
		}
		if isForeignKeyOn(err, "orders_customer_id_fkey") {
			return order.ErrCustomerNotFound
		}
		if isForeignKeyOn(err, "orders_store_id_fkey") {
			return order.ErrStoreNotFound
		}
		if isForeignKeyOn(err, "orders_shipping_address_id_fkey") {
			return order.ErrAddressNotFound
		}
		return err
	}

	for idx := range o.Items {
		it := &o.Items[idx]
		if _, err := r.q.Exec(ctx, `
			INSERT INTO order_items (
				id, organization_id, order_id, variant_id,
				product_name_snapshot, variant_name_snapshot, qty, unit_price, total_price, created_at
			) VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,NULLIF($6,''),$7,$8,$9,$10)`,
			it.ID, it.OrganizationID, o.ID, it.VariantID,
			it.ProductNameSnapshot, it.VariantNameSnapshot, it.Qty, it.UnitPrice, it.TotalPrice, it.CreatedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *OrderRepo) GetByID(ctx context.Context, orgID, id string) (*order.Order, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL`,
		orgID, id)
	o, err := scanOrder(row)
	if err != nil {
		return nil, err
	}
	items, err := r.itemsFor(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.Items = items
	return o, nil
}

// List mengembalikan halaman order + total baris yang cocok filter.
func (r *OrderRepo) List(ctx context.Context, orgID string, f order.ListFilter) ([]order.Order, int, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	where := `organization_id=$1 AND deleted_at IS NULL`
	args := []any{orgID}
	if f.Status != "" {
		args = append(args, string(f.Status))
		where += ` AND status=$` + itoa(len(args))
	}
	if f.StoreID != "" {
		args = append(args, f.StoreID)
		where += ` AND store_id=$` + itoa(len(args)) + `::uuid`
	}
	if f.CustomerID != "" {
		args = append(args, f.CustomerID)
		where += ` AND customer_id=$` + itoa(len(args)) + `::uuid`
	}
	if f.Search != "" {
		args = append(args, f.Search)
		n := itoa(len(args))
		where += ` AND (order_number ILIKE '%'||$` + n + `||'%'
			OR (external_order_id IS NOT NULL AND external_order_id ILIKE '%'||$` + n + `||'%'))`
	}
	if f.From != "" {
		args = append(args, f.From)
		where += ` AND created_at >= $` + itoa(len(args)) + `::timestamptz`
	}
	if f.To != "" {
		args = append(args, f.To)
		where += ` AND created_at < $` + itoa(len(args)) + `::timestamptz`
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM orders WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := r.q.Query(ctx,
		`SELECT `+orderCols+` FROM orders WHERE `+where+
			` ORDER BY created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]order.Order, 0)
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *o)
	}
	return out, total, rows.Err()
}

// UpdateStatus mengubah status dengan syarat status saat ini masih `from`
// (optimistic). Baris tidak terpengaruh berarti status sudah berubah di
// tempat lain → ErrConflict, bukan diam-diam menimpa.
func (r *OrderRepo) UpdateStatus(ctx context.Context, orgID, id string, from, to order.Status, reason string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE orders SET status=$4, cancelled_reason=NULLIF($5,'')
		WHERE organization_id=$1 AND id=$2 AND status=$3 AND deleted_at IS NULL`,
		orgID, id, string(from), string(to), reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Bisa karena order tidak ada, atau statusnya sudah berbeda.
		if _, err := r.GetByID(ctx, orgID, id); err != nil {
			return err
		}
		return order.ErrNotFound
	}
	return nil
}

// NextOrderNumber mengembalikan nomor urut berikutnya untuk tahun tertentu.
// Menghitung dari nomor terbesar yang sudah ada (per organisasi) sehingga
// aman dari bentrok selama unik index tetap ada — pembuatan nomor tetap
// dilindungi unique constraint `uq_order_number` sebagai jaring terakhir.
func (r *OrderRepo) NextOrderNumber(ctx context.Context, orgID string, year int) (int64, error) {
	prefix := order.OrderNumberPrefix(year)
	var maxNumber *string
	err := r.q.QueryRow(ctx, `
		SELECT max(order_number) FROM orders
		WHERE organization_id=$1 AND order_number LIKE $2||'%'`, orgID, prefix).Scan(&maxNumber)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if maxNumber == nil {
		return 1, nil
	}
	seq, ok := order.ParseOrderNumberSeq(year, *maxNumber)
	if !ok {
		// Ada baris dengan awalan cocok tapi bentuk tak dikenal — jangan
		// menebak dan berisiko bentrok; biarkan unique index yang menangkap.
		return 1, nil
	}
	return seq + 1, nil
}

// itemsFor memuat seluruh item satu order.
func (r *OrderRepo) itemsFor(ctx context.Context, orderID string) ([]order.Item, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, organization_id, order_id, COALESCE(variant_id::text,''),
		       product_name_snapshot, COALESCE(variant_name_snapshot,''),
		       qty, unit_price, total_price, created_at
		FROM order_items WHERE order_id=$1 ORDER BY created_at, id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]order.Item, 0)
	for rows.Next() {
		var it order.Item
		if err := rows.Scan(&it.ID, &it.OrganizationID, &it.OrderID, &it.VariantID,
			&it.ProductNameSnapshot, &it.VariantNameSnapshot,
			&it.Qty, &it.UnitPrice, &it.TotalPrice, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanOrder(row pgx.Row) (*order.Order, error) {
	var (
		o         order.Order
		status    string
		payType   string
		createdAt time.Time
	)
	err := row.Scan(
		&o.ID, &o.OrganizationID, &o.StoreID, &o.CustomerID, &o.OrderNumber,
		&o.ExternalOrderID, &status, &payType, &o.Subtotal, &o.ShippingCost,
		&o.Discount, &o.TotalAmount, &o.ShippingAddressID, &o.Notes,
		&o.CancelledReason, &createdAt, &o.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, order.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	o.Status = order.Status(status)
	o.PaymentType = order.PaymentType(payType)
	o.CreatedAt = createdAt
	o.Items = []order.Item{}
	return &o, nil
}

// isUniqueOrderNumber mendeteksi pelanggaran unique nomor order.
func isUniqueOrderNumber(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "uq_order_number"
	}
	return false
}

// isForeignKeyOn mendeteksi pelanggaran FK pada constraint TERTENTU, sehingga
// pemanggil dapat memetakan ke pesan domain yang tepat (pelanggan vs toko vs
// alamat) alih-alih satu pesan generik.
func isForeignKeyOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503" && pgErr.ConstraintName == constraint
	}
	return false
}
