// Middleware: halaman tamu (login/register/lupa sandi) — bila sudah punya
// sesi, alihkan ke dasbor agar tidak melihat form login lagi.
import type { StoredSession } from '~/types/auth'

export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return

  const session = useState<StoredSession | null>('auth:session', () => null)
  if (session.value !== null) {
    return navigateTo('/dashboard')
  }
})