<script setup lang="ts">
// Saldo stok per varian per gudang (TASK-037): tabel, filter gudang,
// low-stock saja, dan dialog penyesuaian (adjust) berjejak.
import { ApiRequestError } from '~/composables/useApi'
import type { InventoryStock, LowStockRow } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { listStock, adjust, lowStock } = useInventory()
const { list: listWarehouses } = useWarehouses()

useHead({ title: () => t('inv.title') })

const rows = ref<InventoryStock[]>([])
const total = ref(0)
const limit = 50
const offset = ref(0)
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)
const search = ref('')
const warehouseId = ref('')
const warehouses = ref<{ id: string, name: string }[]>([])
const lowRows = ref<LowStockRow[]>([])

// Dialog adjust.
const adjusting = ref<InventoryStock | null>(null)
const adjDelta = ref<number | null>(null)
const adjReason = ref('')
const adjBusy = ref(false)

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) {
    if (e.status === 403) return t('inv.forbidden')
    const fields = e.fields ? Object.values(e.fields) : []
    return [e.message, ...fields].filter(Boolean).join(' ')
  }
  return t('common.error')
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const res = await listStock({
      warehouse_id: warehouseId.value || undefined,
      search: search.value || undefined,
      limit,
      offset: offset.value,
    })
    rows.value = res.items
    total.value = res.meta.total
    lowRows.value = await lowStock(warehouseId.value || undefined)
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

async function submitAdjust() {
  if (!adjusting.value || !adjDelta.value) return
  adjBusy.value = true
  error.value = null
  try {
    await adjust({
      variant_id: adjusting.value.variant_id,
      warehouse_id: adjusting.value.warehouse_id,
      qty_delta: adjDelta.value,
      reason: adjReason.value || undefined,
    })
    message.value = t('inv.adjusted')
    adjusting.value = null
    adjDelta.value = null
    adjReason.value = ''
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    adjBusy.value = false
  }
}

const isLow = (s: InventoryStock): boolean => s.qty_on_hand - s.qty_reserved <= s.low_stock_threshold

onMounted(async () => {
  try {
    warehouses.value = await listWarehouses()
  } catch {
    // Daftar gudang gagal: filter tetap bisa dipakai manual.
  }
  await load()
})
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('inv.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('inv.subtitle') }}
        </p>
      </div>
      <NuxtLink
        to="/inventory/movements"
        class="btn-secondary"
      >
        {{ t('inv.movements_title') }}
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

    <section
      v-if="lowRows.length > 0"
      class="alert warn"
      role="status"
    >
      <strong>{{ t('inv.low_stock_title') }}:</strong>
      <ul class="low-list">
        <li
          v-for="lr in lowRows.slice(0, 5)"
          :key="lr.variant_id + lr.warehouse_id"
        >
          {{ lr.product_name }} ({{ lr.sku }}) — {{ t('inv.available') }} {{ lr.qty_available }}
        </li>
      </ul>
      <span
        v-if="lowRows.length > 5"
        class="muted"
      >
        +{{ lowRows.length - 5 }} {{ t('inv.more_items') }}
      </span>
    </section>

    <section class="card toolbar">
      <input
        v-model="search"
        type="search"
        class="search"
        :placeholder="t('inv.search_stock_placeholder')"
        @keydown.enter="load(); offset = 0"
      >
      <select
        v-model="warehouseId"
        class="filter"
        :aria-label="t('inv.col_warehouse')"
        @change="load(); offset = 0"
      >
        <option value="">
          {{ t('inv.all_warehouses') }}
        </option>
        <option
          v-for="w in warehouses"
          :key="w.id"
          :value="w.id"
        >
          {{ w.name }}
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
        v-else-if="rows.length === 0"
        class="state muted"
      >
        {{ t('inv.empty_stock') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">
              {{ t('inv.col_product') }}
            </th>
            <th scope="col">
              SKU
            </th>
            <th scope="col" class="num">
              {{ t('inv.col_on_hand') }}
            </th>
            <th scope="col" class="num">
              {{ t('inv.col_reserved') }}
            </th>
            <th scope="col" class="num">
              {{ t('inv.available') }}
            </th>
            <th scope="col">
              {{ t('common.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="s in rows"
            :key="s.id"
          >
            <td>{{ s.product_name }}</td>
            <td>{{ s.sku }}</td>
            <td class="num">
              {{ s.qty_on_hand }}
            </td>
            <td class="num">
              {{ s.qty_reserved }}
            </td>
            <td
              class="num"
              :class="{ low: isLow(s) }"
            >
              {{ s.qty_on_hand - s.qty_reserved }}
            </td>
            <td>
              <button
                type="button"
                class="btn-mini"
                @click="adjusting = s; adjDelta = null; adjReason = ''"
              >
                {{ t('inv.adjust') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <div
      v-if="total > limit"
      class="pager"
    >
      <button
        type="button"
        class="btn-mini"
        :disabled="offset === 0"
        @click="offset = Math.max(0, offset - limit); load()"
      >
        {{ t('common.back') }}
      </button>
      <span class="muted">{{ offset + 1 }}–{{ Math.min(offset + limit, total) }} / {{ total }}</span>
      <button
        type="button"
        class="btn-mini"
        :disabled="offset + limit >= total"
        @click="offset += limit; load()"
      >
        {{ t('common.next') }}
      </button>
    </div>

    <!-- Dialog adjust -->
    <div
      v-if="adjusting"
      class="modal-backdrop"
      @click.self="adjusting = null"
    >
      <div
        class="modal"
        role="dialog"
        :aria-label="t('inv.adjust')"
      >
        <h3 class="panel-title">
          {{ t('inv.adjust_for', { sku: adjusting.sku }) }}
        </h3>
        <p class="muted">
          {{ t('inv.adjust_hint') }}
        </p>
        <label class="field">
          <span>{{ t('inv.col_qty_delta') }} *</span>
          <input
            v-model.number="adjDelta"
            type="number"
            class="input"
          >
        </label>
        <label class="field">
          <span>{{ t('inv.col_reason') }}</span>
          <input
            v-model="adjReason"
            type="text"
            class="input"
          >
        </label>
        <div class="row">
          <button
            type="button"
            class="btn-primary"
            :disabled="adjBusy || !adjDelta"
            @click="submitAdjust"
          >
            {{ t('common.save') }}
          </button>
          <button
            type="button"
            class="btn-mini"
            @click="adjusting = null"
          >
            {{ t('common.cancel') }}
          </button>
        </div>
      </div>
    </div>
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
.page-title { font-size: var(--text-xl); margin-bottom: var(--space-1); }
.page-subtitle { margin: 0; color: var(--color-ink-secondary); }

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-3);
}
.alert.error { border: 1px solid #fecdd3; background: #fff1f2; color: #9f1239; }
.alert.ok { border: 1px solid #bbf7d0; background: #f0fdf4; color: #14532d; }
.alert.warn { border: 1px solid #fde68a; background: #fffbeb; color: #78350f; }
.low-list { margin: var(--space-1) 0 0; padding-left: var(--space-5); }

.toolbar {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  margin-bottom: var(--space-4);
}
.search, .filter {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}
.search { flex: 1; min-width: 200px; }

.btn-secondary, .btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-ink);
  font: inherit;
}

.table-wrap { padding: var(--space-2) 0; }
.state { padding: var(--space-6) 0; text-align: center; }
.muted { color: var(--color-ink-muted); }

table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.02em; color: var(--color-ink-muted); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
td.low { color: #9f1239; font-weight: 700; }

.pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  margin-top: var(--space-3);
  font-size: var(--text-sm);
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 60;
}
.modal {
  width: 380px;
  max-width: 92vw;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-2); }
.field { display: flex; flex-direction: column; gap: var(--space-1); margin-bottom: var(--space-3); font-size: var(--text-sm); }
.input { padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); font: inherit; background: var(--color-surface); color: var(--color-ink); }
.row { display: flex; gap: var(--space-2); }
</style>
