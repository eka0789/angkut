<script setup lang="ts">
definePageMeta({ layout: 'public' })

const { t } = useT()

useHead({ title: 'Ship smarter. Grow faster.' })
</script>

<template>
  <div class="landing">
    <section class="hero">
      <p class="hero-eyebrow">
        Platform logistik cerdas untuk bisnis Indonesia
      </p>
      <h1 class="hero-title">
        {{ t('app.tagline') }}
      </h1>
      <p class="hero-sub">
        Bandingkan tarif kurir, kirim dalam jumlah banyak, lacak realtime, dan
        kelola uang COD — semuanya dengan asisten cerdas yang memahami bahasa Anda.
      </p>
      <div class="hero-actions">
        <NuxtLink
          to="/register"
          class="btn btn-primary"
        >
          {{ t('auth.register') }}
        </NuxtLink>
        <NuxtLink
          to="/track"
          class="btn btn-secondary"
        >
          {{ t('nav.trackPackage') }}
        </NuxtLink>
      </div>
    </section>

    <section class="highlights">
      <article
        v-for="item in highlights"
        :key="item.title"
        class="card highlight"
      >
        <h2 class="highlight-title">
          {{ item.title }}
        </h2>
        <p class="highlight-body">
          {{ item.body }}
        </p>
      </article>
    </section>
  </div>
</template>

<script lang="ts">
const highlights = [
  {
    title: 'Bandingkan semua kurir sekaligus',
    body: 'Harga, estimasi tiba, dan tingkat keberhasilan dari JNE, J&T, SiCepat, dan lainnya — beserta alasan rekomendasinya.',
  },
  {
    title: 'Kirim massal tanpa ribet',
    body: 'Unggah pesanan dari Excel atau CSV, validasi per baris, lalu buat ratusan resi dalam sekali proses.',
  },
  {
    title: 'Uang COD terlihat jelas',
    body: 'Setiap rupiah COD tercatat sebagai transaksi, direkonsiliasi per kurir, sampai benar-benar cair ke rekening Anda.',
  },
]
</script>

<style scoped>
.hero {
  text-align: center;
  padding: var(--space-16) var(--space-4) var(--space-12);
}

.hero-eyebrow {
  display: inline-block;
  margin: 0 0 var(--space-4);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-primary);
  background: var(--color-primary-soft);
  border-radius: var(--radius-pill);
}

.hero-title {
  font-size: 48px;
  font-weight: 800;
  letter-spacing: -0.03em;
  margin-bottom: var(--space-4);
}

.hero-sub {
  max-width: 62ch;
  margin: 0 auto var(--space-8);
  font-size: var(--text-md);
  color: var(--color-ink-secondary);
  line-height: var(--leading-relaxed);
}

.hero-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: center;
  flex-wrap: wrap;
}

.btn {
  padding: var(--space-3) var(--space-6);
  border-radius: var(--radius-md);
  font-weight: 600;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
}

.btn-primary:hover {
  background: var(--color-primary-hover);
  color: var(--color-ink-inverse);
}

.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}

.highlights {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-5);
  padding-bottom: var(--space-16);
}

.highlight {
  padding: var(--space-6);
}

.highlight-title {
  font-size: var(--text-md);
  margin-bottom: var(--space-2);
}

.highlight-body {
  margin: 0;
  color: var(--color-ink-secondary);
  line-height: var(--leading-relaxed);
}

@media (max-width: 640px) {
  .hero-title {
    font-size: 34px;
  }
}
</style>