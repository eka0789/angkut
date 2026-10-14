// Composable untuk perbandingan tarif kurir (TASK-024/025).
// Semua endpoint di sini read-only dan tenant-scoped.
import type { RateOption, RateQuote, RateRecommendResponse } from '~/types/api'

/** Parameter pencarian tarif — satu bentuk untuk GET, preview, dan recommend. */
export interface RateQueryInput {
  originCityId: string
  destinationCityId: string
  weight: number
  dimensions?: { length: number, width: number, height: number }
  isCOD?: boolean
  codAmount?: number
  insured?: boolean
  declaredValue?: number
}

/** Mengubah parameter menjadi map query string (hanya field terisi). */
function toQuery(input: RateQueryInput): Record<string, string | undefined> {
  const query: Record<string, string | undefined> = {
    origin_city_id: input.originCityId,
    destination_city_id: input.destinationCityId,
    weight: String(input.weight),
  }
  if (input.dimensions) {
    query.length = String(input.dimensions.length)
    query.width = String(input.dimensions.width)
    query.height = String(input.dimensions.height)
  }
  if (input.isCOD) {
    query.is_cod = 'true'
    query.cod_amount = String(input.codAmount ?? 0)
  }
  if (input.insured) {
    query.insurance = 'true'
    query.declared_value = String(input.declaredValue ?? 0)
  }
  return query
}

export function useCouriersRates() {
  const { apiFetch } = useApi()

  /** GET /couriers/rates — perbandingan tarif lengkap dengan skor & label. */
  async function getRates(input: RateQueryInput): Promise<RateQuote> {
    return apiFetch<RateQuote>('/api/v1/couriers/rates', { query: toQuery(input) })
  }

  /**
   * POST /couriers/rates/preview — bentuknya sama dengan GET, dipakai untuk
   * paket besar yang parameter dimensinya lebih nyaman dikirim sebagai body.
   */
  async function getRatePreview(input: RateQueryInput): Promise<RateQuote> {
    return apiFetch<RateQuote>('/api/v1/couriers/rates/preview', {
      method: 'POST',
      body: toQuery(input),
    })
  }

  /** POST /couriers/recommend — pemenang tunggal beserta alasan & keyakinan. */
  async function getRateRecommend(quoteId: string): Promise<RateRecommendResponse> {
    return apiFetch<RateRecommendResponse>('/api/v1/couriers/recommend', {
      method: 'POST',
      body: { quote_id: quoteId },
    })
  }

  /** Mencari opsi yang direkomendasikan dari daftar hasil (fallback UI). */
  function recommendedOption(options: RateOption[]): RateOption | null {
    return options.find(o => o.is_recommended) ?? null
  }

  return { getRates, getRatePreview, getRateRecommend, recommendedOption }
}