// Konfigurasi Nuxt 4 — Angkut.
// SSR aktif dengan beberapa halaman CSR-only (peta & chart berat).
export default defineNuxtConfig({
  compatibilityDate: '2026-01-01',
  devtools: { enabled: true },

  // Nuxt 4: kode aplikasi berada di app/ (srcDir default).
  future: { compatibilityVersion: 4 },

  modules: ['@nuxt/ui', '@nuxt/eslint'],

  css: ['~/assets/css/main.css'],

  runtimeConfig: {
    public: {
      // Diisi dari NUXT_PUBLIC_API_BASE_URL saat build/run.
      apiBaseUrl: process.env.NUXT_PUBLIC_API_BASE_URL || 'http://localhost:8080',
      appName: 'Angkut',
      defaultLocale: 'id',
    },
  },

  app: {
    head: {
      htmlAttrs: { lang: 'id' },
      // Judul halaman selalu ditulis per halaman; template ini menambahkan
      // sufiks merek secara konsisten.
      titleTemplate: '%s · Angkut',
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'description', content: 'Platform logistik cerdas untuk bisnis Indonesia: bandingkan kurir, kirim, lacak, dan kelola COD dalam satu tempat.' },
        { name: 'theme-color', content: '#1F5EFF' },
      ],
      link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
    },
  },

  // Transpile paket yang butuh disesuaikan saat develop.
  build: { transpile: [] },

  nitro: {
    compressPublicAssets: true,
  },

  typescript: {
    strict: true,
    typeCheck: false, // dijalankan terpisah: npm run typecheck
  },
})
