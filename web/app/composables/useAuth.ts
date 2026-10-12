// useAuth — aksi autentikasi + status sesi untuk halaman frontend.
//
// Semua aksi memanggil endpoint backend yang sudah ada (lihat
// api/internal/interfaces/http/router.go §registerAuthRoutes); tidak ada
// logika bisnis di sini, hanya pemetaan DTO → sesi tersimpan.
import type {
  MFARequiredResponse,
  RegisterResponse,
  SessionResponse,
  StoredSession,
} from '~/types/auth'

export function useAuth() {
  const session = useState<StoredSession | null>('auth:session', () => null)
  const { apiFetch, adoptSession, dropSession } = useApi()

  const isAuthenticated = computed(() => session.value !== null)

  /** Pendaftaran. Selalu berhasil → `next_step: verify_email`. */
  function register(input: {
    name: string
    email: string
    password: string
    organizationName: string
  }) {
    return apiFetch<RegisterResponse>('/api/v1/auth/register', {
      method: 'POST',
      body: {
        name: input.name,
        email: input.email,
        password: input.password,
        organization_name: input.organizationName,
      },
      auth: false,
    })
  }

  /**
   * Login. Bila 2FA aktif, backend mengembalikan `mfa_required` +
   * `mfa_token` (bukan sesi) — pemanggil lanjut ke verifyMfa().
   */
  async function login(email: string, password: string): Promise<
    { mfaRequired: true, mfaToken: string } | { mfaRequired: false }
  > {
    const res = await apiFetch<SessionResponse | MFARequiredResponse>(
      '/api/v1/auth/login',
      {
        method: 'POST',
        body: { email, password, device_name: browserDeviceName() },
        auth: false,
      },
    )
    if ('mfa_required' in res && res.mfa_required) {
      return { mfaRequired: true as const, mfaToken: res.mfa_token }
    }
    adoptSession(res as SessionResponse)
    return { mfaRequired: false as const }
  }

  /** Menyelesaikan login yang tertahan 2FA dengan kode TOTP/recovery. */
  async function verifyMfa(mfaToken: string, code: string) {
    const res = await apiFetch<SessionResponse>('/api/v1/auth/2fa/verify', {
      method: 'POST',
      body: { mfa_token: mfaToken, code, device_name: browserDeviceName() },
      auth: false,
    })
    adoptSession(res)
  }

  /** Memverifikasi token dari email. */
  function verifyEmail(token: string) {
    return apiFetch<null>('/api/v1/auth/verify-email', {
      method: 'POST',
      body: { token },
      auth: false,
    })
  }

  /** Meminta tautan reset. Respons selalu sukses (anti email enumeration). */
  function forgotPassword(email: string) {
    return apiFetch<null>('/api/v1/auth/forgot-password', {
      method: 'POST',
      body: { email },
      auth: false,
    })
  }

  /** Menukar token reset dengan password baru. */
  function resetPassword(token: string, password: string) {
    return apiFetch<null>('/api/v1/auth/reset-password', {
      method: 'POST',
      body: { token, password },
      auth: false,
    })
  }

  /** Mengirim ulang email verifikasi. */
  function resendVerification(email: string) {
    return apiFetch<null>('/api/v1/auth/resend-verification', {
      method: 'POST',
      body: { email },
      auth: false,
    })
  }

  /** Keluar: cabut sesi di server lalu hapus sesi lokal. */
  async function logout() {
    try {
      await apiFetch<null>('/api/v1/auth/logout', {
        method: 'POST',
        body: { refresh_token: session.value?.refresh_token ?? '' },
        retryOn401: false,
      })
    } catch {
      // Server tidak terjangkau tetap harus mengakhiri sesi lokal.
    }
    dropSession()
  }

  /** Menandai sesi berhasil dimuat dari localStorage (dipanggil plugin). */
  function hydrate(stored: StoredSession | null) {
    session.value = stored
  }

  return {
    session,
    isAuthenticated,
    register,
    login,
    verifyMfa,
    verifyEmail,
    forgotPassword,
    resetPassword,
    resendVerification,
    logout,
    hydrate,
  }
}

/** Nama perangkat untuk daftar sesi — cukup kasar, bukan fingerprint. */
function browserDeviceName(): string {
  if (!import.meta.client) return 'web'
  const ua = navigator.userAgent
  const browser = /Edg\//.test(ua) ? 'Edge'
    : /Chrome\//.test(ua) ? 'Chrome'
      : /Firefox\//.test(ua) ? 'Firefox'
        : /Safari\//.test(ua) ? 'Safari' : 'Browser'
  const os = /Windows/.test(ua) ? 'Windows'
    : /Mac OS/.test(ua) ? 'macOS'
      : /Android/.test(ua) ? 'Android'
        : /iPhone|iPad/.test(ua) ? 'iOS'
          : /Linux/.test(ua) ? 'Linux' : ''
  return os ? `${browser} on ${os}` : browser
}