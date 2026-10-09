// Kamus bahasa Indonesia — default aplikasi. Kunci bersifat datar dan
// bercabang dengan titik (mis. 'auth.login') agar mudah dicari lewat grep.
// Bahasa Inggris ditambahkan sebagai kamus kedua saat i18n resmi dipasang.
export const id = {
  'app.name': 'Angkut',
  'app.tagline': 'Ship smarter. Grow faster.',

  // Navigasi
  'nav.dashboard': 'Dasbor',
  'nav.operations': 'Operasional',
  'nav.shipments': 'Pengiriman',
  'nav.tracking': 'Pelacakan',
  'nav.pickup': 'Penjemputan',
  'nav.commandCenter': 'Pusat Kendali',
  'nav.catalog': 'Katalog',
  'nav.orders': 'Pesanan',
  'nav.inventory': 'Stok',
  'nav.warehouses': 'Gudang',
  'nav.customers': 'Pelanggan',
  'nav.money': 'Keuangan',
  'nav.cod': 'COD',
  'nav.settlements': 'Penyelesaian',
  'nav.analytics': 'Analitik',
  'nav.intelligence': 'Kecerdasan',
  'nav.aiAssistant': 'Asisten AI',
  'nav.settings': 'Pengaturan',
  'nav.features': 'Fitur',
  'nav.pricing': 'Harga',
  'nav.trackPackage': 'Lacak Paket',

  // Auth
  'auth.login': 'Masuk',
  'auth.register': 'Daftar gratis',
  'auth.logout': 'Keluar',
  'auth.email': 'Email',
  'auth.password': 'Kata sandi',
  'auth.forgotPassword': 'Lupa kata sandi?',

  // Umum
  'common.loading': 'Memuat…',
  'common.empty': 'Belum ada data',
  'common.error': 'Terjadi kesalahan',
  'common.retry': 'Coba lagi',
  'common.search': 'Cari',
  'common.save': 'Simpan',
  'common.cancel': 'Batal',
  'common.confirm': 'Konfirmasi',
  'common.next': 'Lanjut',
  'common.back': 'Kembali',
  'common.status': 'Status',
  'common.actions': 'Aksi',

  // Status pengiriman (kanonik)
  'status.pending': 'Menunggu',
  'status.picked_up': 'Dijemput',
  'status.in_transit': 'Dalam perjalanan',
  'status.out_for_delivery': 'Sedang diantar',
  'status.delivered': 'Terkirim',
  'status.failed': 'Gagal',
  'status.returned': 'Dikembalikan',
  'status.cancelled': 'Dibatalkan',

  // Halaman
  'dashboard.title': 'Dasbor',
  'dashboard.subtitle': 'Ringkasan operasional Anda hari ini.',
  'dashboard.emptyTitle': 'Operasional Anda akan tampil di sini',
  'dashboard.emptyBody': 'Hubungkan toko atau buat pengiriman pertama untuk melihat ringkasan, biaya logistik, dan status COD.',
} as const

export type MessageKey = keyof typeof id
export type Dictionary = Record<string, string>