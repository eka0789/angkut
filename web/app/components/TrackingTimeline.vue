<script setup lang="ts">
// TrackingTimeline — tampilan riwayat pelacakan.
//
// Satu komponen dipakai halaman tenant (/tracking), halaman publik
// (/track/[awb]), dan detail pengiriman. Dengan begitu "timeline" punya satu
// arti visual di seluruh aplikasi: urutan kejadian terlama → terbaru.
import type { TrackingTimeline } from '~/types/api'

const props = defineProps<{
  timeline: TrackingTimeline
  /** True saat aliran hidup tersambung (SSE/polling) — memunculkan indikator. */
  live?: boolean
}>()

const { t } = useT()

const reversed = computed(() => [...props.timeline.events].reverse())

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

function confidenceClass(level: string): string {
  if (level === 'on_track') return 'ok'
  if (level === 'delayed') return 'warn'
  return 'muted'
}
</script>

<template>
  <section class="timeline-card card">
    <header class="timeline-head">
      <div>
        <p class="awb mono">
          {{ timeline.awb }}
        </p>
        <p class="courier">
          {{ timeline.courier.code.toUpperCase() }} / {{ timeline.courier.service }}
          · {{ t('tracking.eta_range', { min: timeline.eta_min_days, max: timeline.eta_max_days }) }}
        </p>
      </div>
      <div class="head-right">
        <span
          v-if="live"
          class="live"
          role="status"
        >
          <span class="dot" />{{ t('tracking.live') }}
        </span>
        <ShipmentStatusBadge :status="timeline.status" />
      </div>
    </header>

    <p
      class="confidence"
      :class="confidenceClass(timeline.confidence.level)"
      role="status"
    >
      {{ timeline.confidence.message }}
      <span
        v-if="timeline.confidence.reason"
        class="reason"
      >— {{ timeline.confidence.reason }}</span>
    </p>

    <p
      v-if="timeline.last_location"
      class="last-location"
    >
      {{ t('tracking.last_location') }}:
      <strong>{{ timeline.last_location }}</strong>
      <span v-if="timeline.last_occurred_at"> · {{ formatDateTime(timeline.last_occurred_at) }}</span>
    </p>

    <ol
      v-if="reversed.length"
      class="events"
    >
      <li
        v-for="(ev, i) in reversed"
        :key="ev.id || `${ev.status}-${ev.occurred_at}-${i}`"
        class="event"
        :class="{ latest: i === 0 }"
      >
        <span
          class="marker"
          aria-hidden="true"
        />
        <div class="event-body">
          <div class="event-top">
            <strong>{{ t(`tracking.status.${ev.status}`) }}</strong>
            <time class="mono">{{ formatDateTime(ev.occurred_at) }}</time>
          </div>
          <p
            v-if="ev.description"
            class="desc"
          >
            {{ ev.description }}
          </p>
          <p
            v-if="ev.location"
            class="loc"
          >
            {{ ev.location }}
          </p>
        </div>
      </li>
    </ol>
    <p
      v-else
      class="empty muted"
    >
      {{ t('tracking.no_events') }}
    </p>
  </section>
</template>

<style scoped>
.timeline-card {
  padding: var(--space-5);
}

.timeline-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-3);
  flex-wrap: wrap;
  margin-bottom: var(--space-3);
}

.awb {
  font-size: var(--text-lg);
  font-weight: 700;
  margin: 0;
}

.courier {
  margin: var(--space-1) 0 0;
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}

.head-right {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.live {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--text-xs);
  font-weight: 600;
  color: #14532d;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #16a34a;
  animation: pulse 1.6s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}

.confidence {
  margin: 0 0 var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.confidence.ok {
  background: #f0fdf4;
  color: #14532d;
}

.confidence.warn {
  background: #fef6e7;
  color: #92400e;
}

.confidence.muted {
  background: var(--color-surface-muted, #f6f7f9);
  color: var(--color-ink-secondary);
}

.reason {
  color: inherit;
  opacity: 0.85;
}

.last-location {
  margin: 0 0 var(--space-4);
  font-size: var(--text-sm);
  color: var(--color-ink-secondary);
}

.events {
  list-style: none;
  margin: 0;
  padding: 0;
  border-left: 2px solid var(--color-border);
  margin-left: var(--space-2);
}

.event {
  position: relative;
  padding: 0 0 var(--space-4) var(--space-5);
}

.event:last-child {
  padding-bottom: 0;
}

.marker {
  position: absolute;
  left: calc(-1 * var(--space-2) - 5px);
  top: 4px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--color-border-strong, #c3c8d2);
  border: 2px solid var(--color-surface, #fff);
}

.event.latest .marker {
  background: var(--color-primary);
}

.event-top {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.event-top time {
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.desc {
  margin: var(--space-1) 0 0;
  font-size: var(--text-sm);
}

.loc {
  margin: var(--space-1) 0 0;
  font-size: var(--text-sm);
  color: var(--color-ink-muted);
}

.empty {
  margin: 0;
}
</style>
