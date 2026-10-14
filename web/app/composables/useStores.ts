// Composable untuk store (kanal penjualan) & warehouse (gudang).
// Keduanya prasyarat order & shipment: order berasal dari store, shipment
// berangkat dari gudang (origin pengiriman).
import type { Store, StoreChannel, Warehouse } from '~/types/api'

export function useStores() {
  const { apiFetch } = useApi()

  async function list(onlyActive = true): Promise<Store[]> {
    return apiFetch<Store[]>('/api/v1/stores', {
      query: { is_active: String(onlyActive) },
    })
  }

  async function get(id: string): Promise<Store> {
    return apiFetch<Store>(`/api/v1/stores/${id}`)
  }

  async function create(input: { name: string, channel: StoreChannel, adapter_type?: string, warehouse_id?: string }): Promise<Store> {
    return apiFetch<Store>('/api/v1/stores', { method: 'POST', body: { ...input } })
  }

  return { list, get, create }
}

export function useWarehouses() {
  const { apiFetch } = useApi()

  async function list(onlyActive = true): Promise<Warehouse[]> {
    return apiFetch<Warehouse[]>('/api/v1/warehouses', {
      query: { is_active: String(onlyActive) },
    })
  }

  async function get(id: string): Promise<Warehouse> {
    return apiFetch<Warehouse>(`/api/v1/warehouses/${id}`)
  }

  async function create(input: { name: string, address_id?: string, capacity?: number, is_default?: boolean }): Promise<Warehouse> {
    return apiFetch<Warehouse>('/api/v1/warehouses', { method: 'POST', body: { ...input } })
  }

  return { list, get, create }
}