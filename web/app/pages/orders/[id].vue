<script setup lang="ts">
// Detail pesanan: item, ringkasan biaya, dan aksi transisi status.
import { ApiRequestError } from '~/composables/useApi'
import type { Order, OrderStatus } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { get, setStatus, cancel } = useOrders()
const route = useRoute()

const order = ref<Order | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)

useHead({ title: () => t('order.detail_title') })

/** Transisi berikutnya yang sah — cerminan state machine server. */
const NEXT_STATUS: Partial<Record<OrderStatus, { status: OrderStatus, label: string }>> = {
  draft: { status: 'confirmed', label: t('order.mark_confirmed') },
  confirmed: { status: 'processing', label: t('order.mark_processing') },
  processing: { status: 'shipped', label: t('order.mark_shipped') },
  shipped: { status: 'delivered', label: t('order.mark_delivered') },
}

const nextStatus = computed(() => {
  const s = order.value?.status
  return s ? (NEXT_STATUS[s] ?? null) : null
})

const canCancel = computed(() => {
  const s = order.value?.status
  return Boolean(s) && s !== 'cancelled' && s !== 'delivered' && s !== 'returned'
})

function formatIDR(amount: number): string {
  return `Rp ${amount.toLocaleString('id-ID')}`
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

async function load() {
  loading.value = true
  error.value = null
  try {
    order.value = await get(String(route.params.id))
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function advance() {
  if (!order.value || !nextStatus.value) return
  error.value = null
  message.value = null
  try {
    order.value = await setStatus(order.value.id, nextStatus.value.status)
    message.value = t('order.status_updated')
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function cancelOrder() {
  if (!order.value) return
  const reason = prompt(t('order.cancel_reason'))
  if (reason === null) return
  if (!reason.trim()) {
    error.value = t('order.cancel_reason_required')
    return
  }
  error.value = null
  try {
    order.value = await cancel(order.value.id, reason.trim())
    message.value = t('order.status_updated')
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

onMounted(load)
</script>

<template>
  <div class="order-detail">
    <header class="page-head">
      <div>
        <NuxtLink
          to="/orders"
          class="back"
        >
          ← {{ t('order.title') }}
        </NuxtLink>
        <h1 class="page-title mono">
          {{ order ? order.order_number : t('order.detail_title') }}
        </h1>
      </div>
      <NuxtLink
        v-if="order"
        :to="`/shipments/new?order_id=${order.id}`"
        class="btn-primary"
      >
        {{ t('order.create_shipment') }}
      </NuxtLink>
    </header>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>
    <div
      v-if="message"
      class="alert ok"
      role="status"
    >
      {{ message }}
    </div>

    <p
      v-if="loading"
      class="card state muted"
    >
      {{ t('common.loading') }}
    </p>

    <template v-else-if="order">
      <section class="card section">
        <div class="row-between">
          <OrderStatusBadge :status="order.status" />
          <div class="actions">
            <button
              v-if="nextStatus"
              type="button"
              class="btn-primary"
              @click="advance"
            >
              {{ nextStatus.label }}
            </button>
            <button
              v-if="canCancel"
              type="button"
              class="btn-secondary danger"
              @click="cancelOrder"
            >
              {{ t('order.cancel') }}
            </button>
          </div>
        </div>
        <dl class="grid">
          <div>
            <dt>{{ t('order.col_payment') }}</dt>
            <dd>{{ order.payment_type === 'cod' ? t('order.payment_cod') : t('order.payment_prepaid') }}</dd>
          </div>
          <div>
            <dt>{{ t('order.col_created') }}</dt>
            <dd>{{ formatDateTime(order.created_at) }}</dd>
          </div>
          <div v-if="order.external_order_id">
            <dt>External ID</dt>
            <dd class="mono">
              {{ order.external_order_id }}
            </dd>
          </div>
        </dl>
        <p
          v-if="order.notes"
          class="notes"
        >
          {{ t('order.notes') }}: {{ order.notes }}
        </p>
        <p
          v-if="order.cancelled_reason"
          class="notes danger-text"
        >
          {{ t('order.cancel') }}: {{ order.cancelled_reason }}
        </p>
      </section>

      <section class="card section">
        <h2 class="section-title">
          {{ t('order.items') }}
        </h2>
        <table>
          <thead>
            <tr>
              <th scope="col">
                {{ t('order.item_product') }}
              </th>
              <th
                scope="col"
                class="num"
              >
                {{ t('order.item_qty') }}
              </th>
              <th
                scope="col"
                class="num"
              >
                {{ t('order.item_price') }}
              </th>
              <th
                scope="col"
                class="num"
              >
                {{ t('order.total') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="it in order.items"
              :key="it.id"
            >
              <td>
                {{ it.product_name_snapshot }}
                <span
                  v-if="it.variant_name_snapshot"
                  class="muted"
                >/ {{ it.variant_name_snapshot }}</span>
              </td>
              <td class="num">
                {{ it.qty }}
              </td>
              <td class="num mono">
                {{ formatIDR(it.unit_price) }}
              </td>
              <td class="num mono">
                {{ formatIDR(it.total_price) }}
              </td>
            </tr>
          </tbody>
        </table>

        <dl class="totals">
          <div>
            <dt>{{ t('order.subtotal') }}</dt>
            <dd class="mono">
              {{ formatIDR(order.subtotal) }}
            </dd>
          </div>
          <div>
            <dt>{{ t('order.shipping_cost') }}</dt>
            <dd class="mono">
              {{ formatIDR(order.shipping_cost) }}
            </dd>
          </div>
          <div v-if="order.discount > 0">
            <dt>{{ t('order.discount') }}</dt>
            <dd class="mono">
              −{{ formatIDR(order.discount) }}
            </dd>
          </div>
          <div class="grand">
            <dt>{{ t('order.total') }}</dt>
            <dd class="mono">
              {{ formatIDR(order.total_amount) }}
            </dd>
          </div>
        </dl>
      </section>
    </template>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
  margin-bottom: var(--space-5);
  flex-wrap: wrap;
}

.back {
  font-size: var(--text-sm);
}

.page-title {
  font-size: var(--text-xl);
  margin: var(--space-1) 0 0;
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

.section {
  padding: var(--space-5);
  margin-bottom: var(--space-4);
}

.section-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-3);
}

.row-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  flex-wrap: wrap;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--space-3);
  margin: 0;
}

.grid dt,
.totals dt {
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}

.grid dd {
  margin: 0;
  font-weight: 500;
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

.totals {
  margin: var(--space-4) 0 0;
  display: grid;
  gap: var(--space-2);
}

.totals > div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
}

.totals dd {
  margin: 0;
  font-weight: 500;
}

.totals .grand {
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-2);
  font-weight: 700;
}

.notes {
  margin: var(--space-3) 0 0;
  color: var(--color-ink-secondary);
}

.danger-text {
  color: #9f1239;
}

.state {
  padding: var(--space-6);
  text-align: center;
}

.btn-primary,
.btn-secondary {
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

.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}

.btn-secondary.danger {
  border-color: #fecdd3;
  color: #9f1239;
}
</style>