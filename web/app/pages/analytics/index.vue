<script setup lang="ts">
// Analytics (TASK-052/055): chart 30 hari (bar CSS murni), performa kurir,
// health score + rekomendasi. Tanpa lib chart agar build tetap ringan.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { apiFetch } = useApi()

useHead({ title: () => t('an.title') })

interface ChartPoint { date: string, value: number }
interface CourierPerf { courier: string, total: number, delivered: number, failed: number, success_rate: number, avg_delivery_days: number }
interface Health { score: number, factors: { name: string, score: number, weight: number }[], recommendations: string[] }
interface Dashboard { kpi: { shipping_cost: number, avg_delivery_days: number }, orders_over_time: ChartPoint[], shipping_cost: ChartPoint[], couriers: CourierPerf[] }

const dash = ref<Dashboard | null>(null)
const health = ref<Health | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)

const fmtIDR = (v: number): string => `Rp ${v.toLocaleString('id-ID')}`

function errText(e: unknown): string {
  return e instanceof ApiRequestError ? e.message : t('common.error')
}

const factorLabel: Record<string, string> = {
  delivery_success: 'Delivery Success', speed: 'Kecepatan', returns: 'Retur', cod_collection: 'COD Collection',
}

const maxOrders = computed(() => Math.max(1, ...(dash.value?.orders_over_time ?? []).map(p => p.value)))
const maxCost = computed(() => Math.max(1, ...(dash.value?.shipping_cost ?? []).map(p => p.value)))
const totalCost30 = computed(() => (dash.value?.shipping_cost ?? []).reduce((a, p) => a + p.value, 0))

function shortDate(d: string): string {
  return d.slice(8) + '/' + d.slice(5, 7)
}

onMounted(async () => {
  try {
    const [d, h] = await Promise.all([
      apiFetch<Dashboard>('/api/v1/analytics/dashboard'),
      apiFetch<Health>('/api/v1/analytics/health-score'),
    ])
    dash.value = d
    health.value = h
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('an.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('an.subtitle') }}
        </p>
      </div>
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

    <template v-else-if="dash">
      <section class="cards">
        <div class="card stat">
          <span class="stat-label">{{ t('an.cost_30d') }}</span>
          <span class="stat-value"><AppCountUp :value="totalCost30" currency /></span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('an.avg_days') }}</span>
          <span class="stat-value"><AppCountUp :value="dash.kpi.avg_delivery_days" :decimals="1" /></span>
        </div>
        <div class="card stat">
          <span class="stat-label">{{ t('dash.health') }}</span>
          <span class="stat-value"><AppCountUp :value="Math.round(health?.score ?? 0)" />/100</span>
        </div>
      </section>

      <div class="cols">
        <section class="card panel">
          <h2 class="panel-title">
            {{ t('an.chart_orders') }}
          </h2>
          <div class="bars">
            <div
              v-for="p in dash.orders_over_time"
              :key="p.date"
              class="bar-col"
              :title="`${p.date}: ${p.value}`"
            >
              <div
                class="bar"
                :style="{ height: Math.max(4, (p.value / maxOrders) * 100) + 'px' }"
              />
              <span class="bar-label">{{ shortDate(p.date) }}</span>
            </div>
            <p
              v-if="dash.orders_over_time.length === 0"
              class="muted"
            >
              {{ t('an.no_data') }}
            </p>
          </div>
        </section>

        <section class="card panel">
          <h2 class="panel-title">
            {{ t('an.chart_cost') }}
          </h2>
          <div class="bars">
            <div
              v-for="p in dash.shipping_cost"
              :key="p.date"
              class="bar-col"
              :title="`${p.date}: ${fmtIDR(p.value)}`"
            >
              <div
                class="bar cost"
                :style="{ height: Math.max(4, (p.value / maxCost) * 100) + 'px' }"
              />
              <span class="bar-label">{{ shortDate(p.date) }}</span>
            </div>
            <p
              v-if="dash.shipping_cost.length === 0"
              class="muted"
            >
              {{ t('an.no_data') }}
            </p>
          </div>
        </section>
      </div>

      <div class="cols">
        <section class="card panel">
          <h2 class="panel-title">
            {{ t('an.courier_perf') }}
          </h2>
          <table>
            <thead>
              <tr>
                <th>Kurir</th>
                <th class="num">Total</th>
                <th class="num">Success</th>
                <th class="num">Hari</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="c in dash.couriers"
                :key="c.courier"
              >
                <td class="caps">{{ c.courier }}</td>
                <td class="num">{{ c.total }}</td>
                <td class="num">
                  <span class="rate" :class="c.success_rate >= 85 ? 'good' : 'bad'">{{ c.success_rate.toFixed(0) }}%</span>
                </td>
                <td class="num">{{ c.avg_delivery_days.toFixed(1) }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="card panel">
          <h2 class="panel-title">
            {{ t('dash.health') }}
          </h2>
          <div class="factors">
            <div
              v-for="f in health?.factors ?? []"
              :key="f.name"
              class="factor"
            >
              <span class="factor-name">{{ factorLabel[f.name] ?? f.name }}</span>
              <div class="factor-bar">
                <div
                  class="factor-fill"
                  :style="{ width: f.score + '%' }"
                />
              </div>
              <span class="factor-score">{{ f.score.toFixed(0) }}</span>
            </div>
          </div>
          <ul class="recs">
            <li
              v-for="(rec, i) in health?.recommendations ?? []"
              :key="i"
            >
              {{ rec }}
            </li>
          </ul>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page-head { display: flex; justify-content: space-between; align-items: flex-start; gap: var(--space-4); margin-bottom: var(--space-5); }
.page-title { font-size: var(--text-xl); margin-bottom: var(--space-1); }
.page-subtitle { margin: 0; color: var(--color-ink-secondary); }

.alert { padding: var(--space-3); border-radius: var(--radius-md); margin-bottom: var(--space-3); }
.alert.error { border: 1px solid #fecdd3; background: #fff1f2; color: #9f1239; }

.state { padding: var(--space-6) 0; }
.muted { color: var(--color-ink-muted); font-size: var(--text-sm); }

.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); margin-bottom: var(--space-4); }
.stat { padding: var(--space-4); display: flex; flex-direction: column; gap: var(--space-1); }
.stat-label { font-size: var(--text-xs); text-transform: uppercase; color: var(--color-ink-muted); letter-spacing: 0.04em; }
.stat-value { font-size: var(--text-lg); font-weight: 700; font-variant-numeric: tabular-nums; }

.cols { display: grid; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); gap: var(--space-4); margin-bottom: var(--space-4); align-items: stretch; }
.cols .panel { height: 100%; }
.panel { padding: var(--space-4); }
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-3); }

.bars { display: flex; align-items: flex-end; gap: 3px; height: 120px; overflow-x: auto; }
.bar-col { display: flex; flex-direction: column; align-items: center; gap: 2px; min-width: 18px; flex: 1; }
.bar { width: 100%; background: var(--color-accent, #0f766e); border-radius: 3px 3px 0 0; opacity: 0.85; }
.bar.cost { background: #b45309; }
.bar-label { font-size: 9px; color: var(--color-ink-muted); transform: rotate(-45deg); white-space: nowrap; }

table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; color: var(--color-ink-muted); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
.caps { text-transform: uppercase; font-weight: 600; }
.rate { font-weight: 700; }
.rate.good { color: #14532d; }
.rate.bad { color: #9f1239; }

.factors { display: flex; flex-direction: column; gap: var(--space-2); margin-bottom: var(--space-3); }
.factor { display: flex; align-items: center; gap: var(--space-3); font-size: var(--text-sm); }
.factor-name { width: 130px; color: var(--color-ink-secondary); }
.factor-bar { flex: 1; height: 8px; background: var(--color-border); border-radius: 999px; overflow: hidden; }
.factor-fill { height: 100%; background: var(--color-accent, #0f766e); }
.factor-score { width: 30px; text-align: right; font-weight: 700; font-variant-numeric: tabular-nums; }
.recs { margin: 0; padding-left: var(--space-4); font-size: var(--text-sm); color: var(--color-ink-secondary); }
</style>
