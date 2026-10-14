<script setup lang="ts">
// Form buat pesanan: pilih pelanggan, alamat kirim, tipe pembayaran, dan item.
// Total dihitung SERVER dari item — halaman ini hanya menampilkan pratinjau
// agar pengguna tidak melihat angka yang berbeda saat tersimpan.
import { ApiRequestError } from '~/composables/useApi'
import type { Address, Customer, Order, OrderPaymentType } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { create } = useOrders()
const { list: listCustomers } = useCustomers()
const { list: listAddresses } = useAddresses()
const router = useRouter()

useHead({ title: () => t('order.add') })

const customers = ref<Customer[]>([])
const addresses = ref<Address[]>([])
const submitting = ref(false)
const error = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})

const form = reactive({
  customerId: '',
  shippingAddressId: '',
  paymentType: 'prepaid' as OrderPaymentType,
  shippingCost: 0,
  discount: 0,
  notes: '',
  items: [{ productName: '', qty: 1, unitPrice: 0 }] as Array<{ productName: string, qty: number, unitPrice: number }>,
})

onMounted(async () => {
  try {
    const [cs, addrs] = await Promise.all([
      listCustomers({ limit: 100 }),
      listAddresses(),
    ])
    customers.value = cs.items
    addresses.value = addrs
  } catch {
    error.value = t('common.error')
  }
})

/** Pratinjau total — server tetap menghitung ulang saat menyimpan. */
const previewSubtotal = computed(() =>
  form.items.reduce((sum, it) => sum + (it.unitPrice || 0) * (it.qty || 0), 0))

const previewTotal = computed(() => {
  const total = previewSubtotal.value - (form.discount || 0) + (form.shippingCost || 0)
  return total < 0 ? 0 : total
})

function addItem() {
  form.items.push({ productName: '', qty: 1, unitPrice: 0 })
}

function removeItem(index: number) {
  if (form.items.length <= 1) return
  form.items.splice(index, 1)
}

async function submit() {
  error.value = null
  fieldErrors.value = {}
  submitting.value = true
  try {
    const order: Order = await create({
      customer_id: form.customerId,
      shipping_address_id: form.shippingAddressId || undefined,
      payment_type: form.paymentType,
      shipping_cost: form.shippingCost,
      discount: form.discount,
      notes: form.notes,
      items: form.items.map(it => ({
        product_name: it.productName,
        qty: it.qty,
        unit_price: it.unitPrice,
      })),
    })
    router.push(`/orders/${order.id}`)
  } catch (err: unknown) {
    if (err instanceof ApiRequestError) {
      fieldErrors.value = err.fields ?? {}
      error.value = err.message
    } else {
      error.value = t('common.error')
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="order-new">
    <header class="page-head">
      <NuxtLink
        to="/orders"
        class="back"
      >
        ← {{ t('order.title') }}
      </NuxtLink>
      <h1 class="page-title">
        {{ t('order.add') }}
      </h1>
    </header>

    <form
      class="card form"
      @submit.prevent="submit"
    >
      <div
        v-if="error"
        class="alert error"
        role="alert"
      >
        {{ error }}
      </div>

      <div class="fields">
        <label>
          <span>{{ t('order.field_customer') }}</span>
          <select v-model="form.customerId">
            <option value="">
              {{ t('wizard.select_placeholder') }}
            </option>
            <option
              v-for="c in customers"
              :key="c.id"
              :value="c.id"
            >
              {{ c.name }} — {{ c.phone }}
            </option>
          </select>
          <small
            v-if="fieldErrors.customer_id"
            class="err"
          >{{ fieldErrors.customer_id }}</small>
        </label>
        <label>
          <span>{{ t('wizard.field_destination') }}</span>
          <select v-model="form.shippingAddressId">
            <option value="">
              {{ t('wizard.select_placeholder') }}
            </option>
            <option
              v-for="a in addresses"
              :key="a.id"
              :value="a.id"
            >
              {{ a.recipient }} — {{ a.street }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('order.field_payment') }}</span>
          <select v-model="form.paymentType">
            <option value="prepaid">
              {{ t('order.payment_prepaid') }}
            </option>
            <option value="cod">
              {{ t('order.payment_cod') }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('order.field_shipping_cost') }}</span>
          <input
            v-model.number="form.shippingCost"
            type="number"
            min="0"
            step="1000"
          >
        </label>
        <label>
          <span>{{ t('order.field_discount') }}</span>
          <input
            v-model.number="form.discount"
            type="number"
            min="0"
            step="1000"
          >
          <small
            v-if="fieldErrors.discount"
            class="err"
          >{{ fieldErrors.discount }}</small>
        </label>
        <label class="wide">
          <span>{{ t('order.field_notes') }}</span>
          <input
            v-model="form.notes"
            type="text"
          >
        </label>
      </div>

      <h2 class="section-title">
        {{ t('order.items') }}
      </h2>
      <small
        v-if="fieldErrors.items"
        class="err"
      >{{ fieldErrors.items }}</small>

      <div class="items">
        <div
          v-for="(it, i) in form.items"
          :key="i"
          class="item-row"
        >
          <label class="grow">
            <span>{{ t('order.item_product') }}</span>
            <input
              v-model="it.productName"
              type="text"
            >
          </label>
          <label>
            <span>{{ t('order.item_qty') }}</span>
            <input
              v-model.number="it.qty"
              type="number"
              min="1"
            >
          </label>
          <label>
            <span>{{ t('order.item_price') }}</span>
            <input
              v-model.number="it.unitPrice"
              type="number"
              min="0"
              step="1000"
            >
          </label>
          <button
            type="button"
            class="btn-mini danger"
            :disabled="form.items.length <= 1"
            @click="removeItem(i)"
          >
            {{ t('order.remove_item') }}
          </button>
        </div>
      </div>

      <button
        type="button"
        class="btn-secondary"
        @click="addItem"
      >
        {{ t('order.add_item') }}
      </button>

      <dl class="totals">
        <div>
          <dt>{{ t('order.subtotal') }}</dt>
          <dd class="mono">
            Rp {{ previewSubtotal.toLocaleString('id-ID') }}
          </dd>
        </div>
        <div>
          <dt>{{ t('order.shipping_cost') }}</dt>
          <dd class="mono">
            Rp {{ (form.shippingCost || 0).toLocaleString('id-ID') }}
          </dd>
        </div>
        <div v-if="form.discount > 0">
          <dt>{{ t('order.discount') }}</dt>
          <dd class="mono">
            −Rp {{ form.discount.toLocaleString('id-ID') }}
          </dd>
        </div>
        <div class="grand">
          <dt>{{ t('order.total') }}</dt>
          <dd class="mono">
            Rp {{ previewTotal.toLocaleString('id-ID') }}
          </dd>
        </div>
      </dl>

      <div class="actions">
        <button
          type="submit"
          class="btn-primary"
          :disabled="submitting"
        >
          {{ submitting ? t('common.loading') : t('order.save') }}
        </button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: var(--space-5);
}

.back {
  font-size: var(--text-sm);
}

.page-title {
  font-size: var(--text-xl);
  margin: var(--space-1) 0 0;
}

.form {
  padding: var(--space-5);
}

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-4);
}

.alert.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
}

.fields label,
.item-row label {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--text-sm);
  font-weight: 600;
}

.fields label.wide {
  grid-column: 1 / -1;
}

input,
select {
  font: inherit;
  font-weight: 400;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-ink);
}

.section-title {
  font-size: var(--text-md);
  margin: var(--space-5) 0 var(--space-3);
}

.items {
  display: grid;
  gap: var(--space-3);
}

.item-row {
  display: grid;
  grid-template-columns: 2fr 0.7fr 1.2fr auto;
  gap: var(--space-2);
  align-items: end;
}

.err {
  color: #9f1239;
  font-weight: 400;
}

.totals {
  margin: var(--space-5) 0 0;
  display: grid;
  gap: var(--space-2);
  max-width: 320px;
  margin-left: auto;
}

.totals > div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
}

.totals dt {
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
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

.actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
}

.btn-primary,
.btn-secondary,
.btn-mini {
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: 0;
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-secondary,
.btn-mini {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}

.btn-mini {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-xs);
}

.btn-mini.danger {
  border-color: #fecdd3;
  color: #9f1239;
}

.btn-mini:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

@media (max-width: 700px) {
  .item-row {
    grid-template-columns: 1fr 1fr;
  }
}
</style>