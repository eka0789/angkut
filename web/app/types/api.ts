// Tipe yang mencerminkan kontrak API (planning/api-contract.md §2).
// Komponen DILARANG memakai `any`; semua bentuk respons punya tipe di sini.

/** Envelope respons tunggal dari backend. */
export interface ApiSuccess<T> {
  success: true
  data: T
  meta?: Record<string, unknown>
  message?: string
}

export interface ApiErrorBody {
  code: string
  message: string
  fields?: Record<string, string>
}

export interface ApiFailure {
  success: false
  error: ApiErrorBody
}

export type ApiResponse<T> = ApiSuccess<T> | ApiFailure

/** Meta paginasi kursor (default) dan offset (tabel). */
export interface CursorMeta {
  next_cursor?: string
  has_more: boolean
  limit: number
}

export interface OffsetMeta {
  current_page: number
  last_page: number
  per_page: number
  total: number
}

/** Status pengiriman kanonik — sama dengan enum backend. */
export type ShipmentStatus =
  | 'pending'
  | 'picked_up'
  | 'in_transit'
  | 'out_for_delivery'
  | 'delivered'
  | 'failed'
  | 'returned'
  | 'cancelled'

/** Label rekomendasi kurir. */
export type RateLabel = 'best_price' | 'fastest' | 'most_reliable' | 'best_value'

/** Provinsi (level 1). */
export interface Province {
  id: string
  name: string
  created_at: string
}

/** Kota/Kabupaten (level 2). */
export interface City {
  id: string
  province_id: string
  name: string
  type: 'kota' | 'kabupaten'
  created_at: string
}

/** Kecamatan (level 3). */
export interface District {
  id: string
  city_id: string
  name: string
  created_at: string
}

/** Desa/Kelurahan (level 4). */
export interface Village {
  id: string
  district_id: string
  name: string
  type: 'kelurahan' | 'desa'
  created_at: string
}

/** Kode pos. */
export interface PostalCode {
  code: string
  district_id: string
  created_at: string
}

/** Alamat tenant-scoped. */
export interface Address {
  id: string
  organization_id: string
  recipient: string
  phone: string
  province_id: string
  city_id: string
  district_id: string
  village_id?: string
  postal_code?: string
  street: string
  notes?: string
  latitude?: number
  longitude?: number
  label?: string
  is_default: boolean
  status: 'active' | 'archived'
  created_by: string
  created_at: string
  updated_at: string
}

/** Hasil validasi alamat dari endpoint /validate. */
export interface AddressValidationResult {
  valid: boolean
  issues: string[]
  suggestions: string[]
}

/** Segmen pelanggan — sama dengan enum backend (migrasi 0011). */
export type CustomerSegment = 'new' | 'regular' | 'vip' | 'high_risk' | 'cod_risk'

/** Metrik agregat pelanggan (jsonb `customers.metrics`). */
export interface CustomerMetrics {
  orders_count: number
  total_spend: number
  aov: number
  return_rate: number
  cod_success_rate?: number
  preferred_courier?: string
}

/** Pelanggan tenant-scoped. */
export interface Customer {
  id: string
  organization_id: string
  name: string
  phone: string
  email?: string
  notes?: string
  default_address_id?: string
  segment: CustomerSegment
  segment_source: 'auto' | 'manual'
  metrics: CustomerMetrics
  created_at: string
  updated_at: string
}

/** Ringkasan metrik (endpoint GET /customers/{id}/stats). */
export interface CustomerStats {
  orders_count: number
  total_spend: number
  aov: number
  return_rate: number
  cod_success_rate?: number
  preferred_courier?: string
}