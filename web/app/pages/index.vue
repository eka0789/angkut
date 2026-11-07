<script setup lang="ts">
// Landing publik: hero aurora, marquee kurir, grid fitur, langkah, CTA.
// Semua klaim mencerminkan fitur yang benar-benar ada di platform.
import AppIcon from '~/components/AppIcon.vue'

definePageMeta({ layout: 'public' })

const { t } = useT()

useHead({ title: 'Ship smarter. Grow faster.' })

const kurir = ['JNE', 'J&T Express', 'SiCepat']

const features = [
  { icon: 'activity', title: 'Pelacakan realtime', body: 'Timeline kejadian per resi dengan pembaruan tanpa refresh. Halaman lacak publik bisa dibagikan langsung ke pembeli.' },
  { icon: 'layers', title: 'Kirim massal CSV', body: 'Unggah ratusan baris, validasi per baris, lalu terbitkan semua resi sekaligus dan unduh label dalam satu ZIP.' },
  { icon: 'banknote', title: 'COD & buku besar', body: 'Setiap rupiah COD tercatat, direkonsiliasi per kurir, dan terjaga ledger append-only yang tidak bisa diubah.' },
  { icon: 'package', title: 'Inventory multi-gudang', body: 'Saldo per gudang (on-hand, reserved, available), riwayat movement, dan transfer antar gudang yang atomik.' },
  { icon: 'sparkles', title: 'Asisten AI', body: 'Tanya biaya, status, atau COD — dijawab dari data organisasi Anda sendiri, dengan konfirmasi untuk aksi sensitif.' },
  { icon: 'bar-chart', title: 'Dasbor & skor kesehatan', body: 'KPI harian, tren biaya 30 hari, performa kurir, dan skor kesehatan logistik dengan rekomendasi konkret.' },
]

const steps = [
  { title: 'Hubungkan operasi Anda', body: 'Daftar, buat gudang dan toko, atur alamat asal — semuanya dari satu dasbor.' },
  { title: 'Buat & bandingkan pengiriman', body: 'Pilih kurir terbaik dari perbandingan tarif dan estimasi, lalu terbitkan resi dengan label siap cetak.' },
  { title: 'Pantau sampai cair', body: 'Lacak setiap paket realtime, rekonsiliasi COD per kurir, dan lihat semuanya di buku besar.' },
]
</script>

<template>
  <div class="landing">
    <section class="hero aurora">
      <span
        class="hero-orb o1"
        aria-hidden="true"
      />
      <span
        class="hero-orb o2"
        aria-hidden="true"
      />
      <span
        class="hero-orb o3"
        aria-hidden="true"
      />

      <p class="hero-eyebrow">
        <AppIcon
          name="zap"
          :size="14"
        />
        Platform logistik cerdas untuk bisnis Indonesia
      </p>
      <h1 class="hero-title">
        <span class="text-gradient">Ship smarter.</span><br>
        Grow faster.
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
          <AppIcon
            name="arrow-right"
            :size="16"
          />
        </NuxtLink>
        <NuxtLink
          to="/track"
          class="btn btn-ghost"
        >
          <AppIcon
            name="map-pin"
            :size="16"
          />
          {{ t('nav.trackPackage') }}
        </NuxtLink>
      </div>
      <p class="hero-chips">
        <span>Gratis untuk mulai</span>
        <span>Tanpa kartu kredit</span>
        <span>Label PDF A4 &amp; thermal</span>
      </p>
    </section>

    <section
      class="marquee marquee-mask"
      aria-label="Kurir yang terhubung"
    >
      <div class="marquee-inner">
        <span class="marquee-label">Terhubung dengan kurir terpercaya</span>
        <div class="marquee-track">
          <template
            v-for="n in 2"
            :key="n"
          >
            <span
              v-for="k in kurir"
              :key="`${n}-${k}`"
              class="marquee-item"
            >
              {{ k }}
            </span>
          </template>
        </div>
      </div>
    </section>

    <section class="features">
      <h2 class="section-title">
        Semua yang Anda butuhkan untuk <span class="text-gradient">mengelola pengiriman</span>
      </h2>
      <p class="section-sub">
        Dari buat order sampai paket diterima — satu platform, satu sumber kebenaran.
      </p>
      <div class="feature-grid">
        <article
          v-for="f in features"
          :key="f.title"
          class="card feature-card"
        >
          <span class="feature-icon">
            <AppIcon
              :name="f.icon"
              :size="22"
            />
          </span>
          <h3>{{ f.title }}</h3>
          <p>{{ f.body }}</p>
        </article>
      </div>
    </section>

    <section class="steps-wrap">
      <h2 class="section-title">
        Mulai dalam <span class="text-gradient">tiga langkah</span>
      </h2>
      <div class="steps">
        <article
          v-for="(s, i) in steps"
          :key="s.title"
          class="card step"
        >
          <span class="step-num">{{ i + 1 }}</span>
          <h3>{{ s.title }}</h3>
          <p>{{ s.body }}</p>
        </article>
      </div>
    </section>

    <section class="cta-band-wrap">
      <div class="cta-band">
        <h2>Siap mengelola logistik dengan lebih cerdas?</h2>
        <p>Buat akun gratis dan terbitkan pengiriman pertama Anda hari ini.</p>
        <div class="cta-actions">
          <NuxtLink
            to="/register"
            class="btn btn-cta"
          >
            {{ t('auth.register') }}
          </NuxtLink>
          <NuxtLink
            to="/pricing"
            class="btn btn-outline-light"
          >
            Lihat harga
          </NuxtLink>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.landing {
  --pad-x: clamp(var(--space-4), 5vw, var(--space-10));
}

/* ── Hero ─────────────────────────────────────────────────────────────── */
.hero {
  text-align: center;
  padding: var(--space-16) var(--pad-x) var(--space-12);
  margin: calc(-1 * var(--space-10)) calc(-1 * var(--pad-x)) 0;
  border-radius: 0 0 var(--radius-xl) var(--radius-xl);
}

.hero-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0 0 var(--space-5);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-xs);
  font-weight: 700;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  border: 1px solid color-mix(in srgb, var(--color-primary) 22%, transparent);
  border-radius: var(--radius-pill);
}

.hero-title {
  font-size: clamp(38px, 6vw, 60px);
  font-weight: 800;
  letter-spacing: -0.03em;
  line-height: 1.06;
  margin-bottom: var(--space-4);
}

.hero-sub {
  max-width: 58ch;
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
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-6);
  border-radius: var(--radius-md);
  font-weight: 700;
  font-size: var(--text-sm);
}

.btn-ghost {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
  box-shadow: var(--shadow-xs);
}
.btn-ghost:hover { border-color: var(--color-primary); color: var(--color-primary); transform: translateY(-1px); }

.hero-chips {
  display: flex;
  gap: var(--space-3);
  justify-content: center;
  flex-wrap: wrap;
  margin: var(--space-6) 0 0;
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}
.hero-chips span {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 3px var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-pill);
}

/* ── Marquee kurir ────────────────────────────────────────────────────── */
.marquee { padding: var(--space-8) 0; }
.marquee-inner {
  max-width: var(--content-max);
  margin: 0 auto;
  padding: 0 var(--pad-x);
}
.marquee-label {
  display: block;
  text-align: center;
  font-size: var(--text-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-ink-muted);
  margin-bottom: var(--space-4);
}
.marquee-item {
  font-size: var(--text-xl);
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--color-ink-muted);
  opacity: 0.75;
  transition: opacity 200ms ease, color 200ms ease;
}
.marquee-item:hover { opacity: 1; color: var(--color-primary); }

/* ── Bagian umum ──────────────────────────────────────────────────────── */
section:not(.hero):not(.marquee) { padding: var(--space-10) var(--pad-x); }

.section-title {
  text-align: center;
  font-size: clamp(24px, 3.4vw, 34px);
  font-weight: 800;
  letter-spacing: -0.02em;
  margin-bottom: var(--space-2);
}
.section-sub {
  text-align: center;
  color: var(--color-ink-secondary);
  margin: 0 auto var(--space-8);
  max-width: 52ch;
}

/* ── Grid fitur ───────────────────────────────────────────────────────── */
.feature-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--space-4);
  max-width: 1100px;
  margin: 0 auto;
}
.feature-card { padding: var(--space-6) var(--space-5); }
.feature-card h3 { font-size: var(--text-md); margin: var(--space-4) 0 var(--space-2); }
.feature-card p { margin: 0; color: var(--color-ink-secondary); font-size: var(--text-sm); line-height: var(--leading-relaxed); }

/* ── Langkah ──────────────────────────────────────────────────────────── */
.steps {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-4);
  max-width: 1000px;
  margin: 0 auto;
  counter-reset: langkah;
}
.step { padding: var(--space-6) var(--space-5); }
.step h3 { font-size: var(--text-md); margin: var(--space-4) 0 var(--space-2); }
.step p { margin: 0; color: var(--color-ink-secondary); font-size: var(--text-sm); line-height: var(--leading-relaxed); }

/* ── CTA band ─────────────────────────────────────────────────────────── */
.cta-band-wrap { padding-bottom: var(--space-16); }
.cta-band {
  max-width: 960px;
  margin: 0 auto;
  padding: var(--space-12) var(--space-6);
  text-align: center;
}
.cta-band h2 {
  font-size: clamp(24px, 3vw, 32px);
  font-weight: 800;
  letter-spacing: -0.02em;
  margin-bottom: var(--space-2);
}
.cta-band p {
  color: rgba(255, 255, 255, 0.85);
  margin: 0 0 var(--space-6);
}
.cta-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: center;
  flex-wrap: wrap;
  position: relative;
  z-index: 1;
}
.btn-cta { border: none; }
.btn-outline-light {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
  border: 1px solid rgba(255, 255, 255, 0.35);
  backdrop-filter: blur(4px);
}
.btn-outline-light:hover { background: rgba(255, 255, 255, 0.2); color: #fff; }
</style>
