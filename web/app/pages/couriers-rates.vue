<script setup lang="ts">
// Halaman perbandingan tarif: form parameter → daftar opsi + rekomendasi.
// Satu panggilan GET /couriers/rates sudah membawa skor, label, dan
// rekomendasi, sehingga halaman tidak perlu memanggil preview/recommend
// secara terpisah kecuali pengguna meminta pemenang tunggal.
import { ApiRequestError } from '~/composables/useApi'
import type { RateQuote, RateRecommendResponse } from '~/types/api'
import type { RateFormValue } from '~/components/RateForm.vue'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { getRates, getRateRecommend } = useCouriersRates()

useHead({ title: () => t('rates.title') })

const route = useRoute()

function initialForm(): RateFormValue {
  return {
    originProvinceId: String(route.query.origin_province_id ?? ''),
    originCityId: String(route.query.origin_city_id ?? ''),
    destinationProvinceId: String(route.query.destination_province_id ?? ''),
    destinationCityId: String(route.query.destination_city_id ?? ''),
    weight: Number(route.query.weight) || 1,
    length: route.query.length ? Number(route.query.length) : null,
    width: route.query.width ? Number(route.query.width) : null,
    height: route.query.height ? Number(route.query.height) : null,
    isCOD: route.query.is_cod === 'true',
    codAmount: Number(route.query.cod_amount) || 0,
    insured: route.query.insurance === 'true',
    declaredValue: Number(route.query.declared_value) || 0,
  }
}

const form = ref<RateFormValue>(initialForm())
const quote = ref<RateQuote | null>(null)
const recommend = ref<RateRecommendResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

/** Dimensi hanya dikirim bila ketiganya terisi (aturan all-or-nothing). */
const dimensions = computed(() => {
  const f = form.value
  if (f.length === null || f.width === null || f.height === null) return undefined
  return { length: f.length, width: f.width, height: f.height }
})

async function loadRates() {
  error.value = null
  const f = form.value
  if (!f.originCityId || !f.destinationCityId || f.weight <= 0) {
    error.value = t('rates.invalid_params')
    return
  }

  loading.value = true
  quote.value = null
  recommend.value = null
  try {
    quote.value = await getRates({
      originCityId: f.originCityId,
      destinationCityId: f.destinationCityId,
      weight: f.weight,
      dimensions: dimensions.value,
      isCOD: f.isCOD,
      codAmount: f.codAmount,
      insured: f.insured,
      declaredValue: f.declaredValue,
    })
    // Rekomendasi dihitung dari quote yang sama; kegagalan di sini tidak
    // boleh menghapus tabel tarif yang sudah berhasil dimuat.
    if (quote.value.quote_id) {
      recommend.value = await getRateRecommend(quote.value.quote_id)
    }
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.value = {
    originProvinceId: '',
    originCityId: '',
    destinationProvinceId: '',
    destinationCityId: '',
    weight: 1,
    length: null,
    width: null,
    height: null,
    isCOD: false,
    codAmount: 0,
    insured: false,
    declaredValue: 0,
  }
  quote.value = null
  recommend.value = null
  error.value = null
}
</script>

<template>
  <div class="couriers-rates">
    <header class="page-head">
      <h1 class="page-title">
        {{ t('rates.title') }}
      </h1>
      <p class="page-subtitle">
        {{ t('rates.subtitle') }}
      </p>
    </header>

    <RateForm
      v-model="form"
      :loading="loading"
      @submit="loadRates"
      @reset="resetForm"
    />

    <div
      v-if="error"
      class="form-alert"
      role="alert"
    >
      {{ error }}
    </div>

    <RatePreview
      :quote="quote"
      :loading="loading"
      class="results"
    />

    <RateRecommendation :recommend="recommend" />
  </div>
</template>

<style scoped>
.couriers-rates {
  padding: var(--space-6);
}

.page-head {
  margin-bottom: var(--space-5);
}

.page-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.page-subtitle {
  margin: 0;
  color: var(--color-ink-secondary);
}

.form-alert {
  margin-top: var(--space-4);
  padding: var(--space-3);
  border-radius: var(--radius-md);
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.results {
  margin-top: var(--space-5);
}
</style>