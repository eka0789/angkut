// Composable untuk registry kurir & layanannya.
// Dipakai wizard pengiriman untuk memilih kurir + layanan; daftar layanan
// baru diambil setelah kurir dipilih sehingga permintaan tetap ringan.
import type { Courier, CourierService } from '~/types/api'

export function useCourierServices() {
  const { apiFetch } = useApi()

  /** Daftar kurir aktif (GET /couriers). Endpoint read-only. */
  async function list(): Promise<Courier[]> {
    return apiFetch<Courier[]>('/api/v1/couriers')
  }

  /** Layanan aktif satu kurir (GET /couriers/{code}/services). */
  async function getServices(code: string): Promise<CourierService[]> {
    const res = await apiFetch<{ courier: Courier, services: CourierService[] }>(
      `/api/v1/couriers/${code}/services`,
    )
    return res.services ?? []
  }

  return { list, getServices }
}