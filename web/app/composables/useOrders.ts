// Composable untuk order (TASK-026). Semua endpoint tenant-scoped & butuh auth.
import type { Order, OrderPaymentType, OrderStatus, OrderSummary } from '~/types/api'

export interface OrderListMeta {
  total: number
  limit: number
  offset: number
}

export interface OrderListOptions {
  status?: OrderStatus | ''
  store_id?: string
  customer_id?: string
  search?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export interface OrderItemInput {
  variant_id?: string
  product_name: string
  variant_name?: string
  qty: number
  unit_price: number
}

export interface OrderCreateInput {
  store_id?: string
  customer_id: string
  shipping_address_id?: string
  payment_type: OrderPaymentType
  shipping_cost?: number
  discount?: number
  notes?: string
  external_order_id?: string
  items: OrderItemInput[]
}

export function useOrders() {
  const { apiFetch, apiFetchWithMeta } = useApi()

  async function list(opts: OrderListOptions = {}): Promise<{ items: Order[], meta: OrderListMeta }> {
    const query: Record<string, string | number | boolean | undefined> = {}
    if (opts.status) query.status = opts.status
    if (opts.store_id) query.store_id = opts.store_id
    if (opts.customer_id) query.customer_id = opts.customer_id
    if (opts.search) query.search = opts.search
    if (opts.from) query.from = opts.from
    if (opts.to) query.to = opts.to
    if (opts.limit) query.limit = opts.limit
    if (typeof opts.offset === 'number') query.offset = opts.offset

    const res = await apiFetchWithMeta<Order[]>('/api/v1/orders', { query })
    const meta = res.meta ?? {}
    return {
      items: res.data,
      meta: {
        total: typeof meta.total === 'number' ? meta.total : res.data.length,
        limit: typeof meta.limit === 'number' ? meta.limit : (opts.limit ?? 20),
        offset: typeof meta.offset === 'number' ? meta.offset : (opts.offset ?? 0),
      },
    }
  }

  async function get(id: string): Promise<Order> {
    return apiFetch<Order>(`/api/v1/orders/${id}`)
  }

  async function create(input: OrderCreateInput): Promise<Order> {
    return apiFetch<Order>('/api/v1/orders', { method: 'POST', body: { ...input } })
  }

  /** Memindahkan status lewat state machine server (tolak transisi ilegal → 409). */
  async function setStatus(id: string, status: OrderStatus, reason?: string): Promise<Order> {
    return apiFetch<Order>(`/api/v1/orders/${id}`, {
      method: 'PATCH',
      body: { status, reason: reason ?? '' },
    })
  }

  async function cancel(id: string, reason: string): Promise<Order> {
    return apiFetch<Order>(`/api/v1/orders/${id}/cancel`, {
      method: 'POST',
      body: { reason },
    })
  }

  async function summary(): Promise<OrderSummary> {
    return apiFetch<OrderSummary>('/api/v1/orders/summary')
  }

  return { list, get, create, setStatus, cancel, summary }
}