// Plugin: memuat sesi tersimpan (localStorage) ke state global SEBELUM
// navigasi pertama, supaya middleware `require-auth` membaca nilai yang benar.
// Hanya dijalankan di client — server tidak punya localStorage.
export default defineNuxtPlugin(() => {
  const { hydrate } = useAuth()
  hydrate(readStoredSession())
})