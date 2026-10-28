<script setup lang="ts">
// Daftar produk & varian (TASK-037): pencarian, filter low-stock, buat produk,
// tambah varian SKU. Mutasi hanya warehouse+ — 403 ditampilkan sebagai pesan.
import { ApiRequestError } from '~/composables/useApi'
import type { Product } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { listProducts, createProduct, addVariant } = useInventory()

useHead({ title: () => t('inv.products_title') })

const items = ref<Product[]>([])
const total = ref(0)
const limit = 20
const offset = ref(0)
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)
const query = ref('')
const lowStockOnly = ref(false)

// Form buat produk.
const showCreate = ref(false)
const creating = ref(false)
const newName = ref('')
const newCategory = ref('')
const newWeight = ref<number | null>(null)

// Form tambah varian.
const variantFor = ref<Product | null>(null)
const variantSku = ref('')
const variantThreshold = ref(5)
const addingVariant = ref(false)

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
    const res = await listProducts({
      search: query.value || undefined,
      low_stock: lowStockOnly.value,
      limit,
      offset: offset.value,
    })
    items.value = res.items
    total.value = res.meta.total
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

async function submitProduct() {
  if (!newName.value.trim()) return
  creating.value = true
  error.value = null
  try {
    await createProduct({
      name: newName.value,
      category: newCategory.value || undefined,
      weight_kg: newWeight.value,
    })
    message.value = t('inv.product_created')
    showCreate.value = false
    newName.value = ''
    newCategory.value = ''
    newWeight.value = null
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    creating.value = false
  }
}

async function submitVariant() {
  if (!variantFor.value || !variantSku.value.trim()) return
  addingVariant.value = true
  error.value = null
  try {
    await addVariant(variantFor.value.id, {
      sku: variantSku.value,
      low_stock_threshold: variantThreshold.value,
    })
    message.value = t('inv.variant_created')
    variantFor.value = null
    variantSku.value = ''
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    addingVariant.value = false
  }
}

function toggleLowStock() {
  lowStockOnly.value = !lowStockOnly.value
  offset.value = 0
  void load()
}

onMounted(load)
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('inv.products_title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('inv.products_subtitle') }}
        </p>
      </div>
      <button
        type="button"
        class="btn-primary"
        @click="showCreate = !showCreate"
      >
        {{ t('inv.add_product') }}
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
        {{ t('inv.add_product') }}
      </h2>
      <div class="form-grid">
        <label class="field">
          <span>{{ t('inv.col_name') }} *</span>
          <input
            v-model="newName"
            type="text"
            class="input"
          >
        </label>
        <label class="field">
          <span>{{ t('inv.col_category') }}</span>
          <input
            v-model="newCategory"
            type="text"
            class="input"
          >
        </label>
        <label class="field">
          <span>{{ t('inv.col_weight') }}</span>
          <input
            v-model.number="newWeight"
            type="number"
            min="0"
            step="0.001"
            class="input"
          >
        </label>
      </div>
      <button
        type="button"
        class="btn-primary"
        :disabled="creating || !newName.trim()"
        @click="submitProduct"
      >
        {{ creating ? t('common.loading') : t('common.save') }}
      </button>
    </section>

    <section class="card toolbar">
      <input
        v-model="query"
        type="search"
        class="search"
        :placeholder="t('inv.search_placeholder')"
        @keydown.enter="load(); offset = 0"
      >
      <button
        type="button"
        class="btn-mini"
        :class="{ active: lowStockOnly }"
        @click="toggleLowStock"
      >
        {{ t('inv.low_stock_only') }}
      </button>
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
        {{ t('inv.empty_products') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">
              {{ t('inv.col_name') }}
            </th>
            <th scope="col">
              {{ t('inv.col_category') }}
            </th>
            <th scope="col">
              {{ t('inv.col_variants') }}
            </th>
            <th scope="col">
              {{ t('common.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="p in items"
            :key="p.id"
          >
            <td>{{ p.name }}</td>
            <td>{{ p.category || '—' }}</td>
            <td>
              <span
                v-for="v in p.variants ?? []"
                :key="v.id"
                class="sku-chip"
              >
                {{ v.sku }}
              </span>
              <span
                v-if="!p.variants?.length"
                class="muted"
              >
                {{ t('inv.no_variants') }}
              </span>
            </td>
            <td>
              <button
                type="button"
                class="btn-mini"
                @click="variantFor = p; variantSku = ''"
              >
                {{ t('inv.add_variant') }}
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

    <!-- Dialog tambah varian -->
    <div
      v-if="variantFor"
      class="modal-backdrop"
      @click.self="variantFor = null"
    >
      <div
        class="modal"
        role="dialog"
        :aria-label="t('inv.add_variant')"
      >
        <h3 class="panel-title">
          {{ t('inv.add_variant_for', { product: variantFor.name }) }}
        </h3>
        <label class="field">
          <span>SKU *</span>
          <input
            v-model="variantSku"
            type="text"
            class="input"
          >
        </label>
        <label class="field">
          <span>{{ t('inv.low_stock_threshold') }}</span>
          <input
            v-model.number="variantThreshold"
            type="number"
            min="0"
            class="input"
          >
        </label>
        <div class="row">
          <button
            type="button"
            class="btn-primary"
            :disabled="addingVariant || !variantSku.trim()"
            @click="submitVariant"
          >
            {{ t('common.save') }}
          </button>
          <button
            type="button"
            class="btn-mini"
            @click="variantFor = null"
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

.panel { padding: var(--space-4); margin-bottom: var(--space-4); }
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-2); }

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}
.field { display: flex; flex-direction: column; gap: var(--space-1); font-size: var(--text-sm); }

.input, .search {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}
.search { flex: 1; min-width: 200px; }

.toolbar {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  margin-bottom: var(--space-4);
  align-items: center;
}

.btn-primary, .btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
  font: inherit;
}
.btn-primary { background: var(--color-accent, #0f766e); color: #fff; }
.btn-mini { border-color: var(--color-border); background: var(--color-surface); color: var(--color-ink); }
.btn-mini.active { border-color: var(--color-accent, #0f766e); color: var(--color-accent, #0f766e); }
.btn-primary:disabled, .btn-mini:disabled { opacity: 0.55; cursor: not-allowed; }

.table-wrap { padding: var(--space-2) 0; }
.state { padding: var(--space-6) 0; text-align: center; }
.muted { color: var(--color-ink-muted); }

table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.02em; color: var(--color-ink-muted); }

.sku-chip {
  display: inline-block;
  padding: 1px var(--space-2);
  margin-right: var(--space-1);
  border: 1px solid var(--color-border);
  border-radius: 999px;
  font-size: var(--text-xs);
}

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
.row { display: flex; gap: var(--space-2); }
</style>
