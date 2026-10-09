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