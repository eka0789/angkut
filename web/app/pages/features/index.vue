<script setup lang="ts">
// Halaman fitur publik (TASK-064): ringkasan kemampuan platform.
definePageMeta({ layout: 'public' })
useHead({ title: () => 'Fitur — Angkut' })

const features = [
  { icon: '▤', title: 'Pengiriman & Label', body: 'Buat pengiriman dengan AWB otomatis, label PDF (Code128 + QR) format A4 & thermal, idempotensi bawaan agar tidak ada pengiriman ganda.' },
  { icon: '⌖', title: 'Pelacakan Realtime', body: 'Timeline kejadian per resi, halaman lacak publik yang bisa dibagikan ke pembeli, dan pembaruan tanpa refresh.' },
  { icon: '₨', title: 'COD & Ledger', body: 'Tagihan COD otomatis per paket, settlement per kurir, buku besar append-only yang tidak bisa diubah, refund tercatat idempoten.' },
  { icon: '☰', title: 'Inventory Multi-Gudang', body: 'Produk & varian SKU, saldo per gudang (on-hand/reserved/available), riwayat movement append-only, transfer antar gudang atomik.' },
  { icon: '⛃', title: 'Pesanan & Pelanggan', body: 'Order dari toko, segmentasi pelanggan otomatis (VIP/regular/risiko), profil lengkap dengan metrik.' },
  { icon: '_bulk', title: 'Kirim Massal', body: 'Unggah CSV ratusan baris, validasi per baris, submit idempoten, unduh semua label sekaligus dalam satu ZIP.' },
  { icon: '✦', title: 'Asisten AI', body: 'Tanya biaya, status, COD — dijawab dari data organisasi Anda. Aksi sensitif selalu butuh konfirmasi Anda.' },
  { icon: '◔', title: 'Dasbor & Skor Kesehatan', body: 'KPI harian, tren biaya 30 hari, performa kurir, dan skor kesehatan logistik dengan rekomendasi.' },
]
</script>

<template>
  <div class="features">
    <header class="hero">
      <h1>Semua yang Anda butuhkan untuk mengelola pengiriman</h1>
      <p>Dari buat order sampai paket diterima — satu platform, satu sumber kebenaran.</p>
    </header>

    <section class="grid">
      <article
        v-for="f in features"
        :key="f.title"
        class="card"
      >
        <span
          class="icon"
          aria-hidden="true"
        >{{ f.icon === '_bulk' ? '⇪' : f.icon }}</span>
        <h2>{{ f.title }}</h2>
        <p>{{ f.body }}</p>
      </article>
    </section>

    <section class="cta">
      <h2>Siap mencoba?</h2>
      <div class="cta-actions">
        <NuxtLink
          to="/register"
          class="btn-primary"
        >
          Daftar gratis
        </NuxtLink>
        <NuxtLink
          to="/pricing"
          class="btn-secondary"
        >
          Lihat harga
        </NuxtLink>
      </div>
    </section>
  </div>
</template>

<style scoped>
.hero { text-align: center; padding: var(--space-10) var(--space-4) var(--space-8); }
.hero h1 { font-size: 32px; max-width: 22ch; margin: 0 auto var(--space-3); }
.hero p { color: var(--color-ink-secondary); font-size: var(--text-md); margin: 0; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-4);
  max-width: 1100px;
  margin: 0 auto var(--space-10);
  padding: 0 var(--space-4);
}
.card { padding: var(--space-5); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-surface); }
.icon { font-size: 26px; margin-bottom: var(--space-3); display: inline-block; }
.card h2 { font-size: var(--text-md); margin: 0 0 var(--space-2); }
.card p { margin: 0; color: var(--color-ink-secondary); font-size: var(--text-sm); line-height: var(--leading-relaxed); }

.cta { text-align: center; padding: var(--space-8) var(--space-4); border-top: 1px solid var(--color-border); }
.cta h2 { margin-bottom: var(--space-4); }
.cta-actions { display: flex; gap: var(--space-3); justify-content: center; flex-wrap: wrap; }
.btn-primary, .btn-secondary {
  padding: var(--space-3) var(--space-6);
  border-radius: var(--radius-md);
  font-weight: 600;
  text-decoration: none;
}
.btn-primary { background: var(--color-primary); color: var(--color-ink-inverse); }
.btn-secondary { background: var(--color-surface); color: var(--color-ink); border: 1px solid var(--color-border); }
</style>
