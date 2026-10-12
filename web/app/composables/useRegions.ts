// Composable untuk region hierarchy (publik, no auth).
import type { Province, City, District, Village, PostalCode } from '~/types/api'

export function useRegions() {
  const { apiFetch } = useApi()

  async function listProvinces(): Promise<Province[]> {
    return apiFetch<Province[]>('/api/v1/regions/provinces', { auth: false })
  }

  async function listCities(provinceId: string): Promise<City[]> {
    return apiFetch<City[]>('/api/v1/regions/cities', { query: { province_id: provinceId }, auth: false })
  }

  async function listDistricts(cityId: string): Promise<District[]> {
    return apiFetch<District[]>('/api/v1/regions/districts', { query: { city_id: cityId }, auth: false })
  }

  async function listVillages(districtId: string): Promise<Village[]> {
    return apiFetch<Village[]>('/api/v1/regions/villages', { query: { district_id: districtId }, auth: false })
  }

  async function listPostalCodes(districtId: string): Promise<PostalCode[]> {
    return apiFetch<PostalCode[]>('/api/v1/regions/postal-codes', { query: { district_id: districtId }, auth: false })
  }

  return { listProvinces, listCities, listDistricts, listVillages, listPostalCodes }
}
