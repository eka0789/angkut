// Middleware rute: memastikan catch-all hanya melayani rute modul yang memang
// direncanakan. Path di luar daftar itu benar-benar tidak ada → 404.
// Dipasang lewat definePageMeta (bukan throw di setup) agar pasti dievaluasi
// oleh router sebelum render.
export default defineNuxtRouteMiddleware((to) => {
  if (!isPlannedModulePath(to.path)) {
    return abortNavigation(
      createError({ statusCode: 404, statusMessage: 'Halaman tidak ditemukan' }),
    )
  }
})
