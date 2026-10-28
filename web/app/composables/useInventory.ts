// Composable untuk produk & stok (TASK-037/038). Semua endpoint tenant-scoped;
// mutasi stok hanya warehouse+ (403 bila role kurang — tampilkan pesan jelas).
import type {
  InventoryMovement,
  InventoryStock,
  LowStockRow,
  Product,
  ProductVariant,
  StockTransfer,
} from '~/types/api'

export function useInventory() {
  const { apiFetch, apiFetchWithMeta } = useApi()

  async function listProducts(opts: { search?: string, category?: string, low_stock?: boolean, limit?: number, offset?: number } = {}): Promise<{ items: Product[], meta: { total: number, limit: number, offset: number } }> {
    const query: Record<string, string | number | boolean | undefined> = {}
    if (opts.search) query.search = opts.search
    if (opts.category) query.category = opts.category
    if (opts.low_stock) query.low_stock = true
    if (opts.limit !== undefined) query.limit = opts.limit
    if (opts.offset !== undefined) query.offset = opts.offset
    const res = await apiFetchWithMeta<Product[]>('/api/v1/products', { query })
    return {
      items: res.data,
      meta: {
        total: Number(res.meta?.total ?? res.data.length),
        limit: Number(res.meta?.limit ?? opts.limit ?? 20),
        offset: Number(res.meta?.offset ?? opts.offset ?? 0),
      },
    }
  }

  async function getProduct(id: string): Promise<Product> {
    return apiFetch<Product>(`/api/v1/products/${id}`)
  }

  async function createProduct(input: { name: string, category?: string, weight_kg?: number | null, length_cm?: number | null, width_cm?: number | null, height_cm?: number | null }): Promise<Product> {
    return apiFetch<Product>('/api/v1/products', { method: 'POST', body: input })
  }

  async function addVariant(productID: string, input: { sku: string, name?: string, low_stock_threshold?: number }): Promise<ProductVariant> {
    return apiFetch<ProductVariant>(`/api/v1/products/${productID}/variants`, { method: 'POST', body: input })
  }

  async function listStock(opts: { warehouse_id?: string, variant_id?: string, search?: string, limit?: number, offset?: number } = {}): Promise<{ items: InventoryStock[], meta: { total: number, limit: number, offset: number } }> {
    const query: Record<string, string | number | undefined> = {}
    if (opts.warehouse_id) query.warehouse_id = opts.warehouse_id
    if (opts.variant_id) query.variant_id = opts.variant_id
    if (opts.search) query.search = opts.search
    if (opts.limit !== undefined) query.limit = opts.limit
    if (opts.offset !== undefined) query.offset = opts.offset
    const res = await apiFetchWithMeta<InventoryStock[]>('/api/v1/inventory', { query })
    return {
      items: res.data,
      meta: {
        total: Number(res.meta?.total ?? res.data.length),
        limit: Number(res.meta?.limit ?? opts.limit ?? 50),
        offset: Number(res.meta?.offset ?? opts.offset ?? 0),
      },
    }
  }

  async function adjust(input: { variant_id: string, warehouse_id: string, qty_delta: number, reason?: string }): Promise<InventoryStock> {
    return apiFetch<InventoryStock>('/api/v1/inventory/adjust', { method: 'POST', body: input })
  }

  async function movements(opts: { variant_id?: string, warehouse_id?: string, type?: string, limit?: number, offset?: number } = {}): Promise<{ items: InventoryMovement[], meta: { total: number, limit: number, offset: number } }> {
    const query: Record<string, string | number | undefined> = {}
    if (opts.variant_id) query.variant_id = opts.variant_id
    if (opts.warehouse_id) query.warehouse_id = opts.warehouse_id
    if (opts.type) query.type = opts.type
    if (opts.limit !== undefined) query.limit = opts.limit
    if (opts.offset !== undefined) query.offset = opts.offset
    const res = await apiFetchWithMeta<InventoryMovement[]>('/api/v1/inventory/movements', { query })
    return {
      items: res.data,
      meta: {
        total: Number(res.meta?.total ?? res.data.length),
        limit: Number(res.meta?.limit ?? opts.limit ?? 50),
        offset: Number(res.meta?.offset ?? opts.offset ?? 0),
      },
    }
  }

  async function lowStock(warehouseID?: string): Promise<LowStockRow[]> {
    const query: Record<string, string | undefined> = {}
    if (warehouseID) query.warehouse_id = warehouseID
    return apiFetch<LowStockRow[]>('/api/v1/inventory/low-stock', { query })
  }

  async function transfer(input: { from: string, to: string, notes?: string, items: { variant_id: string, qty: number }[] }): Promise<StockTransfer> {
    return apiFetch<StockTransfer>('/api/v1/warehouses/transfers', { method: 'POST', body: input })
  }

  return { listProducts, getProduct, createProduct, addVariant, listStock, adjust, movements, lowStock, transfer }
}
