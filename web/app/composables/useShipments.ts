// Composable untuk pengiriman (TASK-027/028). Semua endpoint tenant-scoped.
//
// Catatan idempotency: POST /shipments WAJIB menyertakan Idempotency-Key.
// Composable membuat key sekali per percobaan pembuatan dan memakainya ulang
// bila pemanggil mengulang request yang sama — inilah yang membuat retry aman.
import type { Shipment, ShipmentCreateInput, ShipmentLifecycleStatus } from '~/types/api'

export interface ShipmentListMeta {
  total: number
  limit: number
  offset: number
}

export interface ShipmentListOptions {
  status?: ShipmentLifecycleStatus | ''
  courier?: string
  store_id?: string
  warehouse_id?: string
  order_id?: string
  search?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

/** Menghasilkan key idempotency unik untuk satu percobaan pembuatan. */
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  // Fallback non-kripto: cukup unik untuk mencegah dobel-klik.
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

export function useShipments() {
  const { apiFetch, apiFetchWithMeta, apiFetchBlob } = useApi()

  async function list(opts: ShipmentListOptions = {}): Promise<{ items: Shipment[], meta: ShipmentListMeta }> {
    const query: Record<string, string | number | boolean | undefined> = {}
    if (opts.status) query.status = opts.status
    if (opts.courier) query.courier = opts.courier
    if (opts.store_id) query.store_id = opts.store_id
    if (opts.warehouse_id) query.warehouse_id = opts.warehouse_id
    if (opts.order_id) query.order_id = opts.order_id
    if (opts.search) query.search = opts.search
    if (opts.from) query.from = opts.from
    if (opts.to) query.to = opts.to
    if (opts.limit) query.limit = opts.limit
    if (typeof opts.offset === 'number') query.offset = opts.offset

    const res = await apiFetchWithMeta<Shipment[]>('/api/v1/shipments', { query })
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

  async function get(id: string): Promise<Shipment> {
    return apiFetch<Shipment>(`/api/v1/shipments/${id}`)
  }

  /**
   * Membuat pengiriman. `idempotencyKey` WAJIB diisi; bila pemanggil tidak
   * memberikannya, satu key dibuat otomatis. Untuk retry yang aman, simpan key
   * dari percobaan pertama dan kirimkan lagi.
   */
  async function create(input: ShipmentCreateInput, idempotencyKey?: string): Promise<Shipment> {
    const key = idempotencyKey || newIdempotencyKey()
    return apiFetch<Shipment>('/api/v1/shipments', {
      method: 'POST',
      body: { ...input },
      headers: { 'Idempotency-Key': key },
    })
  }

  async function setStatus(id: string, status: ShipmentLifecycleStatus, reason?: string): Promise<Shipment> {
    return apiFetch<Shipment>(`/api/v1/shipments/${id}`, {
      method: 'PATCH',
      body: { status, reason: reason ?? '' },
    })
  }

  async function cancel(id: string, reason: string): Promise<Shipment> {
    return apiFetch<Shipment>(`/api/v1/shipments/${id}/cancel`, {
      method: 'POST',
      body: { reason },
    })
  }

  /** URL label PDF; `format` = a4 (default) atau thermal. */
  function labelURL(id: string, format: 'a4' | 'thermal' = 'a4'): string {
    return `/api/v1/shipments/${id}/label?format=${format}`
  }

  /**
   * Mengunduh label PDF langsung di browser. Dipisah dari `labelURL` karena
   * unduhan butuh header otorisasi — tautan biasa akan ditolak 401.
   */
  async function downloadLabel(id: string, format: 'a4' | 'thermal' = 'a4'): Promise<void> {
    const { blob, filename } = await apiFetchBlob(labelURL(id, format))
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename ?? `label-${id}.pdf`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  async function shareLink(id: string): Promise<{ url: string, awb: string, expires_at: string }> {
    return apiFetch<{ url: string, awb: string, expires_at: string }>(`/api/v1/shipments/${id}/share-link`)
  }

  return { list, get, create, setStatus, cancel, labelURL, downloadLabel, shareLink }
}