<script setup lang="ts">
// Dasbor (TASK-052/053): KPI dari /analytics/dashboard + insight AI
// (TASK-054) + skor kesehatan (TASK-055). Kosong tetap ramah untuk org baru.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'default' })

const { t } = useT()
const { apiFetch } = useApi()

useHead({ title: () => t('dashboard.title') })

interface KPI {
  orders_today: number
  shipments_today: number
  in_transit: number
  delivered: number
  delayed: number
  failed: number
  cod_balance: number
  shipping_cost: number
  avg_delivery_days: number
}
interface Insight { title: string, body: string, level: string }
interface Health { score: number, recommendations: string[] }

const kpi = ref<KPI | null>(null)
const insights = ref<Insight[]>([])
const health = ref<Health | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)

const fmtIDR = (v: number): string => `Rp ${v.toLocaleString('id-ID')}`

async function load() {
  loading.value = true
  error.value = null
  try {
    const [d, ins, hs] = await Promise.all([
      apiFetch<{ kpi: KPI }>('/api/v1/analytics/dashboard'),
      apiFetch<Insight[]>('/api/v1/ai/insights'),
      apiFetch<Health>('/api/v1/analytics/health-score'),
    ])
    kpi.value = d.kpi
    insights.value = ins
    health.value = hs
  } catch (e) {
    error.value = e instanceof ApiRequestError ? e.message : t('common.error')
  } finally {
    loading.value = false
  }
}

const empty = computed(() =>
  kpi.value && kpi.value.orders_today === 0 && kpi.value.shipments_today === 0 && kpi.value.delivered === 0)

const healthColor = computed(() => {
  const s = health.value?.score ?? 0
  if (s >= 85) return '#14532d'
  if (s >= 70) return '#78350f'
  return '#9f1239'
})

onMounted(load)
</script>

<template>
  <div class="dashboard">
    <header class="page-head">
      <h1 class="page-title">
        {{ t('dashboard.title') }}
      </h1>
      <p class="page-subtitle">
        {{ t('dashboard.subtitle') }}
      </p>
    </header>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>

    <p
      v-if="loading"
      class="state muted"
    >
      {{ t('common.loading') }}
    </p>

    <template v-else-if="empty">
      <section class="card empty">
        <p class="empty-icon" aria-hidden="true">▦</p>
        <h2 class="empty-title">
          {{ t('dashboard.emptyTitle') }}
        </h2>
        <p class="empty-body">
          {{ t('dashboard.emptyBody') }}
        </p>
        <div class="empty-actions">
          <NuxtLink
            to="/shipments/new"
            class="btn-primary"
          >
            Buat pengiriman
          </NuxtLink>
          <NuxtLink
            to="/stores"
            class="btn-secondary"
          >
            Hubungkan toko
          </NuxtLink>
        </div>
      </section>
    </template>

    <template v-else-if="kpi">
      <section class="cards">
        <div class="card stat">
          <span class="stat-label">{{ t('dash.shipments_today') }}</span>
          <span class="stat-value">{{ kpi.shipments_today }}</span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('dash.in_transit') }}</span>
          <span class="stat-value">{{ kpi.in_transit }}</span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('dash.delivered') }}</span>
          <span class="stat-value">{{ kpi.delivered }}</span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('dash.cod_balance') }}</span>
          <span class="stat-value">{{ fmtIDR(kpi.cod_balance) }}</span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('dash.shipping_cost') }}</span>
          <span class="stat-value">{{ fmtIDR(kpi.shipping_cost) }}</span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('dash.avg_days') }}</span>
          <span class="stat-value">{{ kpi.avg_delivery_days.toFixed(1) }}</span>
        </div>
      </section>

      <div class="cols">
        <section class="card panel">
          <h2 class="panel-title">
            {{ t('dash.insights') }}
          </h2>
          <ul class="insights">
            <li
              v-for="(ins, i) in insights"
              :key="i"
              class="insight"
              :class="ins.level"
            >
              <strong>{{ ins.title }}</strong>
              <span>{{ ins.body }}</span>
            </li>
          </ul>
        </section>

        <section class="card panel">
          <h2 class="panel-title">
            {{ t('dash.health') }}
          </h2>
          <div class="health-row">
            <span
              class="health-score"
              :style="{ color: healthColor }"
            >
              {{ Math.round(health?.score ?? 0) }}
            </span>
            <ul class="recs">
              <li
                v-for="(rec, i) in health?.recommendations ?? []"
                :key="i"
              >
                {{ rec }}
              </li>
            </ul>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
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

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-3);
}
.alert.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.state {
  padding: var(--space-6) 0;
}
.muted {
  color: var(--color-ink-muted);
}

.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}
.stat {
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.stat-label {
  font-size: var(--text-xs);
  text-transform: uppercase;
  color: var(--color-ink-muted);
  letter-spacing: 0.04em;
}
.stat-value {
  font-size: var(--text-lg);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--space-4);
}
.panel {
  padding: var(--space-4);
}
.panel-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-3);
}

.insights {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.insight {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}
.insight.warning {
  border-color: #fde68a;
  background: #fffbeb;
}
.insight span {
  color: var(--color-ink-secondary);
}

.health-row {
  display: flex;
  gap: var(--space-4);
  align-items: center;
}
.health-score {
  font-size: 42px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}
.recs {
  margin: 0;
  padding-left: var(--space-4);
  font-size: var(--text-sm);
  color: var(--color-ink-secondary);
}

.empty {
  padding: var(--space-16) var(--space-6);
  text-align: center;
}
.empty-icon {
  font-size: 40px;
  color: var(--color-ink-muted);
  margin: 0 0 var(--space-4);
}
.empty-title {
  font-size: var(--text-lg);
  margin-bottom: var(--space-2);
}
.empty-body {
  max-width: 54ch;
  margin: 0 auto var(--space-6);
  color: var(--color-ink-secondary);
  line-height: var(--leading-relaxed);
}
.empty-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: center;
  flex-wrap: wrap;
}
.btn-primary,
.btn-secondary {
  padding: var(--space-3) var(--space-5);
  border-radius: var(--radius-md);
  font-weight: 600;
}
.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
}
.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}
</style>
