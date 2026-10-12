// Composable untuk address CRUD (tenant-scoped, butuh auth).
import type { Address, AddressValidationResult } from '~/types/api'

export function useAddresses() {
  const { apiFetch } = useApi()

  async function list(status?: 'active' | 'archived'): Promise<Address[]> {
    return apiFetch<Address[]>('/api/v1/addresses', { query: status ? { status } : {} })
  }

  async function search(q: string): Promise<Address[]> {
    return apiFetch<Address[]>('/api/v1/addresses/search', { query: { q } })
  }

  async function get(id: string): Promise<Address> {
    return apiFetch<Address>(`/api/v1/addresses/${id}`)
  }

  async function create(data: Partial<Address>): Promise<Address> {
    return apiFetch<Address>('/api/v1/addresses', { method: 'POST', body: data as Record<string, unknown> })
  }

  async function update(id: string, data: Partial<Address>): Promise<Address> {
    return apiFetch<Address>(`/api/v1/addresses/${id}`, { method: 'PATCH', body: data as Record<string, unknown> })
  }

  async function remove(id: string): Promise<void> {
    await apiFetch<Record<string, never>>(`/api/v1/addresses/${id}`, { method: 'DELETE' })
  }

  async function validate(data: { province_id: string, city_id: string, district_id: string, village_id?: string, postal_code?: string }): Promise<AddressValidationResult> {
    return apiFetch<AddressValidationResult>('/api/v1/addresses/validate', { method: 'POST', body: data })
  }

  return { list, search, get, create, update, remove, validate }
}
