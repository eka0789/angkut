<script setup lang="ts">
// Detail pengiriman: resi, kurir, berat, biaya, payload label, dan aksi
// (unduh label, ubah status, tautan lacak).
import { ApiRequestError } from '~/composables/useApi'
import type { Shipment, ShipmentLifecycleStatus } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { get, setStatus, cancel, downloadLabel, shareLink } = useShipments()
const route = useRoute()

const shipment = ref<Shipment | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)
const labelBusy = ref(false)

useHead({ title: () => t('shipment.detail_title') })

const NEXT_STATUS: Partial<Record<ShipmentLifecycleStatus, ShipmentLifecycleStatus>> = {
  pending_pickup: 'picked_up',
  picked_up: 'in_transit',
  in_transit: 'out_for_delivery',
  out_for_delivery: 'delivered',
  failed: 'pending_pickup',
}

const nextStatus = computed<ShipmentLifecycleStatus | null>(() => {
  const s = shipment.value?.status
  return s ? (NEXT_STATUS[s] ?? null) : null
})

const canCancel = computed(() => {
  const s = shipment.value?.status
  return Boolean(s) && s !== 'cancelled' && s !== 'delivered' && s !== 'returned'
})

function formatIDR(amount: number): string {
  return `Rp ${amount.toLocaleString('id-ID')}`
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

function itemName(it: Shipment['items'][number]): string {
  if ('name_snapshot' in it) return it.name_snapshot
  return 'product_name_snapshot' in it ? it.product_name_snapshot : '—'
}

function itemQty(it: Shipment['items'][number]): number {
  return it.qty
}

async function load() {
  loading.value = true
  error.value = null
  try {
    shipment.value = await get(String(route.params.id))
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function advance() {
  if (!shipment.value || !nextStatus.value) return
  error.value = null
  message.value = null
  try {
    shipment.value = await setStatus(shipment.value.id, nextStatus.value)
    message.value = t('shipment.status_updated')
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function cancelShipment() {
  if (!shipment.value) return
  const reason = prompt(t('shipment.cancel_reason'))
  if (reason === null) return
  error.value = null
  try {
    shipment.value = await cancel(shipment.value.id, reason.trim())
    message.value = t('shipment.status_updated')
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function download(format: 'a4' | 'thermal') {
  if (!shipment.value) return
  labelBusy.value = true
  error.value = null
  try {
    await downloadLabel(shipment.value.id, format)
  } catch {
    error.value = t('common.error')
  } finally {
    labelBusy.value = false
  }
}

async function copyShareLink() {
  if (!shipment.value) return
  try {
    const res = await shareLink(shipment.value.id)
    await navigator.clipboard.writeText(`${window.location.origin}${res.url}`)
    message.value = t('shipment.copied')
  } catch {
    error.value = t('common.error')
  }
}

onMounted(load)
</script>

<template>
  <div class="shipment-detail">
    <header class="page-head">
      <div>
        <NuxtLink
          to="/shipments"
          class="back"
        >
          ← {{ t('shipment.title') }}
        </NuxtLink>
        <h1 class="page-title mono">
          {{ shipment ? shipment.awb : t('shipment.detail_title') }}
        </h1>
      </div>
      <div
        v-if="shipment"
        class="head-actions"
      >
        <button
          type="button"
          class="btn-secondary"
          :disabled="labelBusy"
          @click="download('a4')"
        >
          {{ t('shipment.label_a4') }}
        </button>
        <button
          type="button"
          class="btn-secondary"
          :disabled="labelBusy"
          @click="download('thermal')"
        >
          {{ t('shipment.label_thermal') }}
        </button>
        <button
          type="button"
          class="btn-secondary"
          @click="copyShareLink"
        >
          {{ t('shipment.share_link') }}
        </button>
      </div>
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

    <template v-else-if="shipment">
      <section class="card section">
        <div class="row-between">
          <ShipmentStatusBadge :status="shipment.status" />
          <div class="actions">
            <button
              v-if="nextStatus"
              type="button"
              class="btn-primary"
              @click="advance"
            >
              {{ t('shipment.change_status') }}
            </button>
            <button
              v-if="canCancel"
              type="button"
              class="btn-secondary danger"
              @click="cancelShipment"
            >
              {{ t('shipment.cancel') }}
            </button>
          </div>
        </div>
        <dl class="grid">
          <div>
            <dt>{{ t('shipment.col_courier') }}</dt>
            <dd>{{ shipment.courier.code.toUpperCase() }} / {{ shipment.courier.service }}</dd>
          </div>
          <div>
            <dt>{{ t('shipment.eta') }}</dt>
            <dd>{{ shipment.eta_min_days }}–{{ shipment.eta_max_days }} hari</dd>
          </div>
          <div>
            <dt>{{ t('shipment.col_created') }}</dt>
            <dd>{{ formatDateTime(shipment.created_at) }}</dd>
          </div>
          <div v-if="shipment.order_id">
            <dt>{{ t('shipment.created_from') }}</dt>
            <dd>
              <NuxtLink
                :to="`/orders/${shipment.order_id}`"
                class="link"
              >
                {{ t('shipment.order_link') }}
              </NuxtLink>
            </dd>
          </div>
        </dl>
        <p
          v-if="shipment.notes"
          class="notes"
        >
          {{ t('order.notes') }}: {{ shipment.notes }}
        </p>
      </section>

      <div class="cols">
        <section class="card section">
          <h2 class="section-title">
            {{ t('shipment.package') }}
          </h2>
          <dl class="kv">
            <div>
              <dt>{{ t('shipment.weight_actual') }}</dt>
              <dd class="mono">
                {{ shipment.package.actual_weight }} kg
              </dd>
            </div>
            <div v-if="shipment.package.volumetric_weight > 0">
              <dt>{{ t('shipment.weight_volumetric') }}</dt>
              <dd class="mono">
                {{ shipment.package.volumetric_weight }} kg
              </dd>
            </div>
            <div>
              <dt>{{ t('shipment.weight_chargeable') }}</dt>
              <dd class="mono">
                {{ shipment.package.chargeable_weight }} kg
              </dd>
            </div>
            <div v-if="shipment.package.length">
              <dt>{{ t('shipment.dimensions') }}</dt>
              <dd class="mono">
                {{ shipment.package.length }}×{{ shipment.package.width }}×{{ shipment.package.height }} cm
              </dd>
            </div>
          </dl>

          <h3 class="sub-title">
            {{ t('shipment.items') }}
          </h3>
          <ul
            v-if="shipment.items.length"
            class="items"
          >
            <li
              v-for="(it, i) in shipment.items"
              :key="i"
            >
              {{ itemName(it) }} × {{ itemQty(it) }}
            </li>
          </ul>
          <p
            v-else
            class="muted"
          >
            —
          </p>
        </section>

        <section class="card section">
          <h2 class="section-title">
            {{ t('shipment.pricing') }}
          </h2>
          <dl class="kv">
            <div>
              <dt>{{ t('shipment.shipping_price') }}</dt>
              <dd class="mono">
                {{ formatIDR(shipment.pricing.shipping_price) }}
              </dd>
            </div>
            <div>
              <dt>{{ t('shipment.cost_price') }}</dt>
              <dd class="mono">
                {{ formatIDR(shipment.pricing.cost_price) }}
              </dd>
            </div>
            <div v-if="shipment.pricing.is_cod">
              <dt>{{ t('shipment.cod_amount') }}</dt>
              <dd class="mono">
                {{ formatIDR(shipment.pricing.cod_amount) }}
              </dd>
            </div>
            <div v-if="shipment.pricing.cod_fee > 0">
              <dt>{{ t('shipment.cod_fee') }}</dt>
              <dd class="mono">
                {{ formatIDR(shipment.pricing.cod_fee) }}
              </dd>
            </div>
            <div v-if="shipment.pricing.insurance_amount > 0">
              <dt>{{ t('shipment.insurance') }}</dt>
              <dd class="mono">
                {{ formatIDR(shipment.pricing.insurance_amount) }}
              </dd>
            </div>
          </dl>
        </section>
      </div>

      <section class="card section">
        <h2 class="section-title">
          {{ t('shipment.download_label') }}
        </h2>
        <dl class="kv">
          <div>
            <dt>{{ t('shipment.qr_payload') }}</dt>
            <dd class="mono wrap">
              {{ shipment.qr_payload }}
            </dd>
          </div>
          <div>
            <dt>{{ t('shipment.barcode') }}</dt>
            <dd class="mono">
              {{ shipment.barcode_value }}
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

.head-actions,
.actions {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
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

.sub-title {
  font-size: var(--text-sm);
  margin: var(--space-4) 0 var(--space-2);
}

.row-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  flex-wrap: wrap;
}

.cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--space-4);
}

.grid,
.kv {
  display: grid;
  gap: var(--space-3);
  margin: 0;
}

.grid {
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.kv > div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--space-2);
}

dt {
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}

dd {
  margin: 0;
  text-align: right;
  font-weight: 500;
}

.items {
  margin: 0;
  padding-left: var(--space-5);
}

.notes {
  margin: var(--space-3) 0 0;
  color: var(--color-ink-secondary);
}

.wrap {
  word-break: break-all;
  text-align: right;
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

.btn-secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>