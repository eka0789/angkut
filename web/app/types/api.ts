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

/** Identitas ringkas kurir/layanan pada opsi tarif. */
export interface RateParty {
  code: string
  name: string
}

/** Alasan dan keyakinan rekomendasi dari scoring engine. */
export interface RateRecommendation {
  reasons: string[]
  confidence: number
}

/**
 * Satu opsi tarif hasil GET /couriers/rates.
 * `price` adalah total yang dibayar pelanggan; rinciannya ada di
 * base_price/markup/cod_fee/insurance_fee.
 */
export interface RateOption {
  courier: RateParty
  service: RateParty
  origin_zone: string
  destination_zone: string
  chargeable_weight: number
  volumetric_weight?: number
  price: number
  base_price: number
  markup: number
  cod_fee: number
  insurance_fee: number
  eta_min_days: number
  eta_max_days: number
  reliability_score: number
  supports_cod: boolean
  supports_pickup: boolean
  estimated_success_rate: number
  score: number
  labels: RateLabel[]
  is_recommended: boolean
  recommendation?: RateRecommendation
}

/** Peta label → "KODE_KURIR:KODE_LAYANAN" pemenang tiap kriteria. */
export interface RateSummaryLabels {
  cheapest: string
  fastest: string
  most_reliable: string
  best_value: string
}

/** Hasil lengkap pencarian tarif (GET /rates, POST /rates/preview). */
export interface RateQuote {
  quote_id: string
  origin_zone: string
  destination_zone: string
  chargeable_weight: number
  options: RateOption[]
  summary_labels: RateSummaryLabels
}

/** Hasil POST /couriers/recommend. */
export interface RateRecommendResponse {
  quote_id: string
  chargeable_weight: number
  summary_labels: RateSummaryLabels
  recommended: RateOption | null
}

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
// ─── Store & Warehouse (TASK-026) ────────────────────────────────────────

/** Kanal penjualan tenant. */
export type StoreChannel =
  | 'manual'
  | 'shopify'
  | 'woocommerce'
  | 'tokopedia'
  | 'shopee'
  | 'tiktok_shop'
  | 'custom'

/** Toko/kanal penjualan. Kredensial tidak pernah dikirim ke klien. */
export interface Store {
  id: string
  organization_id: string
  name: string
  channel: StoreChannel
  adapter_type: string
  has_credentials: boolean
  warehouse_id?: string
  is_active: boolean
  created_at: string
  updated_at: string
}

/** Gudang/lokasi fulfillment; alamatnya jadi origin pengiriman. */
export interface Warehouse {
  id: string
  organization_id: string
  name: string
  address_id?: string
  capacity?: number
  is_default: boolean
  is_active: boolean
  created_at: string
  updated_at: string
}

// ─── Order (TASK-026) ────────────────────────────────────────────────────

/** Status order kanonik — sama dengan CHECK di migrasi 0015. */
export type OrderStatus =
  | 'draft'
  | 'confirmed'
  | 'processing'
  | 'shipped'
  | 'delivered'
  | 'cancelled'
  | 'returned'

export type OrderPaymentType = 'prepaid' | 'cod'

/** Satu baris item order (nama produk disnapshot saat order dibuat). */
export interface OrderItem {
  id: string
  order_id: string
  variant_id?: string
  product_name_snapshot: string
  variant_name_snapshot?: string
  qty: number
  unit_price: number
  total_price: number
  created_at: string
}

export interface Order {
  id: string
  organization_id: string
  store_id?: string
  customer_id: string
  order_number: string
  external_order_id?: string
  status: OrderStatus
  payment_type: OrderPaymentType
  subtotal: number
  shipping_cost: number
  discount: number
  total_amount: number
  shipping_address_id?: string
  notes?: string
  cancelled_reason?: string
  items: OrderItem[]
  created_at: string
  updated_at: string
}

export interface OrderSummary {
  today: number
  status_counts: Record<OrderStatus, number>
  total_value: number
}

// ─── Shipment (TASK-027/028) ─────────────────────────────────────────────

/** Status pengiriman kanonik — sama dengan CHECK di migrasi 0016. */
export type ShipmentLifecycleStatus =
  | 'draft'
  | 'pending_pickup'
  | 'picked_up'
  | 'in_transit'
  | 'out_for_delivery'
  | 'delivered'
  | 'failed'
  | 'returned'
  | 'cancelled'

export interface ShipmentParty {
  code: string
  service: string
}

export interface ShipmentPackageDetail {
  actual_weight: number
  volumetric_weight: number
  chargeable_weight: number
  length?: number | null
  width?: number | null
  height?: number | null
  type?: string
}

export interface ShipmentPricing {
  shipping_price: number
  cost_price: number
  insurance_amount: number
  cod_fee: number
  is_cod: boolean
  cod_amount: number
  declared_value: number
}

/** Payload label yang melekat pada shipment (QR & barcode untuk cetak). */
export interface ShipmentLabelPackage {
  id?: string
  sequence: number
  weight: number
  qr_payload: string
  barcode_value: string
}

/** Pengiriman seperti dikembalikan API (bentuk ringkas + detail). */
export interface Shipment {
  id: string
  awb: string
  status: ShipmentLifecycleStatus
  courier: ShipmentParty
  order_id?: string
  store_id?: string
  warehouse_id?: string
  origin_address_id?: string
  destination_address_id?: string
  package: ShipmentPackageDetail
  pricing: ShipmentPricing
  eta_min_days: number
  eta_max_days: number
  qr_payload: string
  barcode_value: string
  label_url: string
  items: OrderItem[] | Array<{ name_snapshot: string, qty: number }>
  packages: ShipmentLabelPackage[]
  notes?: string
  created_at: string
  updated_at: string
}

/** Request pembuatan shipment (POST /shipments, butuh Idempotency-Key). */
export interface ShipmentCreateInput {
  order_id?: string
  store_id?: string
  warehouse_id?: string
  courier_code: string
  service_code: string
  origin_address_id: string
  destination_address_id: string
  weight: number
  length?: number
  width?: number
  height?: number
  package_type?: string
  is_cod?: boolean
  cod_amount?: number
  declared_value?: number
  insurance?: boolean
  notes?: string
  items?: Array<{ order_item_id?: string, variant_id?: string, name: string, qty: number }>
}

// ─── Courier registry (TASK-022) ─────────────────────────────────────────

/** Kurir terdaftar di platform. */
export interface Courier {
  id: string
  code: string
  name: string
  adapter_type: string
  is_active: boolean
  logo_url?: string
  created_at: string
  updated_at: string
}

/** Layanan pengiriman satu kurir (REG, YES, BEST, ...). */
export interface CourierService {
  id: string
  courier_id: string
  code: string
  name: string
  supports_cod: boolean
  supports_pickup: boolean
  supports_insurance: boolean
  eta_min_days: number
  eta_max_days: number
  reliability_score: number
  is_active: boolean
  created_at: string
  updated_at: string
}
