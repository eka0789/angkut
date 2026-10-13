<script setup lang="ts">
// Profil pelanggan: identitas, kartu metrik, override segmen, dan (nanti)
// riwayat order. Metrik bersumber dari jsonb metrics — diisi modul Order.
import { ApiRequestError } from '~/composables/useApi'
import type { Customer, CustomerSegment, CustomerStats } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const route = useRoute()
const { t } = useT()
const { get, update, stats, setSegment } = useCustomers()
const { session } = useAuth()

const custId = computed(() => route.params.id as string)

const customer = ref<Customer | null>(null)
const stat = ref<CustomerStats | null>(null)
const loading = ref(true)
const loadError = ref<string | null>(null)
const saving = ref(false)
const errorMessage = ref<string | null>(null)
const saveMessage = ref<string | null>(null)

// Form identitas.
const form = ref({ name: '', phone: '', email: '', notes: '' })

// Override segmen.
const segmentChoice = ref<CustomerSegment | ''>('')
const canManage = computed(() => {
  const role = session.value?.active_role
  return role === 'owner' || role === 'admin' || role === 'staff'
})

const segments: Array<{ value: CustomerSegment, label: string }> = [
  { value: 'new', label: t('customer.segment_new') },
  { value: 'regular', label: t('customer.segment_regular') },
  { value: 'vip', label: t('customer.segment_vip') },
  { value: 'high_risk', label: t('customer.segment_high_risk') },
  { value: 'cod_risk', label: t('customer.segment_cod_risk') },
]

function segmentLabel(s: CustomerSegment): string {
  const found = segments.find(x => x.value === s)
  return found ? found.label : s
}

function formatSpend(v: number): string {
  return `Rp ${v.toLocaleString('id-ID')}`
}

function pct(v: number): string {
  return `${(v * 100).toFixed(1)}%`
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    const [c, s] = await Promise.all([
      get(custId.value),
      stats(custId.value).catch(() => null),
    ])
    customer.value = c
    stat.value = s
    form.value = {
      name: c.name,
      phone: c.phone,
      email: c.email ?? '',
      notes: c.notes ?? '',
    }
  } catch (err: unknown) {
    loadError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!customer.value) return
  saving.value = true
  errorMessage.value = null
  saveMessage.value = null
  try {
    const updated = await update(customer.value.id, form.value)
    customer.value = updated
    saveMessage.value = t('customer.saved')
  } catch (err: unknown) {
    if (err instanceof ApiRequestError) {
      errorMessage.value = err.fields ? Object.values(err.fields).join(' ') : err.message
    } else {
      errorMessage.value = t('common.error')
    }
  } finally {
    saving.value = false
  }
}

async function applySegment() {
  if (!customer.value) return
  saving.value = true
  errorMessage.value = null
  saveMessage.value = null
  try {
    const updated = await setSegment(customer.value.id, segmentChoice.value)
    customer.value = updated
    saveMessage.value = t('customer.segment_saved')
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    saving.value = false
  }
}

async function clearOverride() {
  if (!customer.value) return
  saving.value = true
  errorMessage.value = null
  saveMessage.value = null
  try {
    const updated = await setSegment(customer.value.id, '')
    customer.value = updated
    saveMessage.value = t('customer.segment_saved')
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="cust-detail">
    <header class="page-head">
      <NuxtLink
        to="/customers"
        class="back"
      >
        ← {{ t('common.back') }}
      </NuxtLink>
      <h1 class="page-title">
        {{ customer ? customer.name : t('customer.profile_title') }}
      </h1>
      <p
        v-if="customer"
        class="page-subtitle"
      >
        <span class="mono">{{ customer.phone }}</span>
        <span
          v-if="customer.email"
        > · {{ customer.email }}</span>
        <span
          class="seg-badge"
          :class="`seg-${customer.segment}`"
        >
          {{ segmentLabel(customer.segment) }}
          <span
            v-if="customer.segment_source === 'manual'"
            class="seg-src"
          >{{ t('customer.manual') }}</span>
        </span>
      </p>
    </header>

    <div
      v-if="loadError"
      class="form-alert"
      role="alert"
    >
      {{ loadError }}
    </div>

    <div
      v-else-if="loading"
      class="state-block"
      role="status"
    >
      {{ t('common.loading') }}
    </div>

    <template v-else-if="customer">
      <div
        v-if="saveMessage"
        class="state-ok"
        role="status"
      >
        {{ saveMessage }}
      </div>
      <div
        v-if="errorMessage"
        class="form-alert"
        role="alert"
      >
        {{ errorMessage }}
      </div>

      <!-- Kartu metrik -->
      <section class="metric-grid">
        <div class="card metric">
          <p class="metric-label">
            {{ t('customer.metric_orders') }}
          </p>
          <p class="metric-value">
            {{ stat?.orders_count ?? customer.metrics.orders_count }}
          </p>
        </div>
        <div class="card metric">
          <p class="metric-label">
            {{ t('customer.metric_spend') }}
          </p>
          <p class="metric-value">
            {{ formatSpend(stat?.total_spend ?? customer.metrics.total_spend) }}
          </p>
        </div>
        <div class="card metric">
          <p class="metric-label">
            {{ t('customer.metric_aov') }}
          </p>
          <p class="metric-value">
            {{ formatSpend(stat?.aov ?? customer.metrics.aov) }}
          </p>
        </div>
        <div class="card metric">
          <p class="metric-label">
            {{ t('customer.metric_return_rate') }}
          </p>
          <p class="metric-value">
            {{ pct(stat?.return_rate ?? customer.metrics.return_rate) }}
          </p>
        </div>
        <div class="card metric">
          <p class="metric-label">
            {{ t('customer.metric_cod_rate') }}
          </p>
          <p class="metric-value">
            {{ (stat?.cod_success_rate ?? customer.metrics.cod_success_rate) != null
              ? pct((stat?.cod_success_rate ?? customer.metrics.cod_success_rate) as number)
              : '—' }}
          </p>
        </div>
        <div class="card metric">
          <p class="metric-label">
            {{ t('customer.metric_courier') }}
          </p>
          <p class="metric-value">
            {{ (stat?.preferred_courier ?? customer.metrics.preferred_courier) || '—' }}
          </p>
        </div>
      </section>

      <!-- Segmen -->
      <section
        v-if="canManage"
        class="card section"
      >
        <h2 class="section-title">
          {{ t('customer.segment') }}
        </h2>
        <p class="hint">
          {{ t('customer.segment_override_hint') }}
        </p>
        <div class="segment-row">
          <select
            v-model="segmentChoice"
            class="segment-select"
            :aria-label="t('customer.segment_set')"
          >
            <option
              value=""
              disabled
            >
              {{ t('customer.segment_set') }}
            </option>
            <option
              v-for="s in segments"
              :key="s.value"
              :value="s.value"
            >
              {{ s.label }}
            </option>
          </select>
          <button
            type="button"
            class="btn-primary"
            :disabled="saving || !segmentChoice"
            @click="applySegment"
          >
            {{ t('customer.segment_set') }}
          </button>
          <button
            v-if="customer.segment_source === 'manual'"
            type="button"
            class="btn-secondary"
            :disabled="saving"
            @click="clearOverride"
          >
            {{ t('customer.segment_clear') }}
          </button>
        </div>
      </section>

      <!-- Identitas -->
      <section class="card section">
        <h2 class="section-title">
          {{ t('customer.edit') }}
        </h2>
        <form
          class="form-grid"
          novalidate
          @submit.prevent="save"
        >
          <div class="field">
            <label for="cd-name">{{ t('customer.field_name') }}</label>
            <input
              id="cd-name"
              v-model="form.name"
              type="text"
              required
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="cd-phone">{{ t('customer.field_phone') }}</label>
            <input
              id="cd-phone"
              v-model="form.phone"
              type="tel"
              required
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="cd-email">{{ t('customer.field_email') }}</label>
            <input
              id="cd-email"
              v-model="form.email"
              type="email"
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="cd-notes">{{ t('customer.field_notes') }}</label>
            <input
              id="cd-notes"
              v-model="form.notes"
              type="text"
              :disabled="!canManage"
            >
          </div>
          <div
            v-if="canManage"
            class="form-actions"
          >
            <button
              type="submit"
              class="btn-primary"
              :disabled="saving"
            >
              {{ saving ? t('common.loading') : t('common.save') }}
            </button>
          </div>
        </form>
      </section>

      <!-- Riwayat order (placeholder sampai TASK-026) -->
      <section class="card section">
        <h2 class="section-title">
          {{ t('nav.orders') }}
        </h2>
        <p class="hint">
          {{ t('customer.orders_empty') }}
        </p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: var(--space-5);
}

.back {
  display: inline-block;
  margin-bottom: var(--space-2);
  font-size: var(--text-sm);
}

.page-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.page-subtitle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  margin: 0;
  color: var(--color-ink-secondary);
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-5);
}

.metric {
  padding: var(--space-4);
}

.metric-label {
  margin: 0 0 var(--space-1);
  font-size: var(--text-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-ink-muted);
}

.metric-value {
  margin: 0;
  font-size: var(--text-lg);
  font-weight: 700;
}

.section {
  padding: var(--space-5);
  margin-bottom: var(--space-4);
}

.section-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-3);
}

.segment-row {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  flex-wrap: wrap;
  margin-top: var(--space-3);
}

.segment-select {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  background: var(--color-surface);
  color: var(--color-ink);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
}

.form-grid .form-actions {
  grid-column: 1 / -1;
}

.field label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 600;
  margin-bottom: var(--space-1);
}

.field input {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  background: var(--color-surface);
  color: var(--color-ink);
}

.field input:disabled {
  background: var(--color-surface-subtle);
  color: var(--color-ink-muted);
}

.form-actions {
  margin-top: var(--space-2);
}

.btn-primary {
  padding: var(--space-2) var(--space-5);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: none;
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  cursor: pointer;
}

.btn-primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-secondary {
  padding: var(--space-2) var(--space-4);
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  cursor: pointer;
}

.seg-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  font-size: var(--text-xs);
  font-weight: 600;
}

.seg-new {
  background: var(--color-surface-sunken);
  color: var(--color-ink-secondary);
}

.seg-regular {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.seg-vip {
  background: #fff3d6;
  color: #8a6100;
}

.seg-high_risk,
.seg-cod_risk {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.seg-src {
  font-weight: 400;
  opacity: 0.7;
}

.form-alert {
  margin-bottom: var(--space-4);
  padding: var(--space-3) var(--space-4);
  background: var(--color-danger-soft);
  color: var(--color-danger);
  border: 1px solid currentColor;
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.state-ok {
  margin-bottom: var(--space-4);
  padding: var(--space-2) var(--space-3);
  background: var(--color-success-soft);
  color: var(--color-success);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.state-block {
  padding: var(--space-8);
  text-align: center;
  color: var(--color-ink-secondary);
}

.hint {
  margin: 0;
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}
</style>
