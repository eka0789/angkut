// Middleware: halaman yang butuh sesi (settings, dasbor terproteksi).
// Di server tidak ada localStorage → biarkan render; pengalihan dilakukan di
// client saat sesi benar-benar terbaca.
import type { StoredSession } from '~/types/auth'

export default defineNuxtRouteMiddleware((to) => {
  if (import.meta.server) return

  const session = useState<StoredSession | null>('auth:session', () => null)
  if (session.value === null) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }
})