// Composable untuk customer CRM (tenant-scoped, butuh auth).
import type { Customer, CustomerSegment, CustomerStats } from '~/types/api'

export interface CustomerListMeta {
  total: number
  limit: number
  offset: number
}

export function useCustomers() {
  const { apiFetch, apiFetchWithMeta } = useApi()

  async function list(opts?: { search?: string, segment?: CustomerSegment, limit?: number, offset?: number }): Promise<{ items: Customer[], meta: CustomerListMeta }> {
    const query: Record<string, string | number | boolean | undefined> = {}
    if (opts?.search) query.search = opts.search
    if (opts?.segment) query.segment = opts.segment
    if (opts?.limit) query.limit = opts.limit
    if (typeof opts?.offset === 'number') query.offset = opts.offset

    const res = await apiFetchWithMeta<Customer[]>('/api/v1/customers', { query })
    const meta = res.meta ?? {}
    return {
      items: res.data,
      meta: {
        total: typeof meta.total === 'number' ? meta.total : res.data.length,
        limit: typeof meta.limit === 'number' ? meta.limit : (opts?.limit ?? 20),
        offset: typeof meta.offset === 'number' ? meta.offset : (opts?.offset ?? 0),
      },
    }
  }

  async function get(id: string): Promise<Customer> {
    return apiFetch<Customer>(`/api/v1/customers/${id}`)
  }

  async function create(data: { name: string, phone: string, email?: string, notes?: string }): Promise<Customer> {
    return apiFetch<Customer>('/api/v1/customers', { method: 'POST', body: data })
  }

  async function update(id: string, data: Partial<Pick<Customer, 'name' | 'phone' | 'email' | 'notes' | 'default_address_id'>>): Promise<Customer> {
    return apiFetch<Customer>(`/api/v1/customers/${id}`, { method: 'PATCH', body: data })
  }

  async function remove(id: string): Promise<void> {
    await apiFetch<Record<string, never>>(`/api/v1/customers/${id}`, { method: 'DELETE' })
  }

  async function stats(id: string): Promise<CustomerStats> {
    return apiFetch<CustomerStats>(`/api/v1/customers/${id}/stats`)
  }

  /** Override segmen; segmen kosong menghapus override (kembali auto). */
  async function setSegment(id: string, segment: CustomerSegment | ''): Promise<Customer> {
    return apiFetch<Customer>(`/api/v1/customers/${id}/segment`, { method: 'POST', body: { segment } })
  }

  return { list, get, create, update, remove, stats, setSegment }
}
