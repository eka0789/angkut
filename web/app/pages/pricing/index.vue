<script setup lang="ts">
import AppIcon from '~/components/AppIcon.vue'
// Halaman harga publik (TASK-064): tiga paket sesuai seed plan (free/growth/scale).
definePageMeta({ layout: 'public' })
useHead({ title: () => 'Harga — Angkut' })

const plans = [
  {
    name: 'Free', price: 'Rp 0', period: '/bulan', highlight: false,
    desc: 'Untuk mulai menjual dan mencoba platform.',
    features: ['100 pengiriman/bulan', '1 gudang', 'Pelacakan publik', 'Dasbor KPI dasar', 'Label PDF A4 & thermal'],
    cta: 'Mulai gratis', to: '/register',
  },
  {
    name: 'Growth', price: 'Rp 249.000', period: '/bulan', highlight: true,
    desc: 'Untuk online seller yang mulai tumbuh.',
    features: ['2.000 pengiriman/bulan', 'Gudang tak terbatas', 'Kirim massal CSV + ZIP label', 'COD + settlement + ledger', 'Insight AI & skor kesehatan', 'API key integrasi'],
    cta: 'Pilih Growth', to: '/register',
  },
  {
    name: 'Scale', price: 'Hubungi kami', period: '', highlight: false,
    desc: 'Untuk operasi logistik bervolume besar.',
    features: ['Pengiriman tak terbatas', 'Multi-cabang & multi-toko', 'Automation engine', 'SLA dukungan prioritas', 'Onboarding tim Anda'],
    cta: 'Hubungi sales', to: '/register',
  },
]

const faqs = [
  { q: 'Apakah ada biaya per pengiriman?', a: 'Tidak ada biaya tersembunyi per paket dari platform. Ongkos kirim mengikuti tarif kurir pilihan Anda.' },
  { q: 'Bisa upgrade/downgrade kapan saja?', a: 'Bisa. Perubahan paket berlaku pada periode tagihan berikutnya.' },
  { q: 'Apa yang terjadi jika kuota habis?', a: 'Pengiriman baru dibatasi sampai periode berikutnya — data Anda tetap aman dan bisa diakses.' },
]
</script>

<template>
  <div class="pricing">
    <header class="hero">
      <span class="hero-badge"><AppIcon :name='"tag"' :size="14" /> Harga jujur, tumbuh bersama Anda</span>
      <h1>Harga jujur, tumbuh bersama Anda</h1>
      <p>Mulai gratis. Upgrade saat volume Anda bertambah.</p>
    </header>

    <section class="grid">
      <article
        v-for="p in plans"
        :key="p.name"
        class="card"
        :class="{ highlight: p.highlight }"
      >
        <span
          v-if="p.highlight"
          class="badge"
        >Paling populer</span>
        <h2>{{ p.name }}</h2>
        <p class="price">
          {{ p.price }} <span class="period">{{ p.period }}</span>
        </p>
        <p class="desc">
          {{ p.desc }}
        </p>
        <ul>
          <li
            v-for="f in p.features"
            :key="f"
          >
            ✓ {{ f }}
          </li>
        </ul>
        <NuxtLink
          :to="p.to"
          class="cta"
          :class="{ primary: p.highlight }"
        >
          {{ p.cta }}
        </NuxtLink>
      </article>
    </section>

    <section class="faq">
      <h2>Pertanyaan umum</h2>
      <details
        v-for="f in faqs"
        :key="f.q"
      >
        <summary>{{ f.q }}</summary>
        <p>{{ f.a }}</p>
      </details>
    </section>
  </div>
</template>

<style scoped>
.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-4);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-xs);
  font-weight: 700;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  border: 1px solid color-mix(in srgb, var(--color-primary) 25%, transparent);
  border-radius: var(--radius-pill);
}
.hero { text-align: center; padding: var(--space-10) var(--space-4) var(--space-8); }
.hero h1 { font-size: 32px; margin: 0 0 var(--space-3); }
.hero p { color: var(--color-ink-secondary); margin: 0; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-4);
  max-width: 1000px;
  margin: 0 auto var(--space-10);
  padding: 0 var(--space-4);
  align-items: start;
}
.card { padding: var(--space-6) var(--space-5); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-surface); position: relative; }
.card.highlight { border-color: var(--color-primary); box-shadow: 0 8px 24px rgba(0,0,0,0.08); }
.badge {
  position: absolute; top: -12px; left: 50%; transform: translateX(-50%);
  padding: 2px var(--space-3); background: var(--color-cta, #f97316); color: #fff;
  border-radius: var(--radius-pill); font-size: var(--text-xs); font-weight: 600; white-space: nowrap;
}
.card h2 { margin: 0 0 var(--space-2); }
.price { font-size: 28px; font-weight: 800; margin: 0 0 var(--space-2); }
.period { font-size: var(--text-sm); font-weight: 400; color: var(--color-ink-secondary); }
.desc { color: var(--color-ink-secondary); font-size: var(--text-sm); margin: 0 0 var(--space-4); }
ul { list-style: none; margin: 0 0 var(--space-5); padding: 0; display: flex; flex-direction: column; gap: var(--space-2); font-size: var(--text-sm); }
.cta {
  display: block; text-align: center; padding: var(--space-3); border-radius: var(--radius-md);
  font-weight: 600; text-decoration: none;
  background: var(--color-surface); color: var(--color-ink); border: 1px solid var(--color-border);
}
.cta.primary { background: var(--color-primary); color: var(--color-ink-inverse); border-color: var(--color-primary); }

.faq { max-width: 720px; margin: 0 auto var(--space-10); padding: 0 var(--space-4); }
.faq h2 { text-align: center; margin-bottom: var(--space-5); }
details { border: 1px solid var(--color-border); border-radius: var(--radius-md); padding: var(--space-3) var(--space-4); margin-bottom: var(--space-2); background: var(--color-surface); }
summary { font-weight: 600; cursor: pointer; }
details p { color: var(--color-ink-secondary); font-size: var(--text-sm); margin: var(--space-2) 0 0; }
</style>
