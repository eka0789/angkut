<script setup lang="ts">
// Daftar pengiriman tenant: filter status & kurir, pencarian resi, dan
// aksi cepat (unduh label, ubah status).
import { ApiRequestError } from '~/composables/useApi'
import type { Shipment, ShipmentLifecycleStatus } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { list, setStatus, cancel, downloadLabel } = useShipments()

useHead({ title: () => t('shipment.title') })

const items = ref<Shipment[]>([])
const total = ref(0)
const limit = 20
const offset = ref(0)
const loading = ref(true)
const loadError = ref<string | null>(null)
const actionError = ref<string | null>(null)
const message = ref<string | null>(null)
const query = ref('')
const statusFilter = ref<'' | ShipmentLifecycleStatus>('')
const labelBusy = ref<string | null>(null)

const statusOptions: Array<{ value: '' | ShipmentLifecycleStatus, label: string }> = [
  { value: '', label: t('order.status_all') },
  { value: 'pending_pickup', label: t('shipment.status.pending_pickup') },
  { value: 'picked_up', label: t('shipment.status.picked_up') },
  { value: 'in_transit', label: t('shipment.status.in_transit') },
  { value: 'out_for_delivery', label: t('shipment.status.out_for_delivery') },
  { value: 'delivered', label: t('shipment.status.delivered') },
  { value: 'failed', label: t('shipment.status.failed') },
  { value: 'returned', label: t('shipment.status.returned') },
  { value: 'cancelled', label: t('shipment.status.cancelled') },
]

/**
 * Transisi berikutnya yang sah, mengikuti state machine server. Rantai ini
 * sengaja tidak melompati tahap: server menolak lompatan dengan 409.
 */
const NEXT_STATUS: Partial<Record<ShipmentLifecycleStatus, ShipmentLifecycleStatus>> = {
  pending_pickup: 'picked_up',
  picked_up: 'in_transit',
  in_transit: 'out_for_delivery',
  out_for_delivery: 'delivered',
  failed: 'pending_pickup',
}

function nextStatusOf(s: ShipmentLifecycleStatus): ShipmentLifecycleStatus | null {
  return NEXT_STATUS[s] ?? null
}

function canCancel(s: ShipmentLifecycleStatus): boolean {
  return s !== 'cancelled' && s !== 'delivered' && s !== 'returned'
}

function formatIDR(amount: number): string {
  return `Rp ${amount.toLocaleString('id-ID')}`
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('id-ID', { dateStyle: 'medium' })
}

async function loadList() {
  loading.value = true
  loadError.value = null
  try {
    const res = await list({
      status: statusFilter.value || undefined,
      search: query.value.trim() || undefined,
      limit,
      offset: offset.value,
    })
    items.value = res.items
    total.value = res.meta.total
  } catch (err: unknown) {
    loadError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function advance(s: Shipment) {
  const next = nextStatusOf(s.status)
  if (!next) return
  actionError.value = null
  message.value = null
  try {
    await setStatus(s.id, next)
    message.value = t('shipment.status_updated')
    await loadList()
  } catch (err: unknown) {
    actionError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function cancelShipment(s: Shipment) {
  const reason = prompt(t('shipment.cancel_reason'))
  if (reason === null) return
  actionError.value = null
  try {
    await cancel(s.id, reason.trim())
    message.value = t('shipment.status_updated')
    await loadList()
  } catch (err: unknown) {
    actionError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function download(s: Shipment, format: 'a4' | 'thermal') {
  labelBusy.value = s.id
  actionError.value = null
  try {
    await downloadLabel(s.id, format)
  } catch {
    actionError.value = t('common.error')
  } finally {
    labelBusy.value = null
  }
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(query, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    offset.value = 0
    void loadList()
  }, 300)
})
watch(statusFilter, () => {
  offset.value = 0
  void loadList()
})

function prevPage() {
  if (offset.value >= limit) {
    offset.value -= limit
    void loadList()
  }
}

function nextPage() {
  if (offset.value + limit < total.value) {
    offset.value += limit
    void loadList()
  }
}

onMounted(loadList)
</script>

<template>
  <div class="shipments">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('shipment.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('shipment.subtitle') }}
        </p>
      </div>
      <div class="head-actions">
        <NuxtLink
          to="/shipments/bulk"
          class="btn-secondary"
        >
          {{ t('bulk.title') }}
        </NuxtLink>
        <NuxtLink
          to="/shipments/new"
          class="btn-primary"
        >
          {{ t('shipment.add') }}
        </NuxtLink>
      </div>
    </header>

    <div
      v-if="loadError"
      class="alert error"
      role="alert"
    >
      {{ loadError }}
    </div>
    <div
      v-if="actionError"
      class="alert error"
      role="alert"
    >
      {{ actionError }}
    </div>
    <div
      v-if="message"
      class="alert ok"
      role="status"
    >
      {{ message }}
    </div>

    <section class="card toolbar">
      <input
        v-model="query"
        type="search"
        class="search"
        :placeholder="t('shipment.search_placeholder')"
      >
      <select
        v-model="statusFilter"
        class="filter"
        :aria-label="t('shipment.filter_status')"
      >
        <option
          v-for="o in statusOptions"
          :key="o.value"
          :value="o.value"
        >
          {{ o.label }}
        </option>
      </select>
    </section>

    <section class="card table-wrap">
      <p
        v-if="loading"
        class="state muted"
      >
        {{ t('common.loading') }}
      </p>
      <p
        v-else-if="items.length === 0"
        class="state muted"
      >
        {{ query || statusFilter ? t('shipment.empty_filtered') : t('shipment.empty') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">
              {{ t('shipment.col_awb') }}
            </th>
            <th scope="col">
              {{ t('shipment.col_courier') }}
            </th>
            <th scope="col">
              {{ t('shipment.col_status') }}
            </th>
            <th
              scope="col"
              class="num"
            >
              {{ t('shipment.col_cod') }}
            </th>
            <th
              scope="col"
              class="num"
            >
              {{ t('shipment.col_price') }}
            </th>
            <th scope="col">
              {{ t('shipment.col_created') }}
            </th>
            <th scope="col">
              {{ t('common.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="s in items"
            :key="s.id"
          >
            <td>
              <NuxtLink
                :to="`/shipments/${s.id}`"
                class="mono link"
              >
                {{ s.awb }}
              </NuxtLink>
            </td>
            <td>
              {{ s.courier.code.toUpperCase() }}
              <span class="muted">/ {{ s.courier.service }}</span>
            </td>
            <td>
              <ShipmentStatusBadge :status="s.status" />
            </td>
            <td class="num mono">
              {{ s.pricing.is_cod ? formatIDR(s.pricing.cod_amount) : '—' }}
            </td>
            <td class="num mono">
              {{ formatIDR(s.pricing.shipping_price) }}
            </td>
            <td class="muted">
              {{ formatDate(s.created_at) }}
            </td>
            <td class="actions">
              <button
                type="button"
                class="btn-mini"
                :disabled="labelBusy === s.id"
                @click="download(s, 'a4')"
              >
                {{ t('shipment.download_label') }}
              </button>
              <button
                v-if="nextStatusOf(s.status)"
                type="button"
                class="btn-mini"
                @click="advance(s)"
              >
                {{ t('shipment.change_status') }}
              </button>
              <button
                v-if="canCancel(s.status)"
                type="button"
                class="btn-mini danger"
                @click="cancelShipment(s)"
              >
                {{ t('shipment.cancel') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <div
        v-if="total > limit"
        class="pager"
      >
        <button
          type="button"
          class="btn-secondary"
          :disabled="offset === 0"
          @click="prevPage"
        >
          {{ t('common.prev') }}
        </button>
        <span class="muted">{{ offset + 1 }}–{{ Math.min(offset + limit, total) }} / {{ total }}</span>
        <button
          type="button"
          class="btn-secondary"
          :disabled="offset + limit >= total"
          @click="nextPage"
        >
          {{ t('common.next') }}
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
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

.alert.ok {
  border: 1px solid #bbf7d0;
  background: #f0fdf4;
  color: #14532d;
}

.toolbar {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  margin-bottom: var(--space-4);
}

.search,
.filter {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

.search {
  flex: 1;
  min-width: 200px;
}

.table-wrap {
  padding: var(--space-4);
  overflow-x: auto;
}

.state {
  padding: var(--space-6) 0;
  text-align: center;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

th,
td {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
}

th {
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: 0.02em;
  color: var(--color-ink-muted);
}

.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.link {
  font-weight: 600;
}

.actions {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.head-actions {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}

.btn-primary,
.btn-secondary,
.btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: 0;
}

.btn-secondary,
.btn-mini {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}

.btn-mini {
  padding: var(--space-1) var(--space-2);
  font-size: var(--text-xs);
}

.btn-mini.danger {
  border-color: #fecdd3;
  color: #9f1239;
}

.btn-mini:disabled,
.btn-secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  padding-top: var(--space-4);
}
</style>