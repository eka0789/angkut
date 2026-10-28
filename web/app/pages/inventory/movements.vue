<script setup lang="ts">
// Riwayat pergerakan stok (movement append-only) dengan filter jenis.
import { ApiRequestError } from '~/composables/useApi'
import type { InventoryMovement, InventoryMovementType } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { movements } = useInventory()

useHead({ title: () => t('inv.movements_title') })

const rows = ref<InventoryMovement[]>([])
const total = ref(0)
const limit = 50
const offset = ref(0)
const loading = ref(true)
const error = ref<string | null>(null)
const typeFilter = ref<'' | InventoryMovementType>('')

const typeOptions: Array<{ value: '' | InventoryMovementType, label: string }> = [
  { value: '', label: t('inv.type_all') },
  { value: 'adjust', label: t('inv.type_adjust') },
  { value: 'transfer_out', label: t('inv.type_transfer_out') },
  { value: 'transfer_in', label: t('inv.type_transfer_in') },
  { value: 'reserve', label: t('inv.type_reserve') },
  { value: 'release', label: t('inv.type_release') },
]

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) return e.message
  return t('common.error')
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const res = await movements({
      type: typeFilter.value || undefined,
      limit,
      offset: offset.value,
    })
    rows.value = res.items
    total.value = res.meta.total
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

function onTypeChange() {
  offset.value = 0
  void load()
}

const typeLabel = (m: InventoryMovement): string =>
  typeOptions.find(o => o.value === m.type)?.label ?? m.type

onMounted(load)
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('inv.movements_title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('inv.movements_subtitle') }}
        </p>
      </div>
      <NuxtLink
        to="/inventory"
        class="btn-secondary"
      >
        {{ t('inv.back_to_stock') }}
      </NuxtLink>
    </header>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>

    <section class="card toolbar">
      <select
        v-model="typeFilter"
        class="filter"
        :aria-label="t('inv.col_type')"
        @change="onTypeChange"
      >
        <option
          v-for="o in typeOptions"
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
        v-else-if="rows.length === 0"
        class="state muted"
      >
        {{ t('inv.empty_movements') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">
              {{ t('inv.col_time') }}
            </th>
            <th scope="col">
              {{ t('inv.col_type') }}
            </th>
            <th scope="col" class="num">
              {{ t('inv.col_qty_delta') }}
            </th>
            <th scope="col" class="num">
              {{ t('inv.col_qty_after') }}
            </th>
            <th scope="col">
              {{ t('inv.col_reason') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="m in rows"
            :key="m.id"
          >
            <td>{{ new Date(m.created_at).toLocaleString('id-ID') }}</td>
            <td>
              <span class="type-chip" :class="`t-${m.type}`">{{ typeLabel(m) }}</span>
            </td>
            <td class="num" :class="m.qty_delta < 0 ? 'neg' : 'pos'">
              {{ m.qty_delta > 0 ? '+' : '' }}{{ m.qty_delta }}
            </td>
            <td class="num">
              {{ m.qty_after }}
            </td>
            <td>{{ m.reason || '—' }}</td>
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

.toolbar { display: flex; gap: var(--space-3); padding: var(--space-3) var(--space-4); margin-bottom: var(--space-4); }
.filter {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

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
.btn-mini:disabled { opacity: 0.55; cursor: not-allowed; }

.table-wrap { padding: var(--space-2) 0; }
.state { padding: var(--space-6) 0; text-align: center; }
.muted { color: var(--color-ink-muted); }

table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.02em; color: var(--color-ink-muted); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
.pos { color: #14532d; font-weight: 600; }
.neg { color: #9f1239; font-weight: 600; }

.type-chip {
  display: inline-block;
  padding: 1px var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: 999px;
  font-size: var(--text-xs);
}
.t-transfer_in { border-color: #bbf7d0; background: #f0fdf4; color: #14532d; }
.t-transfer_out { border-color: #fecdd3; background: #fff1f2; color: #9f1239; }

.pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  margin-top: var(--space-3);
  font-size: var(--text-sm);
}
</style>
