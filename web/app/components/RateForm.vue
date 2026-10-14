<script setup lang="ts">
// RateForm — form parameter pencarian tarif: kota asal/tujuan (cascade
// provinsi→kota), berat, dimensi opsional, COD, dan asuransi.
import type { City, Province } from '~/types/api'

export interface RateFormValue {
  originProvinceId: string
  originCityId: string
  destinationProvinceId: string
  destinationCityId: string
  weight: number
  length: number | null
  width: number | null
  height: number | null
  isCOD: boolean
  codAmount: number
  insured: boolean
  declaredValue: number
}

const props = defineProps<{
  modelValue: RateFormValue
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: RateFormValue]
  submit: []
  reset: []
}>()

const { t } = useT()
const { listProvinces, listCities } = useRegions()

const provinces = ref<Province[]>([])
const originCities = ref<City[]>([])
const destinationCities = ref<City[]>([])

onMounted(async () => {
  try {
    provinces.value = await listProvinces()
  } catch {
    provinces.value = []
  }
  // Muat kota untuk nilai yang sudah terisi (mis. dari query URL).
  await Promise.all([
    loadOriginCities(props.modelValue.originProvinceId, false),
    loadDestinationCities(props.modelValue.destinationProvinceId, false),
  ])
})

function patch(partial: Partial<RateFormValue>) {
  emit('update:modelValue', { ...props.modelValue, ...partial })
}

async function loadOriginCities(provinceId: string, resetCity = true) {
  if (resetCity) patch({ originProvinceId: provinceId, originCityId: '' })
  originCities.value = provinceId ? await safeCities(provinceId) : []
}

async function loadDestinationCities(provinceId: string, resetCity = true) {
  if (resetCity) patch({ destinationProvinceId: provinceId, destinationCityId: '' })
  destinationCities.value = provinceId ? await safeCities(provinceId) : []
}

async function safeCities(provinceId: string): Promise<City[]> {
  try {
    return await listCities(provinceId)
  } catch {
    return []
  }
}

// Dimensi bersifat "semua atau tidak sama sekali" — sama dengan validasi
// backend. Kosongkan salah satu untuk menonaktifkan perhitungan volumetrik.
const dimsFilled = computed(() => {
  const v = props.modelValue
  return v.length !== null && v.width !== null && v.height !== null
})

function onNumber(field: 'weight' | 'length' | 'width' | 'height', raw: string) {
  const n = raw === '' ? null : Number(raw)
  if (field === 'weight') {
    patch({ weight: n ?? 0 })
    return
  }
  patch({ [field]: n } as Partial<RateFormValue>)
}

const canSubmit = computed(() => {
  const v = props.modelValue
  return Boolean(v.originCityId && v.destinationCityId && v.weight > 0)
})
</script>

<template>
  <form
    class="rate-form card"
    @submit.prevent="emit('submit')"
  >
    <div class="grid">
      <fieldset class="group">
        <legend>{{ t('rates.origin') }}</legend>
        <label>
          <span>{{ t('rates.select_province') }}</span>
          <select
            :value="modelValue.originProvinceId"
            @change="loadOriginCities(($event.target as HTMLSelectElement).value)"
          >
            <option value="">
              {{ t('rates.select_province') }}
            </option>
            <option
              v-for="p in provinces"
              :key="p.id"
              :value="p.id"
            >
              {{ p.name }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('rates.origin') }}</span>
          <select
            :value="modelValue.originCityId"
            :disabled="!modelValue.originProvinceId"
            @change="patch({ originCityId: ($event.target as HTMLSelectElement).value })"
          >
            <option value="">
              {{ t('rates.select_city') }}
            </option>
            <option
              v-for="c in originCities"
              :key="c.id"
              :value="c.id"
            >
              {{ c.name }}
            </option>
          </select>
        </label>
      </fieldset>

      <fieldset class="group">
        <legend>{{ t('rates.destination') }}</legend>
        <label>
          <span>{{ t('rates.select_province') }}</span>
          <select
            :value="modelValue.destinationProvinceId"
            @change="loadDestinationCities(($event.target as HTMLSelectElement).value)"
          >
            <option value="">
              {{ t('rates.select_province') }}
            </option>
            <option
              v-for="p in provinces"
              :key="p.id"
              :value="p.id"
            >
              {{ p.name }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('rates.destination') }}</span>
          <select
            :value="modelValue.destinationCityId"
            :disabled="!modelValue.destinationProvinceId"
            @change="patch({ destinationCityId: ($event.target as HTMLSelectElement).value })"
          >
            <option value="">
              {{ t('rates.select_city') }}
            </option>
            <option
              v-for="c in destinationCities"
              :key="c.id"
              :value="c.id"
            >
              {{ c.name }}
            </option>
          </select>
        </label>
      </fieldset>

      <fieldset class="group">
        <legend>{{ t('rates.weight') }}</legend>
        <label>
          <span>{{ t('rates.weight') }}</span>
          <input
            type="number"
            min="0.1"
            step="0.1"
            :value="modelValue.weight"
            @input="onNumber('weight', ($event.target as HTMLInputElement).value)"
          >
        </label>
        <label class="check">
          <input
            type="checkbox"
            :checked="modelValue.isCOD"
            @change="patch({ isCOD: ($event.target as HTMLInputElement).checked })"
          >
          <span>{{ t('rates.is_cod') }}</span>
        </label>
        <label v-if="modelValue.isCOD">
          <span>{{ t('rates.cod_amount') }}</span>
          <input
            type="number"
            min="0"
            step="1000"
            :value="modelValue.codAmount"
            @input="patch({ codAmount: Number(($event.target as HTMLInputElement).value) || 0 })"
          >
        </label>
      </fieldset>

      <fieldset class="group">
        <legend>{{ t('rates.dimensions') }}</legend>
        <div class="dims">
          <label>
            <span>{{ t('rates.length') }}</span>
            <input
              type="number"
              min="0"
              :value="modelValue.length ?? ''"
              @input="onNumber('length', ($event.target as HTMLInputElement).value)"
            >
          </label>
          <label>
            <span>{{ t('rates.width') }}</span>
            <input
              type="number"
              min="0"
              :value="modelValue.width ?? ''"
              @input="onNumber('width', ($event.target as HTMLInputElement).value)"
            >
          </label>
          <label>
            <span>{{ t('rates.height') }}</span>
            <input
              type="number"
              min="0"
              :value="modelValue.height ?? ''"
              @input="onNumber('height', ($event.target as HTMLInputElement).value)"
            >
          </label>
        </div>
        <p
          v-if="!dimsFilled"
          class="hint"
        >
          Isi panjang, lebar, dan tinggi sekaligus untuk menghitung berat volumetrik.
        </p>
        <label class="check">
          <input
            type="checkbox"
            :checked="modelValue.insured"
            @change="patch({ insured: ($event.target as HTMLInputElement).checked })"
          >
          <span>{{ t('rates.insured') }}</span>
        </label>
        <label v-if="modelValue.insured">
          <span>{{ t('rates.declared_value') }}</span>
          <input
            type="number"
            min="0"
            step="1000"
            :value="modelValue.declaredValue"
            @input="patch({ declaredValue: Number(($event.target as HTMLInputElement).value) || 0 })"
          >
        </label>
      </fieldset>
    </div>

    <div class="actions">
      <button
        type="submit"
        class="btn-primary"
        :disabled="!canSubmit || loading"
      >
        {{ loading ? t('rates.loading') : t('rates.submit') }}
      </button>
      <button
        type="button"
        class="btn-secondary"
        :disabled="loading"
        @click="emit('reset')"
      >
        {{ t('rates.reset') }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.rate-form {
  padding: var(--space-4);
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-4);
}

.group {
  border: 0;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.group legend {
  font-weight: 600;
  font-size: var(--text-sm);
  margin-bottom: var(--space-1);
}

.group label {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--text-sm);
  font-weight: 600;
}

.group input,
.group select {
  font: inherit;
  font-weight: 400;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-ink);
}

.dims {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
}

.check {
  flex-direction: row;
  align-items: center;
  gap: var(--space-2);
}

.hint {
  margin: 0;
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.actions {
  display: flex;
  gap: var(--space-3);
  margin-top: var(--space-4);
}

.btn-primary,
.btn-secondary {
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: 0;
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}
</style>