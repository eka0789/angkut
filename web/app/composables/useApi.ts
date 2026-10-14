// useApi — klien API tipis di atas $fetch.
//
// Tanggung jawab: (1) menempel header otorisasi + tenant, (2) meng-unwrapping
// envelope { success, data | error }, (3) menormalkan kegagalan menjadi
// ApiRequestError ber-code stabil. Tidak ada logika bisnis di sini.
//
// $fetch diimpor dari ofetch secara langsung (bukan global Nuxt) karena
// backend adalah server eksternal — pemetaan rute typed Nuxt tidak relevan
// dan justru memicu TS2321 pada path dinamis.
import { $fetch } from 'ofetch'
import type { ApiErrorBody } from '~/types/api'
import type { StoredSession } from '~/types/auth'

/** Error ternormalisasi dari API; `code` sama dengan kode envelope backend. */
export class ApiRequestError extends Error {
  readonly code: string
  readonly status: number
  readonly fields?: Record<string, string>

  constructor(status: number, body: ApiErrorBody) {
    super(body.message)
    this.name = 'ApiRequestError'
    this.code = body.code
    this.status = status
    this.fields = body.fields
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'
  body?: Record<string, unknown>
  query?: Record<string, string | number | boolean | undefined>
  /** Set false untuk endpoint auth publik (tanpa header Authorization). */
  auth?: boolean
  /** Set true untuk memicu refresh token otomatis saat 401 (sekali coba). */
  retryOn401?: boolean
  /**
   * Header tambahan (mis. Idempotency-Key). Header otorisasi & tenant ditulis
   * SETELAH ini sehingga pemanggil tidak dapat menimpanya.
   */
  headers?: Record<string, string>
}

/** Simpan/muat sesi. localStorage hanya dibaca di client; SSR pakai useState. */
const SESSION_KEY = 'angkut_session'

export function readStoredSession(): StoredSession | null {
  if (!import.meta.client) return null
  try {
    const raw = localStorage.getItem(SESSION_KEY)
    return raw ? (JSON.parse(raw) as StoredSession) : null
  } catch {
    return null
  }
}

export function writeStoredSession(session: StoredSession): void {
  if (!import.meta.client) return
  localStorage.setItem(SESSION_KEY, JSON.stringify(session))
}

export function clearStoredSession(): void {
  if (!import.meta.client) return
  localStorage.removeItem(SESSION_KEY)
}

export function useApi() {
  const config = useRuntimeConfig()
  const session = useState<StoredSession | null>('auth:session', () => null)

  /** Envelope mentah sebelum di-unwrap — dipakai bila meta dibutuhkan. */
  interface RawEnvelope<T> {
    success: boolean
    data?: T
    meta?: Record<string, unknown>
    message?: string
    error?: ApiErrorBody
  }

  async function requestEnvelope<T>(
    path: string,
    options: RequestOptions,
    allowRefresh: boolean,
  ): Promise<RawEnvelope<T>> {
    const { method = 'GET', body, query, auth = true, retryOn401 = true, headers: extraHeaders } = options

    const headers: Record<string, string> = { ...extraHeaders }
    if (auth && session.value?.access_token) {
      headers.Authorization = `Bearer ${session.value.access_token}`
      if (session.value.active_org_id) {
        headers['X-Organization-Id'] = session.value.active_org_id
      }
    }

    let res: RawEnvelope<T>
    try {
      res = await $fetch(path, {
        baseURL: config.public.apiBaseUrl as string,
        method,
        body,
        query,
        headers,
      })
    } catch (err: unknown) {
      // $fetch melempar FetchError berisi respons tidak-2xx; envelope error
      // ada di err.data (bukan err.error) pada versi ofetch saat ini.
      const status = (err as { statusCode?: number, status?: number })?.statusCode
        ?? (err as { status?: number })?.status
        ?? 0
      const envelope = (err as { data?: { error?: ApiErrorBody } })?.data
      if (envelope?.error) {
        // 401 di endpoint terproteksi: coba tukar refresh token sekali lalu
        // ulangi request asli. Bila refresh gagal, jatuhkan sesi (di bawah).
        if (status === 401 && auth && retryOn401 && allowRefresh) {
          const refreshed = await tryRefresh()
          if (refreshed) {
            return requestEnvelope<T>(path, options, false)
          }
          dropSession()
        }
        throw new ApiRequestError(status, envelope.error)
      }
      // Kegagalan jaringan / respons bukan-JSON: tetap beri code stabil.
      throw new ApiRequestError(status, {
        code: status === 0 ? 'NETWORK_ERROR' : 'BAD_RESPONSE',
        message: status === 0
          ? 'Tidak dapat terhubung ke server. Periksa koneksi Anda.'
          : 'Respons server tidak dapat diproses.',
      })
    }

    if (!res.success) {
      throw new ApiRequestError(0, {
        code: 'BAD_RESPONSE',
        message: res.message ?? 'Respons server tidak valid.',
      })
    }
    return res
  }

  async function apiFetch<T>(
    path: string,
    options: RequestOptions = {},
    allowRefresh = true,
  ): Promise<T> {
    const res = await requestEnvelope<T>(path, options, allowRefresh)
    return res.data as T
  }

  /** Sama seperti apiFetch, tetapi juga mengembalikan meta paginasi. */
  async function apiFetchWithMeta<T>(
    path: string,
    options: RequestOptions = {},
    allowRefresh = true,
  ): Promise<{ data: T, meta?: Record<string, unknown> }> {
    const res = await requestEnvelope<T>(path, options, allowRefresh)
    return { data: res.data as T, meta: res.meta }
  }

  /** Menukar refresh token lama dengan sesi baru. Gagal → false. */
  async function tryRefresh(): Promise<boolean> {
    const current = session.value ?? readStoredSession()
    if (!current?.refresh_token) return false
    try {
      const res = await apiFetch<import('~/types/auth').SessionResponse>(
        '/api/v1/auth/refresh',
        { method: 'POST', body: { refresh_token: current.refresh_token }, auth: false },
        false,
      )
      adoptSession(res)
      return true
    } catch {
      return false
    }
  }

  /** Menulis sesi baru (dipakai login/refresh/2FA) ke state + localStorage. */
  function adoptSession(res: import('~/types/auth').SessionResponse): StoredSession {
    const stored: StoredSession = {
      access_token: res.access_token,
      refresh_token: res.refresh_token,
      expires_at: Date.now() + res.expires_in * 1000,
      user: res.user,
      active_org_id: res.active_organization_id ?? res.organizations[0]?.id ?? '',
      active_role: res.active_role ?? res.organizations[0]?.role ?? '',
      organizations: res.organizations,
    }
    session.value = stored
    writeStoredSession(stored)
    return stored
  }

  /** Menjatuhkan sesi lokal (token kedaluwarsa / logout). */
  function dropSession(): void {
    session.value = null
    clearStoredSession()
  }

  /**
   * Mengambil berkas biner (mis. label PDF) dengan header otorisasi + tenant.
   * Berbeda dari apiFetch: TIDAK membuka envelope JSON — responsnya blob.
   * Nama berkas diambil dari Content-Disposition bila ada.
   */
  async function apiFetchBlob(path: string): Promise<{ blob: Blob, filename: string | null }> {
    const headers: Record<string, string> = {}
    if (session.value?.access_token) {
      headers.Authorization = `Bearer ${session.value.access_token}`
      if (session.value.active_org_id) {
        headers['X-Organization-Id'] = session.value.active_org_id
      }
    }
    const res = await $fetch.raw(path, {
      baseURL: config.public.apiBaseUrl as string,
      method: 'GET',
      headers,
      responseType: 'blob',
    })
    const filename = parseFilename(res.headers.get('content-disposition'))
    return { blob: res._data as Blob, filename }
  }

  return { apiFetch, apiFetchWithMeta, apiFetchBlob, adoptSession, dropSession, tryRefresh }
}

/** Mengambil nama berkas dari header Content-Disposition, bila ada. */
function parseFilename(disposition: string | null): string | null {
  if (!disposition) return null
  const match = /filename="?([^";]+)"?/i.exec(disposition)
  return match?.[1] ?? null
}