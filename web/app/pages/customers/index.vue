<script setup lang="ts">
// Daftar pelanggan tenant: cari, filter segmen, buat, edit, hapus.
// Detail metrik ada di /customers/{id}; halaman ini fokus pada tabel.
import { ApiRequestError } from '~/composables/useApi'
import type { Customer, CustomerSegment } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { list, create, update, remove } = useCustomers()
const { session } = useAuth()

useHead({ title: () => t('nav.customers') })

const items = ref<Customer[]>([])
const total = ref(0)
const limit = 20
const offset = ref(0)
const loading = ref(true)
const loadError = ref<string | null>(null)
const query = ref('')
const segmentFilter = ref<'' | CustomerSegment>('')
const errorMessage = ref<string | null>(null)
const saveMessage = ref<string | null>(null)
const saving = ref(false)

// Form tambah/ubah — panel muncul saat editing/creating.
const showForm = ref(false)
const editing = ref<Customer | null>(null)
const form = ref({ name: '', phone: '', email: '', notes: '' })

// RBAC: hanya role yang punya customer.manage yang boleh menulis.
const canManage = computed(() => {
  const role = session.value?.active_role
  return role === 'owner' || role === 'admin' || role === 'staff'
})
const canDelete = computed(() => {
  const role = session.value?.active_role
  return role === 'owner' || role === 'admin'
})

const segments: Array<{ value: '' | CustomerSegment, label: string }> = [
  { value: '', label: t('customer.segment_all') },
  { value: 'new', label: t('customer.segment_new') },
  { value: 'regular', label: t('customer.segment_regular') },
  { value: 'vip', label: t('customer.segment_vip') },
  { value: 'high_risk', label: t('customer.segment_high_risk') },
  { value: 'cod_risk', label: t('customer.segment_cod_risk') },
]

function segmentLabel(s: CustomerSegment): string {
  const found = segments.find(x => x.value === s)
  return found ? found.label : s
}

function formatSpend(v: number): string {
  return `Rp ${v.toLocaleString('id-ID')}`
}

async function loadList() {
  loading.value = true
  loadError.value = null
  try {
    const res = await list({
      search: query.value.trim() || undefined,
      segment: segmentFilter.value || undefined,
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

// Debounce pencarian sederhana; filter segmen langsung.
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(query, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    offset.value = 0
    void loadList()
  }, 300)
})
watch(segmentFilter, () => {
  offset.value = 0
  void loadList()
})

function openCreate() {
  editing.value = null
  form.value = { name: '', phone: '', email: '', notes: '' }
  showForm.value = true
}

function openEdit(c: Customer) {
  editing.value = c
  form.value = {
    name: c.name,
    phone: c.phone,
    email: c.email ?? '',
    notes: c.notes ?? '',
  }
  showForm.value = true
}

async function save() {
  saving.value = true
  errorMessage.value = null
  saveMessage.value = null
  try {
    if (editing.value) {
      await update(editing.value.id, form.value)
      saveMessage.value = t('customer.saved')
    } else {
      await create(form.value)
      saveMessage.value = t('customer.created')
    }
    showForm.value = false
    await loadList()
  } catch (err: unknown) {
    if (err instanceof ApiRequestError) {
      errorMessage.value = err.fields ? Object.values(err.fields).join(' ') : err.message
    } else {
      errorMessage.value = t('common.error')
    }
  } finally {
    saving.value = false
  }
}

async function drop(c: Customer) {
  if (!confirm(t('customer.confirm_delete', { name: c.name }))) return
  errorMessage.value = null
  try {
    await remove(c.id)
    await loadList()
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

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
  <div class="customers">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('customer.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('customer.subtitle') }}
        </p>
      </div>
      <button
        v-if="canManage"
        type="button"
        class="btn-primary"
        @click="openCreate"
      >
        {{ t('customer.add') }}
      </button>
    </header>

    <div
      v-if="loadError"
      class="form-alert"
      role="alert"
    >
      {{ loadError }}
    </div>
    <div
      v-else-if="saveMessage"
      class="state-ok"
      role="status"
    >
      {{ saveMessage }}
    </div>

    <!-- Filter bar -->
    <section class="card toolbar">
      <input
        v-model="query"
        type="search"
        class="search"
        :placeholder="t('customer.search_placeholder')"
      >
      <select
        v-model="segmentFilter"
        class="segment-filter"
        :aria-label="t('customer.filter_segment')"
      >
        <option
          v-for="s in segments"
          :key="s.value"
          :value="s.value"
        >
          {{ s.label }}
        </option>
      </select>
    </section>

    <!-- Form tambah/ubah -->
    <section
      v-if="showForm"
      class="card section"
    >
      <h2 class="section-title">
        {{ editing ? t('customer.edit') : t('customer.add') }}
      </h2>
      <div
        v-if="errorMessage"
        class="form-alert"
        role="alert"
      >
        {{ errorMessage }}
      </div>
      <form
        class="form-grid"
        novalidate
        @submit.prevent="save"
      >
        <div class="field">
          <label for="cust-name">{{ t('customer.field_name') }}</label>
          <input
            id="cust-name"
            v-model="form.name"
            type="text"
            required
          >
        </div>
        <div class="field">
          <label for="cust-phone">{{ t('customer.field_phone') }}</label>
          <input
            id="cust-phone"
            v-model="form.phone"
            type="tel"
            required
          >
        </div>
        <div class="field">
          <label for="cust-email">{{ t('customer.field_email') }}</label>
          <input
            id="cust-email"
            v-model="form.email"
            type="email"
          >
        </div>
        <div class="field field-wide">
          <label for="cust-notes">{{ t('customer.field_notes') }}</label>
          <input
            id="cust-notes"
            v-model="form.notes"
            type="text"
          >
        </div>
        <div class="form-actions">
          <button
            type="button"
            class="btn-secondary"
            @click="showForm = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            class="btn-primary"
            :disabled="saving"
          >
            {{ saving ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </form>
    </section>

    <!-- Tabel -->
    <section class="card table-wrap">
      <div
        v-if="loading"
        class="state-block"
        role="status"
      >
        {{ t('common.loading') }}
      </div>

      <p
        v-else-if="items.length === 0"
        class="hint empty"
      >
        {{ query || segmentFilter ? t('customer.empty_filtered') : t('customer.empty') }}
      </p>

      <table
        v-else
        class="cust-table"
      >
        <thead>
          <tr>
            <th>{{ t('customer.field_name') }}</th>
            <th>{{ t('customer.field_phone') }}</th>
            <th>{{ t('customer.segment') }}</th>
            <th class="num">
              {{ t('customer.col_orders') }}
            </th>
            <th class="num">
              {{ t('customer.col_spend') }}
            </th>
            <th class="actions-col" />
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="c in items"
            :key="c.id"
          >
            <td>
              <NuxtLink
                :to="`/customers/${c.id}`"
                class="cust-name"
              >
                {{ c.name }}
              </NuxtLink>
              <span
                v-if="c.email"
                class="cust-email"
              >{{ c.email }}</span>
            </td>
            <td class="mono">
              {{ c.phone }}
            </td>
            <td>
              <span
                class="seg-badge"
                :class="`seg-${c.segment}`"
              >
                {{ segmentLabel(c.segment) }}
                <span
                  v-if="c.segment_source === 'manual'"
                  class="seg-src"
                >{{ t('customer.manual') }}</span>
              </span>
            </td>
            <td class="num">
              {{ c.metrics.orders_count }}
            </td>
            <td class="num">
              {{ formatSpend(c.metrics.total_spend) }}
            </td>
            <td class="actions-col">
              <button
                v-if="canManage"
                type="button"
                class="btn-ghost"
                @click="openEdit(c)"
              >
                {{ t('common.edit') }}
              </button>
              <button
                v-if="canDelete"
                type="button"
                class="btn-ghost btn-danger-ghost"
                @click="drop(c)"
              >
                {{ t('common.remove') }}
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
        <span class="pager-info">{{ offset + 1 }}–{{ Math.min(offset + limit, total) }} / {{ total }}</span>
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

.toolbar {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  margin-bottom: var(--space-4);
}

.search {
  flex: 1;
  min-width: 200px;
}

.search,
.segment-filter {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  background: var(--color-surface);
  color: var(--color-ink);
}

.section {
  padding: var(--space-5);
  margin-bottom: var(--space-4);
}

.section-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-4);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
}

.field-wide {
  grid-column: 1 / -1;
}

.field label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 600;
  margin-bottom: var(--space-1);
}

.field input {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  background: var(--color-surface);
  color: var(--color-ink);
}

.form-actions {
  grid-column: 1 / -1;
  display: flex;
  gap: var(--space-3);
  justify-content: flex-end;
}

.btn-primary {
  padding: var(--space-2) var(--space-5);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: none;
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  cursor: pointer;
}

.btn-primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-secondary {
  padding: var(--space-2) var(--space-4);
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  cursor: pointer;
}

.btn-secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.table-wrap {
  padding: var(--space-2) 0;
}

.table-wrap :deep(table) {
  width: 100%;
}

.cust-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.cust-table th {
  text-align: left;
  padding: var(--space-2) var(--space-4);
  font-size: var(--text-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-ink-muted);
  border-bottom: 1px solid var(--color-border);
}

.cust-table td {
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border);
  vertical-align: top;
}

.cust-table tr:last-child td {
  border-bottom: none;
}

.num {
  text-align: right;
}

.actions-col {
  text-align: right;
  white-space: nowrap;
}

.cust-name {
  display: block;
  font-weight: 600;
  color: var(--color-ink);
}

.cust-name:hover {
  color: var(--color-primary);
}

.cust-email {
  display: block;
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.seg-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  font-size: var(--text-xs);
  font-weight: 600;
}

.seg-new {
  background: var(--color-surface-sunken);
  color: var(--color-ink-secondary);
}

.seg-regular {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.seg-vip {
  background: #fff3d6;
  color: #8a6100;
}

.seg-high_risk,
.seg-cod_risk {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.seg-src {
  font-weight: 400;
  opacity: 0.7;
}

.btn-ghost {
  padding: var(--space-1) var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  color: var(--color-primary);
  cursor: pointer;
}

.btn-ghost:hover {
  background: var(--color-surface-sunken);
}

.btn-danger-ghost {
  color: var(--color-danger);
}

.pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
}

.pager-info {
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.form-alert {
  margin-bottom: var(--space-4);
  padding: var(--space-3) var(--space-4);
  background: var(--color-danger-soft);
  color: var(--color-danger);
  border: 1px solid currentColor;
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.state-ok {
  margin-bottom: var(--space-4);
  padding: var(--space-2) var(--space-3);
  background: var(--color-success-soft);
  color: var(--color-success);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.state-block {
  padding: var(--space-8);
  text-align: center;
  color: var(--color-ink-secondary);
}

.empty {
  padding: var(--space-8) var(--space-4);
  text-align: center;
}

.hint {
  font-size: var(--text-sm);
  color: var(--color-ink-muted);
}

@media (max-width: 720px) {
  .cust-table {
    font-size: var(--text-xs);
  }

  .cust-table th:nth-child(4),
  .cust-table td:nth-child(4) {
    display: none;
  }
}
</style>
