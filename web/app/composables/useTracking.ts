// useTracking — timeline pelacakan (TASK-030/034/035).
//
// Dua jalur akses dengan bentuk respons IDENTIK:
//   - tenant  : GET /api/v1/tracking/{awb}        (butuh token + tenant)
//   - publik  : GET /api/v1/public/track/{awb}    (tanpa auth, rate-limited)
//
// Ditambah `streamURL` untuk EventSource: browser tidak bisa menempel header
// Authorization pada EventSource, jadi token dikirim lewat query — endpoint
// backend memang menerimanya (middleware AuthQuery) khusus untuk streaming.
import type { TrackingEventStatus, TrackingTimeline } from '~/types/api'

/** Input pencatatan kejadian manual oleh staf (POST /tracking/events). */
export interface TrackingAppendInput {
  shipment_id: string
  status: TrackingEventStatus
  description?: string
  location?: string
  occurred_at?: string
}

export interface TrackingAppendResult {
  duplicate: boolean
  status_changed: boolean
  shipment_status: string
}

export function useTracking() {
  const { apiFetch } = useApi()
  const config = useRuntimeConfig()
  const session = useState<{ access_token?: string } | null>('auth:session')

  /** Timeline untuk pengguna yang login (tenant-scoped). */
  async function byAWB(awb: string): Promise<TrackingTimeline> {
    return apiFetch<TrackingTimeline>(`/api/v1/tracking/${encodeURIComponent(awb)}`)
  }

  /** Timeline publik — dipakai halaman /track yang tidak butuh akun. */
  async function publicByAWB(awb: string): Promise<TrackingTimeline> {
    return apiFetch<TrackingTimeline>(`/api/v1/public/track/${encodeURIComponent(awb)}`, {
      auth: false,
      retryOn401: false,
    })
  }

  /**
   * URL EventSource untuk resi tertentu, atau null bila belum ada sesi.
   *
   * Catatan keamanan: access token muncul di URL. Ini diterima karena (a) TTL
   * token pendek, (b) endpoint hanya untuk streaming, dan (c) logger akses
   * backend hanya mencatat path. Halaman publik TIDAK memakai ini.
   */
  function streamURL(awb: string): string | null {
    const token = session.value?.access_token
    if (!token) return null
    const base = String(config.public.apiBaseUrl || '').replace(/\/$/, '')
    return `${base}/api/v1/tracking/${encodeURIComponent(awb)}/stream?token=${encodeURIComponent(token)}`
  }

  /**
   * Mencatat kejadian manual. Server mengembalikan `duplicate: true` bila
   * kejadian identik sudah ada — itu bukan error, jadi pemanggil tidak perlu
   * menampilkannya sebagai kegagalan.
   */
  async function append(input: TrackingAppendInput): Promise<TrackingAppendResult> {
    return apiFetch<TrackingAppendResult>('/api/v1/tracking/events', {
      method: 'POST',
      body: { ...input },
    })
  }

  return { byAWB, publicByAWB, streamURL, append }
}
