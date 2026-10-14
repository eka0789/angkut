<script setup lang="ts">
// ShipmentWizard — pembuatan pengiriman dalam 3 langkah: paket → rute & kurir
// → tinjau. Idempotency-Key dibuat SEKALI saat wizard dibuka dan dipakai ulang
// pada setiap percobaan submit, sehingga mengklik "Buat" dua kali (atau retry
// setelah jaringan gagal) tidak menghasilkan pengiriman ganda.
import { ApiRequestError } from '~/composables/useApi'
import { newIdempotencyKey } from '~/composables/useShipments'
import type { Address, Shipment, ShipmentCreateInput } from '~/types/api'

const emit = defineEmits<{ created: [shipment: Shipment] }>()

const { t } = useT()
const { create } = useShipments()
const { list: listAddresses } = useAddresses()
const { list: listCouriers, getServices } = useCourierServices()

const step = ref(0)
const submitting = ref(false)
const error = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})

/** Key idempotency stabil untuk seluruh masa hidup wizard ini. */
const idempotencyKeyId = newIdempotencyKey()

const addresses = ref<Address[]>([])
const couriers = ref<Array<{ code: string, name: string }>>([])
const services = ref<Array<{ code: string, name: string, supports_cod: boolean }>>([])
const loadingServices = ref(false)

const form = reactive({
  originAddressId: '',
  destinationAddressId: '',
  courierCode: '',
  serviceCode: '',
  weight: 1,
  length: null as number | null,
  width: null as number | null,
  height: null as number | null,
  packageType: 'box',
  isCOD: false,
  codAmount: 0,
  declaredValue: 0,
  insurance: false,
  notes: '',
})

const STEPS = ['wizard.step_package', 'wizard.step_route', 'wizard.step_review'] as const

onMounted(async () => {
  try {
    const [addrs, crs] = await Promise.all([listAddresses(), listCouriers()])
    addresses.value = addrs
    couriers.value = crs.map(c => ({ code: c.code, name: c.name }))
  } catch {
    error.value = t('common.error')
  }
})

watch(() => form.courierCode, async (code) => {
  form.serviceCode = ''
  services.value = []
  if (!code) return
  loadingServices.value = true
  try {
    services.value = await getServices(code)
  } catch {
    services.value = []
  } finally {
    loadingServices.value = false
  }
})

/** Dimensi hanya dikirim bila ketiganya terisi (aturan all-or-nothing server). */
const dimensions = computed(() => {
  if (form.length === null || form.width === null || form.height === null) return {}
  return { length: form.length, width: form.width, height: form.height }
})

const stepValid = computed(() => {
  if (step.value === 0) return form.weight > 0
  if (step.value === 1) {
    return Boolean(form.originAddressId && form.destinationAddressId && form.courierCode && form.serviceCode)
  }
  return true
})

function next() {
  if (!stepValid.value) {
    error.value = t('wizard.invalid_step')
    return
  }
  error.value = null
  if (step.value < STEPS.length - 1) step.value += 1
}

function prev() {
  error.value = null
  if (step.value > 0) step.value -= 1
}

function buildPayload(): ShipmentCreateInput {
  return {
    courier_code: form.courierCode,
    service_code: form.serviceCode,
    origin_address_id: form.originAddressId,
    destination_address_id: form.destinationAddressId,
    weight: form.weight,
    ...dimensions.value,
    package_type: form.packageType,
    is_cod: form.isCOD,
    cod_amount: form.isCOD ? form.codAmount : 0,
    declared_value: form.declaredValue,
    insurance: form.insurance,
    notes: form.notes,
  }
}

async function submit() {
  if (!stepValid.value) {
    error.value = t('wizard.invalid_step')
    return
  }
  submitting.value = true
  error.value = null
  fieldErrors.value = {}
  try {
    // Key sama untuk setiap percobaan → retry tidak menduplikasi.
    const sh = await create(buildPayload(), idempotencyKeyId)
    emit('created', sh)
  } catch (err: unknown) {
    if (err instanceof ApiRequestError) {
      fieldErrors.value = err.fields ?? {}
      error.value = err.message
    } else {
      error.value = t('common.error')
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form
    class="wizard card"
    @submit.prevent="submit"
  >
    <ol class="steps">
      <li
        v-for="(s, i) in STEPS"
        :key="s"
        :class="{ active: i === step, done: i < step }"
      >
        <span class="num">{{ i + 1 }}</span>
        <span>{{ t(s) }}</span>
      </li>
    </ol>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>

    <p
      v-if="addresses.length === 0"
      class="alert info"
    >
      {{ t('wizard.need_addresses') }}
    </p>

    <!-- Langkah 1: paket -->
    <fieldset
      v-if="step === 0"
      class="step-body"
    >
      <p class="hint">
        {{ t('wizard.package_hint') }}
      </p>
      <div class="fields">
        <label>
          <span>{{ t('wizard.field_weight') }}</span>
          <input
            v-model.number="form.weight"
            type="number"
            min="0.1"
            step="0.1"
          >
          <small
            v-if="fieldErrors.weight"
            class="err"
          >{{ fieldErrors.weight }}</small>
        </label>
        <label>
          <span>{{ t('rates.length') }}</span>
          <input
            :value="form.length ?? ''"
            type="number"
            min="0"
            @input="form.length = ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value)"
          >
        </label>
        <label>
          <span>{{ t('rates.width') }}</span>
          <input
            :value="form.width ?? ''"
            type="number"
            min="0"
            @input="form.width = ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value)"
          >
        </label>
        <label>
          <span>{{ t('rates.height') }}</span>
          <input
            :value="form.height ?? ''"
            type="number"
            min="0"
            @input="form.height = ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value)"
          >
        </label>
        <label>
          <span>{{ t('wizard.field_package_type') }}</span>
          <input
            v-model="form.packageType"
            type="text"
          >
        </label>
      </div>
    </fieldset>

    <!-- Langkah 2: rute & kurir -->
    <fieldset
      v-else-if="step === 1"
      class="step-body"
    >
      <p class="hint">
        {{ t('wizard.route_hint') }}
      </p>
      <div class="fields">
        <label>
          <span>{{ t('wizard.field_origin') }}</span>
          <select v-model="form.originAddressId">
            <option value="">
              {{ t('wizard.select_placeholder') }}
            </option>
            <option
              v-for="a in addresses"
              :key="a.id"
              :value="a.id"
            >
              {{ a.recipient }} — {{ a.street }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('wizard.field_destination') }}</span>
          <select v-model="form.destinationAddressId">
            <option value="">
              {{ t('wizard.select_placeholder') }}
            </option>
            <option
              v-for="a in addresses"
              :key="a.id"
              :value="a.id"
            >
              {{ a.recipient }} — {{ a.street }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('wizard.field_courier') }}</span>
          <select v-model="form.courierCode">
            <option value="">
              {{ t('wizard.select_placeholder') }}
            </option>
            <option
              v-for="c in couriers"
              :key="c.code"
              :value="c.code"
            >
              {{ c.name }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('wizard.field_service') }}</span>
          <select
            v-model="form.serviceCode"
            :disabled="!form.courierCode || loadingServices"
          >
            <option value="">
              {{ t('wizard.select_placeholder') }}
            </option>
            <option
              v-for="s in services"
              :key="s.code"
              :value="s.code"
            >
              {{ s.name }} ({{ s.code }})
            </option>
          </select>
        </label>
      </div>

      <div class="fields">
        <label class="check">
          <input
            v-model="form.isCOD"
            type="checkbox"
          >
          <span>{{ t('wizard.field_is_cod') }}</span>
        </label>
        <label v-if="form.isCOD">
          <span>{{ t('wizard.field_cod_amount') }}</span>
          <input
            v-model.number="form.codAmount"
            type="number"
            min="0"
            step="1000"
          >
          <small
            v-if="fieldErrors.cod_amount"
            class="err"
          >{{ fieldErrors.cod_amount }}</small>
        </label>
        <label class="check">
          <input
            v-model="form.insurance"
            type="checkbox"
          >
          <span>{{ t('wizard.field_insurance') }}</span>
        </label>
        <label>
          <span>{{ t('wizard.field_declared_value') }}</span>
          <input
            v-model.number="form.declaredValue"
            type="number"
            min="0"
            step="1000"
          >
        </label>
        <label class="wide">
          <span>{{ t('wizard.field_notes') }}</span>
          <input
            v-model="form.notes"
            type="text"
          >
        </label>
      </div>
    </fieldset>

    <!-- Langkah 3: tinjau -->
    <fieldset
      v-else
      class="step-body"
    >
      <p class="hint">
        {{ t('wizard.review_hint') }}
      </p>
      <dl class="review">
        <div>
          <dt>{{ t('wizard.field_weight') }}</dt>
          <dd>{{ form.weight }} kg</dd>
        </div>
        <div>
          <dt>{{ t('wizard.field_origin') }}</dt>
          <dd>{{ addresses.find(a => a.id === form.originAddressId)?.recipient ?? '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('wizard.field_destination') }}</dt>
          <dd>{{ addresses.find(a => a.id === form.destinationAddressId)?.recipient ?? '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('wizard.field_courier') }}</dt>
          <dd>{{ form.courierCode.toUpperCase() }} / {{ form.serviceCode }}</dd>
        </div>
        <div>
          <dt>{{ t('wizard.field_is_cod') }}</dt>
          <dd>{{ form.isCOD ? `${form.codAmount.toLocaleString('id-ID')}` : '—' }}</dd>
        </div>
        <div v-if="form.insurance">
          <dt>{{ t('wizard.field_declared_value') }}</dt>
          <dd>{{ form.declaredValue.toLocaleString('id-ID') }}</dd>
        </div>
      </dl>
    </fieldset>

    <div class="actions">
      <button
        v-if="step > 0"
        type="button"
        class="btn-secondary"
        :disabled="submitting"
        @click="prev"
      >
        {{ t('wizard.prev') }}
      </button>
      <button
        v-if="step < STEPS.length - 1"
        type="button"
        class="btn-primary"
        @click="next"
      >
        {{ t('wizard.next') }}
      </button>
      <button
        v-else
        type="submit"
        class="btn-primary"
        :disabled="submitting || !stepValid"
      >
        {{ submitting ? t('wizard.creating') : t('wizard.submit') }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.wizard {
  padding: var(--space-5);
}

.steps {
  display: flex;
  gap: var(--space-3);
  list-style: none;
  padding: 0;
  margin: 0 0 var(--space-5);
  flex-wrap: wrap;
}

.steps li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--color-ink-muted);
}

.steps li.active {
  color: var(--color-ink);
  font-weight: 600;
}

.steps li.done {
  color: var(--color-primary);
}

.steps .num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--color-border);
  font-size: var(--text-xs);
}

.steps li.active .num {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border-color: var(--color-primary);
}

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-4);
}

.alert.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.alert.info {
  border: 1px solid #bfdbfe;
  background: #eff6ff;
  color: #1e40af;
}

.step-body {
  border: 0;
  margin: 0;
  padding: 0;
}

.hint {
  margin: 0 0 var(--space-3);
  color: var(--color-ink-secondary);
  font-size: var(--text-sm);
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}

.fields label {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--text-sm);
  font-weight: 600;
}

.fields label.wide {
  grid-column: 1 / -1;
}

.fields .check {
  flex-direction: row;
  align-items: center;
}

.fields input,
.fields select {
  font: inherit;
  font-weight: 400;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-ink);
}

.err {
  color: #9f1239;
  font-weight: 400;
}

.review {
  display: grid;
  gap: var(--space-2);
  margin: 0;
}

.review > div {
  display: flex;
  justify-content: space-between;
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--space-2);
}

.review dt {
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}

.review dd {
  margin: 0;
  font-weight: 500;
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