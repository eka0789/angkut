<script setup lang="ts">
// OrderStatusBadge — label status pesanan dengan warna sesuai tahap.
import type { OrderStatus } from '~/types/api'

const props = defineProps<{ status: OrderStatus }>()

const { t } = useT()

const LABEL_KEY: Record<OrderStatus, string> = {
  draft: 'order.status.draft',
  confirmed: 'order.status.confirmed',
  processing: 'order.status.processing',
  shipped: 'order.status.shipped',
  delivered: 'order.status.delivered',
  cancelled: 'order.status.cancelled',
  returned: 'order.status.returned',
}

/** Warna menurut makna tahap: maju (biru), selesai (hijau), batal (merah). */
const TONE: Record<OrderStatus, string> = {
  draft: 'neutral',
  confirmed: 'info',
  processing: 'info',
  shipped: 'progress',
  delivered: 'ok',
  cancelled: 'danger',
  returned: 'warn',
}

const label = computed(() => t(LABEL_KEY[props.status]))
const tone = computed(() => TONE[props.status] ?? 'neutral')
</script>

<template>
  <span
    class="badge"
    :class="tone"
  >{{ label }}</span>
</template>

<style scoped>
.badge {
  display: inline-block;
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  font-size: var(--text-xs);
  font-weight: 600;
  white-space: nowrap;
}

.badge.neutral {
  background: var(--color-surface-sunken);
  color: var(--color-ink-secondary);
}

.badge.info {
  background: #eff6ff;
  color: #1d4ed8;
}

.badge.progress {
  background: #fef3c7;
  color: #92400e;
}

.badge.ok {
  background: #f0fdf4;
  color: #14532d;
}

.badge.warn {
  background: #fff7ed;
  color: #9a3412;
}

.badge.danger {
  background: #fff1f2;
  color: #9f1239;
}
</style>