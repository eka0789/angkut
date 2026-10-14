<script setup lang="ts">
// ShipmentStatusBadge — label status pengiriman dengan warna sesuai tahap.
import type { ShipmentLifecycleStatus } from '~/types/api'

const props = defineProps<{ status: ShipmentLifecycleStatus }>()

const { t } = useT()

const LABEL_KEY: Record<ShipmentLifecycleStatus, string> = {
  draft: 'shipment.status.draft',
  pending_pickup: 'shipment.status.pending_pickup',
  picked_up: 'shipment.status.picked_up',
  in_transit: 'shipment.status.in_transit',
  out_for_delivery: 'shipment.status.out_for_delivery',
  delivered: 'shipment.status.delivered',
  failed: 'shipment.status.failed',
  returned: 'shipment.status.returned',
  cancelled: 'shipment.status.cancelled',
}

const TONE: Record<ShipmentLifecycleStatus, string> = {
  draft: 'neutral',
  pending_pickup: 'info',
  picked_up: 'info',
  in_transit: 'progress',
  out_for_delivery: 'progress',
  delivered: 'ok',
  failed: 'danger',
  returned: 'warn',
  cancelled: 'danger',
}

const label = computed(() => LABEL_KEY[props.status] ? t(LABEL_KEY[props.status]) : t('shipment.status_unknown'))
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