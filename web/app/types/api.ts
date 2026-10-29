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

// ─── Tracking (TASK-030/034/035) ──────────────────────────────────────────

/** Kejadian pelacakan seperti dilaporkan kurir atau staf. */
export type TrackingEventStatus =
  | 'created'
  | 'pickup_requested'
  | 'picked_up'
  | 'sorting'
  | 'in_transit'
  | 'out_for_delivery'
  | 'delivered'
  | 'failed'
  | 'returned'

export interface TrackingEvent {
  id: string
  shipment_id: string
  status: TrackingEventStatus
  description?: string
  location?: string
  latitude?: number | null
  longitude?: number | null
  occurred_at: string
  source: 'webhook' | 'manual' | 'system' | 'sync'
  created_at: string
}

/** Penilaian apakah pengiriman masih sesuai jadwal. */
export interface TrackingConfidence {
  level: 'on_track' | 'delayed' | 'unknown'
  message: string
  reason?: string
}

/**
 * Timeline satu resi — bentuk yang SAMA dipakai endpoint tenant, publik,
 * snapshot SSE, dan pesan realtime. FE tidak perlu menangani dua bentuk.
 */
export interface TrackingTimeline {
  awb: string
  status: ShipmentLifecycleStatus
  courier: ShipmentParty
  events: TrackingEvent[]
  last_location?: string
  last_lat?: number | null
  last_lng?: number | null
  last_occurred_at?: string | null
  eta_min_days: number
  eta_max_days: number
  confidence: TrackingConfidence
  shipment_id: string
}

// ─── Bulk shipping (TASK-039) ────────────────────────────────────────────

/** Satu baris input pengiriman massal (dari CSV atau JSON). */
export interface BulkRow {
  recipient_name: string
  recipient_phone: string
  street: string
  postal_code: string
  weight_kg: number
  length_cm?: number | null
  width_cm?: number | null
  height_cm?: number | null
  cod_amount: number
  declared_value: number
  insurance: boolean
  notes?: string
}

/** Satu masalah validasi pada sebuah baris. */
export interface BulkIssue {
  field: string
  message: string
  /** error = baris ditolak; warning = baris tetap jalan dengan catatan. */
  severity?: 'error' | 'warning'
}

/** Hasil validasi satu baris. */
export interface BulkRowReport {
  row_no: number
  status: 'valid' | 'warning' | 'error'
  issues?: BulkIssue[]
  row: BulkRow
}

/** Laporan validasi seluruh baris sebelum submit. */
export interface BulkReport {
  total: number
  valid: number
  warning: number
  error: number
  rows: BulkRowReport[]
}

/** Hasil unggahan CSV (POST /shipments/bulk/upload). */
export interface BulkUploadResult {
  upload_id: string
  row_count: number
  rows: BulkRow[]
}

/** Hasil submit (202 Accepted). */
export interface BulkSubmitResult {
  job_id: string
  accepted: number
  rejected: number
  replayed: boolean
}

export type BulkJobState = 'pending' | 'processing' | 'completed' | 'failed'
export type BulkJobRowState = 'valid' | 'invalid' | 'creating' | 'created' | 'failed'

/** Status job massal + hasil per baris (GET /shipments/bulk/{job_id}). */
export interface BulkJob {
  id: string
  status: BulkJobState
  courier_code: string
  service_code: string
  origin_warehouse_id: string
  total_rows: number
  accepted_rows: number
  rejected_rows: number
  processed_rows: number
  created_rows: number
  failed_rows: number
  error_summary?: string
  created_at: string
  started_at?: string
  finished_at?: string
  rows?: BulkJobRowResult[]
}

/** Hasil eksekusi satu baris dalam job. */
export interface BulkJobRowResult {
  job_id: string
  row_no: number
  row: BulkRow
  status: BulkJobRowState
  issues?: BulkIssue[]
  shipment_id?: string
  awb?: string
  error?: string
  created_at: string
}

// ─── Notification (TASK-040) ─────────────────────────────────────────────

export type NotificationChannel = 'inapp' | 'email'
export type NotificationPriority = 'low' | 'normal' | 'high'
export type NotificationEventKey =
  | 'shipment.created'
  | 'shipment.picked_up'
  | 'shipment.in_transit'
  | 'shipment.delivered'
  | 'shipment.failed'
  | 'shipment.returned'
  | 'shipment.cancelled'
  | 'bulk.job_completed'

/** Notifikasi in-app milik satu pengguna. */
export interface AppNotification {
  id: string
  organization_id: string
  user_id: string
  event_key: NotificationEventKey
  channel: NotificationChannel
  priority: NotificationPriority
  title: string
  body: string
  data?: Record<string, unknown>
  read_at?: string | null
  created_at: string
}

/** Template notifikasi efektif (kustom organisasi atau bawaan platform). */
export interface NotificationTemplateView {
  key: NotificationEventKey
  title_template: string
  body_template: string
  custom: boolean
}

/** Preferensi efektif satu baris (event × kanal). */
export interface NotificationPreferenceView {
  event_key: NotificationEventKey
  channel: NotificationChannel
  enabled: boolean
  explicit: boolean
}

// ─── Inventory & Produk (TASK-037/038) ───────────────────────────────────

export interface ProductVariant {
  id: string
  organization_id: string
  product_id: string
  sku: string
  name?: string
  weight_kg?: number | null
  length_cm?: number | null
  width_cm?: number | null
  height_cm?: number | null
  low_stock_threshold: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface Product {
  id: string
  organization_id: string
  name: string
  category?: string
  weight_kg?: number | null
  length_cm?: number | null
  width_cm?: number | null
  height_cm?: number | null
  is_active: boolean
  variants?: ProductVariant[]
  created_at: string
  updated_at: string
}

export interface InventoryStock {
  id: string
  organization_id: string
  variant_id: string
  warehouse_id: string
  qty_on_hand: number
  qty_reserved: number
  version: number
  sku: string
  product_name: string
  variant_name?: string
  low_stock_threshold: number
}

export type InventoryMovementType =
  | 'adjust'
  | 'transfer_out'
  | 'transfer_in'
  | 'reserve'
  | 'release'

export interface InventoryMovement {
  id: string
  organization_id: string
  variant_id: string
  warehouse_id: string
  type: InventoryMovementType
  qty_delta: number
  qty_after: number
  reason?: string
  transfer_id?: string
  created_by?: string
  created_at: string
}

export interface LowStockRow {
  variant_id: string
  sku: string
  product_name: string
  warehouse_id: string
  qty_available: number
  low_stock_threshold: number
}

export interface StockTransferItem {
  variant_id: string
  qty: number
}

export interface StockTransfer {
  id: string
  organization_id: string
  from_warehouse_id: string
  to_warehouse_id: string
  notes?: string
  items: StockTransferItem[]
  created_at: string
}

// ─── Money: COD, ledger, returns (TASK-041..046) ─────────────────────────

export type CODStatus = 'pending' | 'collected' | 'settling' | 'settled' | 'failed'

export interface CODTransaction {
  id: string
  organization_id: string
  shipment_id: string
  courier_code: string
  amount: number
  fee: number
  status: CODStatus
  collected_at?: string | null
  created_at: string
}

export interface CODSummary {
  total: number
  pending: number
  collected: number
  settling: number
  settled: number
  expected_settlement: number
  count: number
}

export interface CODSettlement {
  id: string
  courier_code: string
  period_start: string
  period_end: string
  status: 'pending' | 'paid'
  total_amount: number
  total_fee: number
  net_amount: number
  reference?: string
  paid_at?: string | null
  items?: CODTransaction[]
  created_at: string
}

export interface LedgerEntry {
  id: string
  organization_id: string
  entry_date: string
  account: string
  debit: number
  credit: number
  ref_type: string
  ref_id: string
  memo?: string
  created_at: string
}

export interface FinancialReport {
  shipping_expense: number
  courier_fees: number
  insurance: number
  refunds: number
  cod_revenue: number
  payment_received: number
  net_logistics_cost: number
}

export type ReturnStatus =
  | 'requested' | 'approved' | 'rejected'
  | 'received' | 'inspected' | 'refunded'

export interface ReturnItem {
  id: string
  order_item_id?: string
  name_snapshot?: string
  qty: number
  condition: 'unknown' | 'good' | 'damaged' | 'lost'
  restocked: boolean
}

export interface ProductReturn {
  id: string
  organization_id: string
  order_id?: string
  shipment_id?: string
  type: 'refund' | 'exchange'
  reason?: string
  status: ReturnStatus
  refund_amount: number
  refund_method?: string
  items?: ReturnItem[]
  created_at: string
}

export interface ReturnPolicy {
  window_days: number
  auto_approve_limit: number
  conditions?: string[]
}
