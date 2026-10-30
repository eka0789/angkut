<script setup lang="ts">
import AppIcon from '~/components/AppIcon.vue'
// Kalkulator publik (TASK-064): estimasi biaya berbasis tarif mock yang sama
// dengan engine (berat chargeable = max(aktual, volumetrik), tarif dasar per
// berat dari tier kurir mock). Estimasi — harga final lewat /couriers/rates.
import { computed, reactive } from 'vue'

definePageMeta({ layout: 'public' })
useHead({ title: () => 'Kalkulator Ongkir — Angkut' })

const form = reactive({
  weight: 1.0,
  length: 25,
  width: 20,
  height: 10,
  isCod: false,
  codAmount: 0,
})

const services = [
  { code: 'jne-REG', name: 'JNE Reguler', eta: '2-3 hari', divisor: 6000, base: 9000, perKg: 4000, codRate: 0.01, codMin: 2500 },
  { code: 'jnt-EZ', name: 'J&T EZ', eta: '2-4 hari', divisor: 6000, base: 8500, perKg: 3800, codRate: 0.01, codMin: 2500 },
  { code: 'scp-BEST', name: 'SiCepat BEST', eta: '2-3 hari', divisor: 6000, base: 8000, perKg: 3600, codRate: 0.01, codMin: 2500 },
  { code: 'jne-YES', name: 'JNE YES (next day)', eta: '1 hari', divisor: 6000, base: 15000, perKg: 6000, codRate: 0.01, codMin: 2500 },
]

const volumetric = computed(() => (form.length * form.width * form.height) / 6000)
const chargeable = computed(() => Math.max(form.weight, volumetric.value))
const roundedKg = computed(() => Math.ceil(chargeable.value))

 

const results = computed(() =>
  services.map(s => {
    const shipping = s.base + Math.max(0, roundedKg.value - 1) * s.perKg
    const codFee = form.isCod ? Math.max(2500, Math.round(form.codAmount * s.codRate)) : 0
    return { ...s, shipping, codFee, total: shipping + codFee }
  }))

const fmtIDR = (v: number): string => `Rp ${v.toLocaleString('id-ID')}`
</script>

<template>
  <div class="calc">
    <header class="hero">
      <span class="hero-badge"><AppIcon :name='"calculator"' :size="14" /> Kalkulator Ongkir</span>
      <h1>Kalkulator Ongkir</h1>
      <p>Estimasi cepat biaya pengiriman. Harga final dihitung otomatis saat Anda membuat pengiriman.</p>
    </header>

    <div class="layout">
      <section class="card form">
        <label class="field">
          <span>Berat aktual (kg)</span>
          <input
            v-model.number="form.weight"
            type="number"
            min="0.1"
            step="0.1"
          >
        </label>
        <div class="dims">
          <label class="field">
            <span>Panjang (cm)</span>
            <input
              v-model.number="form.length"
              type="number"
              min="1"
            >
          </label>
          <label class="field">
            <span>Lebar (cm)</span>
            <input
              v-model.number="form.width"
              type="number"
              min="1"
            >
          </label>
          <label class="field">
            <span>Tinggi (cm)</span>
            <input
              v-model.number="form.height"
              type="number"
              min="1"
            >
          </label>
        </div>
        <label class="check">
          <input
            v-model="form.isCod"
            type="checkbox"
          >
          Pakai COD
        </label>
        <label
          v-if="form.isCod"
          class="field"
        >
          <span>Nilai COD (Rp)</span>
          <input
            v-model.number="form.codAmount"
            type="number"
            min="0"
            step="1000"
          >
        </label>
        <p class="note">
          Berat chargeable: <strong>{{ chargeable.toFixed(2) }} kg</strong>
          (maksimum berat aktual {{ form.weight }} kg dan volumetrik {{ volumetric.toFixed(2) }} kg, dibulatkan ke atas = {{ roundedKg }} kg).
        </p>
      </section>

      <section class="results">
        <article
          v-for="r in results"
          :key="r.code"
          class="card result"
        >
          <div class="result-head">
            <h2>{{ r.name }}</h2>
            <span class="eta">{{ r.eta }}</span>
          </div>
          <div class="rows">
            <span>Ongkos kirim</span><span>{{ fmtIDR(r.shipping) }}</span>
            <span
              v-if="form.isCod"
            >Fee COD</span><span
              v-if="form.isCod"
            >{{ fmtIDR(r.codFee) }}</span>
            <span class="total">Total</span><span class="total">{{ fmtIDR(r.total) }}</span>
          </div>
        </article>
      </section>
    </div>
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
.hero { text-align: center; padding: var(--space-10) var(--space-4) var(--space-6); }
.hero h1 { font-size: 32px; margin: 0 0 var(--space-3); }
.hero p { color: var(--color-ink-secondary); margin: 0; max-width: 60ch; margin-inline: auto; }

.layout {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: var(--space-5);
  max-width: 960px;
  margin: 0 auto var(--space-10);
  padding: 0 var(--space-4);
  align-items: start;
}
@media (max-width: 800px) { .layout { grid-template-columns: 1fr; } }

.card { padding: var(--space-5); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-surface); }

.field { display: flex; flex-direction: column; gap: var(--space-1); margin-bottom: var(--space-3); font-size: var(--text-sm); }
.field input {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}
.dims { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--space-2); }
.check { display: flex; gap: var(--space-2); align-items: center; font-size: var(--text-sm); margin-bottom: var(--space-3); }
.note { font-size: var(--text-xs); color: var(--color-ink-muted); margin: 0; }

.results { display: flex; flex-direction: column; gap: var(--space-3); }
.result-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--space-2); }
.result-head h2 { font-size: var(--text-md); margin: 0; }
.eta { font-size: var(--text-xs); color: var(--color-ink-muted); }
.rows {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: var(--space-1) var(--space-4);
  font-size: var(--text-sm);
  color: var(--color-ink-secondary);
}
.rows .total { font-weight: 700; color: var(--color-ink); font-size: var(--text-md); }
</style>
