<script setup lang="ts">
// Gudang (TASK-038): daftar gudang, ringkasan stok, dan form transfer stok
// antar gudang (atomik — satu varian gagal = seluruh transfer ditolak).
import { ApiRequestError } from '~/composables/useApi'
import type { Warehouse } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { list: listWarehouses, create: createWarehouse } = useWarehouses()
const { listStock, transfer, lowStock } = useInventory()

useHead({ title: () => t('inv.warehouses_title') })

const warehouses = ref<Warehouse[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)

// Form gudang baru.
const showCreate = ref(false)
const newName = ref('')
const creating = ref(false)

// Transfer.
const trFrom = ref('')
const trTo = ref('')
const trNotes = ref('')
const trVariantID = ref('')
const trQty = ref<number | null>(null)
const trBusy = ref(false)
const stock = ref<{ variant_id: string, sku: string, product_name: string, qty_on_hand: number, qty_reserved: number }[]>([])
const lowCount = ref(0)

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
    warehouses.value = await listWarehouses()
    if (!trFrom.value && warehouses.value.length > 0) trFrom.value = warehouses.value[0]?.id ?? ''
    if (!trTo.value && warehouses.value.length > 1) trTo.value = warehouses.value[1]?.id ?? ''
    await loadStock()
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

async function loadStock() {
  if (!trFrom.value) {
    stock.value = []
    return
  }
  try {
    const res = await listStock({ warehouse_id: trFrom.value, limit: 100 })
    stock.value = res.items
    lowCount.value = (await lowStock(trFrom.value)).length
  } catch (e) {
    error.value = errText(e)
  }
}

async function submitWarehouse() {
  if (!newName.value.trim()) return
  creating.value = true
  error.value = null
  try {
    await createWarehouse({ name: newName.value })
    message.value = t('inv.warehouse_created')
    newName.value = ''
    showCreate.value = false
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    creating.value = false
  }
}

async function submitTransfer() {
  if (!trFrom.value || !trTo.value || !trVariantID.value || !trQty.value) return
  trBusy.value = true
  error.value = null
  message.value = null
  try {
    await transfer({
      from: trFrom.value,
      to: trTo.value,
      notes: trNotes.value || undefined,
      items: [{ variant_id: trVariantID.value, qty: trQty.value }],
    })
    message.value = t('inv.transfer_done')
    trQty.value = null
    trNotes.value = ''
    await loadStock()
  } catch (e) {
    error.value = errText(e)
  } finally {
    trBusy.value = false
  }
}

const warehouseName = (id: string): string =>
  warehouses.value.find(w => w.id === id)?.name ?? id.slice(0, 8)

onMounted(load)
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('inv.warehouses_title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('inv.warehouses_subtitle') }}
        </p>
      </div>
      <button
        type="button"
        class="btn-primary"
        @click="showCreate = !showCreate"
      >
        {{ t('inv.add_warehouse') }}
      </button>
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
      v-if="showCreate"
      class="card panel"
    >
      <h2 class="panel-title">
        {{ t('inv.add_warehouse') }}
      </h2>
      <label class="field">
        <span>{{ t('inv.col_name') }} *</span>
        <input
          v-model="newName"
          type="text"
          class="input"
        >
      </label>
      <p class="muted hint">
        {{ t('inv.warehouse_address_hint') }}
      </p>
      <button
        type="button"
        class="btn-primary"
        :disabled="creating || !newName.trim()"
        @click="submitWarehouse"
      >
        {{ t('common.save') }}
      </button>
    </section>

    <section class="card panel">
      <h2 class="panel-title">
        {{ t('inv.transfer_title') }}
      </h2>
      <p
        v-if="loading"
        class="muted"
      >
        {{ t('common.loading') }}
      </p>
      <template v-else-if="warehouses.length >= 2">
        <div class="form-grid">
          <label class="field">
            <span>{{ t('inv.from_warehouse') }}</span>
            <select
              v-model="trFrom"
              class="input"
              @change="loadStock"
            >
              <option
                v-for="w in warehouses"
                :key="w.id"
                :value="w.id"
              >
                {{ w.name }}
              </option>
            </select>
          </label>
          <label class="field">
            <span>{{ t('inv.to_warehouse') }}</span>
            <select
              v-model="trTo"
              class="input"
            >
              <option
                v-for="w in warehouses.filter(x => x.id !== trFrom)"
                :key="w.id"
                :value="w.id"
              >
                {{ w.name }}
              </option>
            </select>
          </label>
          <label class="field">
            <span>{{ t('inv.col_variant') }}</span>
            <select
              v-model="trVariantID"
              class="input"
            >
              <option value="">
                {{ t('inv.select_variant') }}
              </option>
              <option
                v-for="s in stock"
                :key="s.variant_id"
                :value="s.variant_id"
              >
                {{ s.product_name }} ({{ s.sku }}) — {{ t('inv.available') }} {{ s.qty_on_hand - s.qty_reserved }}
              </option>
            </select>
          </label>
          <label class="field">
            <span>{{ t('inv.col_qty') }}</span>
            <input
              v-model.number="trQty"
              type="number"
              min="1"
              class="input"
            >
          </label>
        </div>
        <label class="field">
          <span>{{ t('inv.col_notes') }}</span>
          <input
            v-model="trNotes"
            type="text"
            class="input"
          >
        </label>
        <button
          type="button"
          class="btn-primary"
          :disabled="trBusy || !trFrom || !trTo || !trVariantID || !trQty || trFrom === trTo"
          @click="submitTransfer"
        >
          {{ trBusy ? t('common.loading') : t('inv.transfer_do') }}
        </button>
        <p
          v-if="trFrom === trTo"
          class="muted hint"
        >
          {{ t('inv.transfer_same_wh') }}
        </p>
      </template>
      <p
        v-else
        class="muted"
      >
        {{ t('inv.transfer_need_two_wh') }}
      </p>
    </section>

    <section class="card table-wrap">
      <h2 class="panel-title pad">
        {{ t('inv.stock_summary_title', { warehouse: warehouseName(trFrom) }) }}
        <span
          v-if="lowCount > 0"
          class="low-badge"
        >
          {{ lowCount }} {{ t('inv.low_stock_short') }}
        </span>
      </h2>
      <p
        v-if="stock.length === 0"
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
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="s in stock"
            :key="s.variant_id"
          >
            <td>{{ s.product_name }}</td>
            <td>{{ s.sku }}</td>
            <td class="num">{{ s.qty_on_hand }}</td>
            <td class="num">{{ s.qty_reserved }}</td>
            <td class="num">{{ s.qty_on_hand - s.qty_reserved }}</td>
          </tr>
        </tbody>
      </table>
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
.page-title { font-size: var(--text-xl); margin-bottom: var(--space-1); }
.page-subtitle { margin: 0; color: var(--color-ink-secondary); }

.alert { padding: var(--space-3); border-radius: var(--radius-md); margin-bottom: var(--space-3); }
.alert.error { border: 1px solid #fecdd3; background: #fff1f2; color: #9f1239; }
.alert.ok { border: 1px solid #bbf7d0; background: #f0fdf4; color: #14532d; }

.panel { padding: var(--space-4); margin-bottom: var(--space-4); }
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-2); display: flex; align-items: center; gap: var(--space-2); }
.pad { padding-bottom: var(--space-2); }

.low-badge {
  padding: 2px var(--space-2);
  border-radius: 999px;
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #78350f;
  font-size: var(--text-xs);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}
.field { display: flex; flex-direction: column; gap: var(--space-1); margin-bottom: var(--space-3); font-size: var(--text-sm); }
.input {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

.btn-primary {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
  font: inherit;
  background: var(--color-accent, #0f766e);
  color: #fff;
}
.btn-primary:disabled { opacity: 0.55; cursor: not-allowed; }

.muted { color: var(--color-ink-muted); font-size: var(--text-sm); }
.hint { margin-top: var(--space-2); }

.table-wrap { padding: var(--space-2) 0; }
.state { padding: var(--space-6) 0; text-align: center; }

table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.02em; color: var(--color-ink-muted); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
</style>
