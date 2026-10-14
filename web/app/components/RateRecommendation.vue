<script setup lang="ts">
// RateRecommendation — kartu opsi pemenang beserta alasan dan tingkat keyakinan.
import type { RateRecommendResponse } from '~/types/api'

const props = defineProps<{
  recommend: RateRecommendResponse | null
}>()

const { t } = useT()

function formatIDR(amount: number): string {
  return `Rp ${amount.toLocaleString('id-ID')}`
}

const option = computed(() => props.recommend?.recommended ?? null)

/** Keyakinan ditampilkan sebagai persen bulat — desimalnya tidak bermakna. */
const confidencePercent = computed(() =>
  option.value?.recommendation
    ? Math.round(option.value.recommendation.confidence * 100)
    : 0)
</script>

<template>
  <section
    v-if="option"
    class="rate-recommendation card"
  >
    <header class="head">
      <span class="badge">{{ t('rates.recommended') }}</span>
      <span
        v-if="confidencePercent > 0"
        class="muted confidence"
      >
        {{ t('rates.confidence', { percent: confidencePercent }) }}
      </span>
    </header>

    <p class="pick">
      <strong>{{ option.courier.name }}</strong>
      <span class="muted">— {{ option.service.name }}</span>
      <span class="mono price">{{ formatIDR(option.price) }}</span>
    </p>

    <ul
      v-if="option.recommendation?.reasons.length"
      class="reasons"
    >
      <li
        v-for="(reason, i) in option.recommendation.reasons"
        :key="i"
      >
        {{ reason }}
      </li>
    </ul>
  </section>
</template>

<style scoped>
.rate-recommendation {
  padding: var(--space-5);
  border-left: 4px solid var(--color-primary);
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}

.badge {
  padding: 2px var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  font-size: var(--text-xs);
  font-weight: 600;
}

.confidence {
  font-size: var(--text-sm);
}

.pick {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-2);
  margin: 0 0 var(--space-3);
}

.price {
  margin-left: auto;
  font-size: var(--text-md);
  font-weight: 600;
}

.reasons {
  margin: 0;
  padding-left: var(--space-5);
  color: var(--color-ink-secondary);
  font-size: var(--text-sm);
}

.reasons li {
  margin-bottom: var(--space-1);
}
</style>