<script setup lang="ts">
// RatePreview — tabel perbandingan tarif: harga, ETA, keandalan, skor, label.
import type { RateLabel, RateOption, RateQuote, RateSummaryLabels } from '~/types/api'

const props = defineProps<{
  quote: RateQuote | null
  loading?: boolean
}>()

const { t } = useT()

/** Label → kunci terjemahan. */
const LABEL_KEY: Record<RateLabel, string> = {
  best_price: 'rates.best_price',
  fastest: 'rates.fastest',
  most_reliable: 'rates.most_reliable',
  best_value: 'rates.best_value',
}

function labelText(label: RateLabel): string {
  return t(LABEL_KEY[label])
}

function formatIDR(amount: number): string {
  return `Rp ${amount.toLocaleString('id-ID')}`
}

function formatEta(opt: RateOption): string {
  if (opt.eta_min_days <= 0 && opt.eta_max_days <= 0) return t('rates.eta_unknown')
  if (opt.eta_min_days === opt.eta_max_days) {
    return t('rates.eta_days_same', { min: opt.eta_min_days })
  }
  return t('rates.eta_days', { min: opt.eta_min_days, max: opt.eta_max_days })
}

function formatReliability(opt: RateOption): string {
  return `${opt.reliability_score.toFixed(1)}%`
}

/** Ringkasan label sebagai daftar berpasangan untuk ditampilkan. */
const summaryRows = computed<Array<{ label: RateLabel, winner: string }>>(() => {
  const s: RateSummaryLabels | undefined = props.quote?.summary_labels
  if (!s) return []
  const rows: Array<{ label: RateLabel, winner: string }> = [
    { label: 'best_price', winner: s.cheapest },
    { label: 'fastest', winner: s.fastest },
    { label: 'most_reliable', winner: s.most_reliable },
    { label: 'best_value', winner: s.best_value },
  ]
  return rows.filter(row => Boolean(row.winner))
})

const options = computed(() => props.quote?.options ?? [])
</script>

<template>
  <section class="rate-preview card">
    <header class="head">
      <h2 class="section-title">
        {{ t('rates.title') }}
      </h2>
      <p
        v-if="quote"
        class="muted chargeable"
      >
        {{ t('rates.chargeable') }}:
        <span class="mono">{{ quote.chargeable_weight }} kg</span>
      </p>
    </header>

    <p
      v-if="loading"
      class="state muted"
    >
      {{ t('rates.loading') }}
    </p>

    <p
      v-else-if="!quote"
      class="state muted"
    >
      {{ t('rates.empty') }}
    </p>

    <p
      v-else-if="options.length === 0"
      class="state muted"
    >
      {{ t('rates.no_options') }}
    </p>

    <template v-else>
      <div
        v-if="summaryRows.length"
        class="summary"
      >
        <span class="summary-label">{{ t('rates.summary') }}:</span>
        <span
          v-for="row in summaryRows"
          :key="row.label"
          class="chip"
        >
          <strong>{{ labelText(row.label) }}</strong>
          <span class="mono">{{ row.winner }}</span>
        </span>
      </div>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th scope="col">
                {{ t('rates.column_courier') }}
              </th>
              <th scope="col">
                {{ t('rates.column_service') }}
              </th>
              <th
                scope="col"
                class="num"
              >
                {{ t('rates.column_price') }}
              </th>
              <th scope="col">
                {{ t('rates.column_eta') }}
              </th>
              <th
                scope="col"
                class="num"
              >
                {{ t('rates.column_reliability') }}
              </th>
              <th
                scope="col"
                class="num"
              >
                {{ t('rates.column_score') }}
              </th>
              <th scope="col">
                Label
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="opt in options"
              :key="`${opt.courier.code}:${opt.service.code}`"
              :class="{ recommended: opt.is_recommended }"
            >
              <td>{{ opt.courier.name }}</td>
              <td>
                {{ opt.service.name }}
                <span class="muted code">({{ opt.service.code }})</span>
                <span
                  v-if="opt.is_recommended"
                  class="flag"
                >{{ t('rates.recommended') }}</span>
              </td>
              <td class="num mono">
                {{ formatIDR(opt.price) }}
              </td>
              <td>{{ formatEta(opt) }}</td>
              <td class="num mono">
                {{ formatReliability(opt) }}
              </td>
              <td class="num mono">
                {{ opt.score.toFixed(1) }}
              </td>
              <td>
                <span
                  v-for="label in opt.labels"
                  :key="label"
                  class="chip small"
                >
                  {{ labelText(label) }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>

<style scoped>
.rate-preview {
  padding: var(--space-5);
  margin-bottom: var(--space-4);
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.section-title {
  font-size: var(--text-md);
  margin: 0;
}

.chargeable {
  margin: 0;
  font-size: var(--text-sm);
}

.state {
  padding: var(--space-6) 0;
  text-align: center;
}

.summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin: var(--space-3) 0;
}

.summary-label {
  font-size: var(--text-sm);
  font-weight: 600;
}

.chip {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  padding: 2px var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-surface-subtle);
  font-size: var(--text-xs);
}

.chip.small {
  margin-right: var(--space-1);
}

.table-wrap {
  overflow-x: auto;
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

tr.recommended {
  background: var(--color-surface-subtle);
}

.code {
  font-size: var(--text-xs);
}

.flag {
  display: inline-block;
  margin-left: var(--space-2);
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  font-size: var(--text-xs);
  font-weight: 600;
}
</style>