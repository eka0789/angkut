<script setup lang="ts">
// Command Center (TASK-053): papan operasi dengan pemantauan berkala.
// Data dari /analytics/dashboard + daftar pengiriman berjalan/tertahan.
import { ApiRequestError } from '~/composables/useApi'
import type { Shipment } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { apiFetch } = useApi()

useHead({ title: () => t('cc.title') })

interface KPI {
  in_transit: number
  delayed: number
  failed: number
  out_for_delivery?: number
  cod_balance: number
}
interface CourierPerf { courier: string, total: number, delivered: number, failed: number, success_rate: number, avg_delivery_days: number }

const kpi = ref<KPI | null>(null)
const couriers = ref<CourierPerf[]>([])
const active = ref<Shipment[]>([])
const lastRefresh = ref<string>('')
const loading = ref(true)
const error = ref<string | null>(null)
let timer: ReturnType<typeof setInterval> | null = null

function errText(e: unknown): string {
  return e instanceof ApiRequestError ? e.message : t('common.error')
}

async function load() {
  error.value = null
  try {
    const [d, list] = await Promise.all([
      apiFetch<{ kpi: KPI, couriers: CourierPerf[] }>('/api/v1/analytics/dashboard'),
      apiFetch<{ items: Shipment[] }>('/api/v1/shipments', { query: { limit: 50 } }),
    ])
    kpi.value = d.kpi
    couriers.value = d.couriers ?? []
    active.value = (list.items ?? []).filter(s =>
      ['picked_up', 'in_transit', 'out_for_delivery', 'failed'].includes(s.status))
    lastRefresh.value = new Date().toLocaleTimeString('id-ID')
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

const inTransit = computed(() => active.value.filter(s => ['picked_up', 'in_transit'].includes(s.status)))
const outForDelivery = computed(() => active.value.filter(s => s.status === 'out_for_delivery'))
const failedList = computed(() => active.value.filter(s => s.status === 'failed'))

const statusLabel: Record<string, string> = {
  pending_pickup: 'Menunggu', picked_up: 'Dijemput', in_transit: 'Perjalanan',
  out_for_delivery: 'Diantar', delivered: 'Terkirim', failed: 'Gagal',
  returned: 'Kembali', cancelled: 'Batal',
}

onMounted(() => {
  void load()
  timer = setInterval(load, 30000)
})
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('cc.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('cc.subtitle') }}
          <span
            v-if="lastRefresh"
            class="refresh-tag"
          >{{ t('cc.refreshed') }} {{ lastRefresh }}</span>
        </p>
      </div>
      <button
        type="button"
        class="btn-secondary"
        @click="load"
      >
        {{ t('cc.refresh') }}
      </button>
    </header>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>

    <section
      v-if="loading && !kpi"
      class="card state-wrap"
    >
      <p class="state muted">
        {{ t('common.loading') }}
      </p>
    </section>

    <template v-else-if="kpi">
      <section class="kpi-grid">
        <div class="card kpi warn">
          <span class="kpi-label">{{ t('cc.in_transit') }}</span>
          <span class="kpi-value">{{ kpi.in_transit }}</span>
        </div>
        <div class="card kpi danger">
          <span class="kpi-label">{{ t('cc.delayed') }}</span>
          <span class="kpi-value">{{ kpi.delayed }}</span>
        </div>
        <div class="card kpi danger">
          <span class="kpi-label">{{ t('cc.failed') }}</span>
          <span class="kpi-value">{{ kpi.failed }}</span>
        </div>
        <div class="card kpi">
          <span class="kpi-label">{{ t('cc.cod_held') }}</span>
          <span class="kpi-value">Rp {{ kpi.cod_balance.toLocaleString('id-ID') }}</span>
        </div>
      </section>

      <div class="cols">
        <section class="card panel">
          <h2 class="panel-title">
            {{ t('cc.in_transit_list') }} ({{ inTransit.length }})
          </h2>
          <p
            v-if="inTransit.length === 0"
            class="muted"
          >
            {{ t('cc.none') }}
          </p>
          <ul class="ship-list">
            <li
              v-for="s in inTransit"
              :key="s.id"
            >
              <NuxtLink
                :to="`/shipments/${s.id}`"
                class="mono"
              >
                {{ s.awb }}
              </NuxtLink>
              <span class="muted">{{ statusLabel[s.status] }} · {{ s.courier?.code }}</span>
            </li>
          </ul>
        </section>

        <section class="card panel">
          <h2 class="panel-title">
            {{ t('cc.out_for_delivery') }} ({{ outForDelivery.length }})
          </h2>
          <p
            v-if="outForDelivery.length === 0"
            class="muted"
          >
            {{ t('cc.none') }}
          </p>
          <ul class="ship-list">
            <li
              v-for="s in outForDelivery"
              :key="s.id"
            >
              <NuxtLink
                :to="`/shipments/${s.id}`"
                class="mono"
              >
                {{ s.awb }}
              </NuxtLink>
              <span class="muted">{{ s.courier?.code }}</span>
            </li>
          </ul>
        </section>

        <section class="card panel">
          <h2 class="panel-title danger">
            {{ t('cc.failed_list') }} ({{ failedList.length }})
          </h2>
          <p
            v-if="failedList.length === 0"
            class="muted"
          >
            {{ t('cc.none_failed') }}
          </p>
          <ul class="ship-list">
            <li
              v-for="s in failedList"
              :key="s.id"
            >
              <NuxtLink
                :to="`/shipments/${s.id}`"
                class="mono danger"
              >
                {{ s.awb }}
              </NuxtLink>
              <span class="muted">{{ statusLabel[s.status] }}</span>
            </li>
          </ul>
        </section>
      </div>

      <section class="card table-wrap">
        <h2 class="panel-title">
          {{ t('cc.courier_perf') }}
        </h2>
        <table>
          <thead>
            <tr>
              <th>Kurir</th>
              <th class="num">Total</th>
              <th class="num">Terkirim</th>
              <th class="num">Gagal</th>
              <th class="num">Success Rate</th>
              <th class="num">Rata-rata Hari</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="c in couriers"
              :key="c.courier"
            >
              <td class="caps">{{ c.courier }}</td>
              <td class="num">{{ c.total }}</td>
              <td class="num">{{ c.delivered }}</td>
              <td class="num">{{ c.failed }}</td>
              <td class="num">
                <span class="rate" :class="c.success_rate >= 85 ? 'good' : 'bad'">{{ c.success_rate.toFixed(0) }}%</span>
              </td>
              <td class="num">{{ c.avg_delivery_days.toFixed(1) }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
  </div>
</template>

<style scoped>
.page-head { display: flex; justify-content: space-between; align-items: flex-start; gap: var(--space-4); margin-bottom: var(--space-5); }
.page-title { font-size: var(--text-xl); margin-bottom: var(--space-1); }
.page-subtitle { margin: 0; color: var(--color-ink-secondary); }
.refresh-tag { margin-left: var(--space-2); font-size: var(--text-xs); color: var(--color-ink-muted); }

.alert { padding: var(--space-3); border-radius: var(--radius-md); margin-bottom: var(--space-3); }
.alert.error { border: 1px solid #fecdd3; background: #fff1f2; color: #9f1239; }

.state-wrap { padding: var(--space-8); }
.state { text-align: center; }
.muted { color: var(--color-ink-muted); font-size: var(--text-sm); }

.kpi-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--space-3); margin-bottom: var(--space-4); }
@media (max-width: 900px) { .kpi-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .kpi-grid { grid-template-columns: 1fr; } }
.kpi { padding: var(--space-4); display: flex; flex-direction: column; gap: var(--space-1); border-left: 4px solid var(--color-border); }
.kpi.warn { border-left-color: #f59e0b; }
.kpi.danger { border-left-color: #dc2626; }
.kpi-label { font-size: var(--text-xs); text-transform: uppercase; color: var(--color-ink-muted); letter-spacing: 0.04em; }
.kpi-value { font-size: var(--text-lg); font-weight: 700; font-variant-numeric: tabular-nums; }

.cols { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: var(--space-4); margin-bottom: var(--space-4); }
.panel { padding: var(--space-4); }
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-3); }
.panel-title.danger { color: #9f1239; }

.ship-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
.ship-list li { display: flex; justify-content: space-between; gap: var(--space-2); font-size: var(--text-sm); }
.mono { font-family: ui-monospace, monospace; font-weight: 600; }
.danger { color: #9f1239; }

.table-wrap { padding: var(--space-3); }
table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; color: var(--color-ink-muted); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
.caps { text-transform: uppercase; font-weight: 600; }
.rate { font-weight: 700; }
.rate.good { color: #14532d; }
.rate.bad { color: #9f1239; }

.btn-secondary { padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); font-weight: 600; cursor: pointer; border: 1px solid var(--color-border); background: var(--color-surface); color: var(--color-ink); font: inherit; }
</style>
