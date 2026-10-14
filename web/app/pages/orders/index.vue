<script setup lang="ts">
// Daftar pesanan tenant: filter status, pencarian nomor, ringkasan, dan
// aksi transisi status (state machine ditegakkan server).
import { ApiRequestError } from '~/composables/useApi'
import type { Order, OrderStatus, OrderSummary } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { list, summary, setStatus, cancel } = useOrders()

useHead({ title: () => t('order.title') })

const items = ref<Order[]>([])
const stats = ref<OrderSummary | null>(null)
const total = ref(0)
const limit = 20
const offset = ref(0)
const loading = ref(true)
const loadError = ref<string | null>(null)
const actionError = ref<string | null>(null)
const message = ref<string | null>(null)
const query = ref('')
const statusFilter = ref<'' | OrderStatus>('')

/** Urutan status & tombol transisi berikutnya yang sah per status. */
const statusOptions: Array<{ value: '' | OrderStatus, label: string }> = [
  { value: '', label: t('order.status_all') },
  { value: 'draft', label: t('order.status.draft') },
  { value: 'confirmed', label: t('order.status.confirmed') },
  { value: 'processing', label: t('order.status.processing') },
  { value: 'shipped', label: t('order.status.shipped') },
  { value: 'delivered', label: t('order.status.delivered') },
  { value: 'cancelled', label: t('order.status.cancelled') },
  { value: 'returned', label: t('order.status.returned') },
]

const NEXT_STATUS: Partial<Record<OrderStatus, { status: OrderStatus, label: string }>> = {
  draft: { status: 'confirmed', label: t('order.mark_confirmed') },
  confirmed: { status: 'processing', label: t('order.mark_processing') },
  processing: { status: 'shipped', label: t('order.mark_shipped') },
  shipped: { status: 'delivered', label: t('order.mark_delivered') },
}

function nextStatusOf(s: OrderStatus) {
  return NEXT_STATUS[s] ?? null
}

function canCancel(s: OrderStatus): boolean {
  return s !== 'cancelled' && s !== 'delivered' && s !== 'returned'
}

function formatIDR(amount: number): string {
  return `Rp ${amount.toLocaleString('id-ID')}`
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
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

async function loadSummary() {
  try {
    stats.value = await summary()
  } catch {
    stats.value = null
  }
}

/** Menjalankan transisi status dan melaporkan kegagalan transisi ilegal apa adanya. */
async function advance(o: Order) {
  const next = nextStatusOf(o.status)
  if (!next) return
  actionError.value = null
  message.value = null
  try {
    await setStatus(o.id, next.status)
    message.value = t('order.status_updated')
    await Promise.all([loadList(), loadSummary()])
  } catch (err: unknown) {
    actionError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function cancelOrder(o: Order) {
  const reason = prompt(t('order.cancel_reason'))
  if (reason === null) return
  if (!reason.trim()) {
    actionError.value = t('order.cancel_reason_required')
    return
  }
  actionError.value = null
  try {
    await cancel(o.id, reason.trim())
    message.value = t('order.status_updated')
    await Promise.all([loadList(), loadSummary()])
  } catch (err: unknown) {
    actionError.value = err instanceof ApiRequestError ? err.message : t('common.error')
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

onMounted(() => {
  void loadList()
  void loadSummary()
})
</script>

<template>
  <div class="orders">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('order.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('order.subtitle') }}
        </p>
      </div>
      <NuxtLink
        to="/orders/new"
        class="btn-primary"
      >
        {{ t('order.add') }}
      </NuxtLink>
    </header>

    <section
      v-if="stats"
      class="stats"
    >
      <div class="stat card">
        <span class="stat-label">{{ t('order.col_created') }} (hari ini)</span>
        <span class="stat-value">{{ stats.today }}</span>
      </div>
      <div class="stat card">
        <span class="stat-label">{{ t('order.total') }}</span>
        <span class="stat-value">{{ formatIDR(stats.total_value) }}</span>
      </div>
      <div class="stat card">
        <span class="stat-label">{{ t('order.status.draft') }}</span>
        <span class="stat-value">{{ stats.status_counts.draft ?? 0 }}</span>
      </div>
      <div class="stat card">
        <span class="stat-label">{{ t('order.status.processing') }}</span>
        <span class="stat-value">{{ stats.status_counts.processing ?? 0 }}</span>
      </div>
    </section>

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
        :placeholder="t('order.search_placeholder')"
      >
      <select
        v-model="statusFilter"
        class="filter"
        :aria-label="t('order.filter_status')"
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
        {{ query || statusFilter ? t('order.empty_filtered') : t('order.empty') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">
              {{ t('order.col_number') }}
            </th>
            <th scope="col">
              {{ t('order.col_status') }}
            </th>
            <th scope="col">
              {{ t('order.col_payment') }}
            </th>
            <th
              scope="col"
              class="num"
            >
              {{ t('order.col_total') }}
            </th>
            <th scope="col">
              {{ t('order.col_created') }}
            </th>
            <th scope="col">
              {{ t('common.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="o in items"
            :key="o.id"
          >
            <td>
              <NuxtLink
                :to="`/orders/${o.id}`"
                class="mono link"
              >
                {{ o.order_number }}
              </NuxtLink>
            </td>
            <td>
              <OrderStatusBadge :status="o.status" />
            </td>
            <td>{{ o.payment_type === 'cod' ? t('order.payment_cod') : t('order.payment_prepaid') }}</td>
            <td class="num mono">
              {{ formatIDR(o.total_amount) }}
            </td>
            <td class="muted">
              {{ formatDate(o.created_at) }}
            </td>
            <td class="actions">
              <button
                v-if="nextStatusOf(o.status)"
                type="button"
                class="btn-mini"
                @click="advance(o)"
              >
                {{ nextStatusOf(o.status)?.label }}
              </button>
              <button
                v-if="canCancel(o.status)"
                type="button"
                class="btn-mini danger"
                @click="cancelOrder(o)"
              >
                {{ t('order.cancel') }}
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

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}

.stat {
  padding: var(--space-3) var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.stat-label {
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.stat-value {
  font-size: var(--text-md);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
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