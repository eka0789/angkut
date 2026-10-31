<script setup lang="ts">
// Keuangan (TASK-041..047): ringkasan COD, settlement, ledger, laporan
// net logistics cost, dan retur. Halaman tunggal bertab agar finance cukup
// satu pintu; aksi mutasi hanya muncul bila role berwenang (server tetap
// otoritatif — 403 ditampilkan sebagai pesan).
import { ApiRequestError } from '~/composables/useApi'
import type { CODSettlement, CODTransaction, LedgerEntry, ProductReturn } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const money = useMoney()

useHead({ title: () => t('money.title') })

const tab = ref<'cod' | 'settlements' | 'returns'>('cod')
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)

const summary = ref<Awaited<ReturnType<typeof money.summary>> | null>(null)
const financial = ref<Awaited<ReturnType<typeof money.financial>> | null>(null)
const txns = ref<CODTransaction[]>([])
const settlements = ref<CODSettlement[]>([])
const returns = ref<ProductReturn[]>([])
const ledger = ref<LedgerEntry[]>([])
const busy = ref<string | null>(null)

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) {
    if (e.status === 403) return t('money.forbidden')
    const fields = e.fields ? Object.values(e.fields) : []
    return [e.message, ...fields].filter(Boolean).join(' ')
  }
  return t('common.error')
}

const fmtIDR = (v: number): string => `Rp ${v.toLocaleString('id-ID')}`

async function load() {
  loading.value = true
  error.value = null
  try {
    const [s, f, tx, st, ret, led] = await Promise.all([
      money.summary(), money.financial(), money.transactions(),
      money.settlements(), money.returnsList(), money.ledger(),
    ])
    summary.value = s
    financial.value = f
    txns.value = tx
    settlements.value = st
    returns.value = ret
    ledger.value = led
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

async function markPaid(st: CODSettlement) {
  busy.value = st.id
  error.value = null
  try {
    await money.markPaid(st.id, `manual-${Date.now()}`, crypto.randomUUID())
    message.value = t('money.settlement_paid')
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = null
  }
}

async function onReturnAction(ret: ProductReturn, action: 'approve' | 'reject' | 'receive' | 'inspect') {
  busy.value = ret.id + action
  error.value = null
  try {
    if (action === 'inspect') {
      const items = (ret.items ?? []).map(it => ({ item_id: it.id, condition: 'damaged' }))
      await money.returnAction(ret.id, 'inspect', { restock: false, items })
    } else {
      await money.returnAction(ret.id, action, { note: '' })
    }
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = null
  }
}

async function refundReturn(ret: ProductReturn) {
  busy.value = ret.id + 'refund'
  error.value = null
  try {
    await money.returnAction(ret.id, 'refund', {
      amount: ret.refund_amount || 1, method: 'transfer',
    }, crypto.randomUUID())
    message.value = t('money.refunded')
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = null
  }
}

const statusLabelCOD: Record<string, string> = {
  pending: 'Menunggu', collected: 'Tercollect', settling: 'Menunggu transfer',
  settled: 'Lunas', failed: 'Gagal',
}
const statusLabelReturn: Record<string, string> = {
  requested: 'Diajukan', approved: 'Disetujui', rejected: 'Ditolak',
  received: 'Diterima', inspected: 'Diinspeksi', refunded: 'Direfund',
}

onMounted(load)
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('money.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('money.subtitle') }}
        </p>
      </div>
      <button
        type="button"
        class="btn-secondary"
        @click="money.exportSettlementsCSV()"
      >
        {{ t('money.export_csv') }}
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
      v-if="summary && financial"
      class="cards"
    >
      <div class="card stat">
        <span class="stat-label">{{ t('money.cod_expected') }}</span>
        <span class="stat-value">{{ fmtIDR(summary.expected_settlement) }}</span>
      </div>
      <div class="card stat">
        <span class="stat-label">{{ t('money.cod_settled') }}</span>
        <span class="stat-value">{{ fmtIDR(summary.settled) }}</span>
      </div>
      <div class="card stat">
        <span class="stat-label">{{ t('money.net_cost') }}</span>
        <span class="stat-value">{{ fmtIDR(financial.net_logistics_cost) }}</span>
      </div>
      <div class="card stat">
        <span class="stat-label">{{ t('money.refunds') }}</span>
        <span class="stat-value">{{ fmtIDR(financial.refunds) }}</span>
      </div>
    </section>

    <nav class="tabs">
      <button
        type="button"
        class="tab"
        :class="{ active: tab === 'cod' }"
        @click="tab = 'cod'"
      >
        {{ t('money.tab_cod') }}
      </button>
      <button
        type="button"
        class="tab"
        :class="{ active: tab === 'settlements' }"
        @click="tab = 'settlements'"
      >
        {{ t('money.tab_settlements') }}
      </button>
      <button
        type="button"
        class="tab"
        :class="{ active: tab === 'returns' }"
        @click="tab = 'returns'"
      >
        {{ t('money.tab_returns') }}
      </button>
    </nav>

    <section
      v-if="loading"
      class="card table-wrap"
    >
      <p class="state muted">
        {{ t('common.loading') }}
      </p>
    </section>

    <!-- Tab COD -->
    <section
      v-else-if="tab === 'cod'"
      class="card table-wrap"
    >
      <p
        v-if="txns.length === 0"
        class="state muted"
      >
        {{ t('money.empty_cod') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th>Shipment</th>
            <th>Kurir</th>
            <th class="num">Nilai</th>
            <th class="num">Fee</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="tx in txns"
            :key="tx.id"
          >
            <td class="mono">{{ tx.shipment_id.slice(0, 8) }}</td>
            <td>{{ tx.courier_code }}</td>
            <td class="num">{{ fmtIDR(tx.amount) }}</td>
            <td class="num">{{ fmtIDR(tx.fee) }}</td>
            <td>{{ statusLabelCOD[tx.status] ?? tx.status }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Tab settlement -->
    <section
      v-else-if="tab === 'settlements'"
      class="card table-wrap"
    >
      <p
        v-if="settlements.length === 0"
        class="state muted"
      >
        {{ t('money.empty_settlements') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th>Kurir</th>
            <th>Periode</th>
            <th class="num">Net</th>
            <th>Status</th>
            <th>{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="st in settlements"
            :key="st.id"
          >
            <td>{{ st.courier_code }}</td>
            <td>{{ st.period_start }} → {{ st.period_end }}</td>
            <td class="num">{{ fmtIDR(st.net_amount) }}</td>
            <td>{{ st.status === 'paid' ? 'Lunas' : 'Menunggu' }}</td>
            <td>
              <button
                v-if="st.status === 'pending'"
                type="button"
                class="btn-mini"
                :disabled="busy === st.id"
                @click="markPaid(st)"
              >
                {{ t('money.mark_paid') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Tab retur -->
    <section
      v-else
      class="card table-wrap"
    >
      <p
        v-if="returns.length === 0"
        class="state muted"
      >
        {{ t('money.empty_returns') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th>Tipe</th>
            <th>Alasan</th>
            <th class="num">Refund</th>
            <th>Status</th>
            <th>{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="ret in returns"
            :key="ret.id"
          >
            <td>{{ ret.type }}</td>
            <td>{{ ret.reason }}</td>
            <td class="num">{{ fmtIDR(ret.refund_amount) }}</td>
            <td>{{ statusLabelReturn[ret.status] ?? ret.status }}</td>
            <td class="actions">
              <button
                v-if="ret.status === 'requested'"
                type="button"
                class="btn-mini"
                :disabled="busy === ret.id + 'approve'"
                @click="onReturnAction(ret, 'approve')"
              >
                Setujui
              </button>
              <button
                v-if="ret.status === 'requested'"
                type="button"
                class="btn-mini"
                @click="onReturnAction(ret, 'reject')"
              >
                Tolak
              </button>
              <button
                v-if="ret.status === 'approved'"
                type="button"
                class="btn-mini"
                @click="onReturnAction(ret, 'receive')"
              >
                Terima
              </button>
              <button
                v-if="ret.status === 'received'"
                type="button"
                class="btn-mini"
                @click="onReturnAction(ret, 'inspect')"
              >
                Inspeksi
              </button>
              <button
                v-if="ret.status === 'inspected'"
                type="button"
                class="btn-mini"
                :disabled="busy === ret.id + 'refund'"
                @click="refundReturn(ret)"
              >
                Refund
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Ledger ringkas -->
    <section
      v-if="!loading && tab === 'cod' && ledger.length > 0"
      class="card table-wrap"
    >
      <h2 class="panel-title">
        {{ t('money.ledger_title') }}
      </h2>
      <table>
        <thead>
          <tr>
            <th>Tanggal</th>
            <th>Akun</th>
            <th class="num">Debit</th>
            <th class="num">Credit</th>
            <th>Keterangan</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="e in ledger.slice(0, 10)"
            :key="e.id"
          >
            <td>{{ e.entry_date }}</td>
            <td class="mono">{{ e.account }}</td>
            <td class="num">{{ e.debit ? fmtIDR(e.debit) : '—' }}</td>
            <td class="num">{{ e.credit ? fmtIDR(e.credit) : '—' }}</td>
            <td>{{ e.memo }}</td>
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

.cards {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}
@media (max-width: 900px) { .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .cards { grid-template-columns: 1fr; } }
.stat { padding: var(--space-4); display: flex; flex-direction: column; gap: var(--space-1); }
.stat-label { font-size: var(--text-xs); text-transform: uppercase; color: var(--color-ink-muted); letter-spacing: 0.04em; }
.stat-value { font-size: var(--text-lg); font-weight: 700; font-variant-numeric: tabular-nums; }

.tabs { display: flex; gap: var(--space-2); margin-bottom: var(--space-3); }
.tab {
  padding: var(--space-2) var(--space-4);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-weight: 600;
}
.tab.active { border-color: var(--color-accent, #0f766e); color: var(--color-accent, #0f766e); }

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

.table-wrap { padding: var(--space-2) 0; margin-bottom: var(--space-4); }
.state { padding: var(--space-6) 0; text-align: center; }
.muted { color: var(--color-ink-muted); }

table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
th, td { text-align: left; padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); }
th { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.02em; color: var(--color-ink-muted); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
.mono { font-family: ui-monospace, monospace; font-size: var(--text-xs); }
.actions { display: flex; gap: var(--space-1); flex-wrap: wrap; }
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-2); }
</style>
