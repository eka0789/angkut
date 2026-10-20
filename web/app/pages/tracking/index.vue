<script setup lang="ts">
// /tracking — pelacakan tenant dengan pembaruan hidup (TASK-034).
//
// Alur: cari resi → snapshot REST (/tracking/{awb}) → buka EventSource ke
// /tracking/{awb}/stream. Snapshot datang lagi sebagai event pertama aliran,
// jadi tampilan tidak pernah kosong selama menunggu; setiap event
// `tracking_update` mengganti timeline dengan versi terbaru.
//
// Bila SSE gagal (proxy memutus, Redis mati), EventSource menyambung ulang
// sendiri. Kita hanya mematikan indikator "live" agar pengguna tahu data
// mungkin tertinggal — tidak ada fallback polling diam-diam.
import { ApiRequestError } from '~/composables/useApi'
import type { TrackingEventStatus, TrackingTimeline } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const route = useRoute()
const router = useRouter()
const { byAWB, streamURL, append } = useTracking()

useHead({ title: () => t('tracking.title') })

const awbInput = ref(String(route.query.awb ?? ''))
const timeline = ref<TrackingTimeline | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const live = ref(false)
const message = ref<string | null>(null)

// Form pencatatan manual (staf).
const showForm = ref(false)
const formStatus = ref<TrackingEventStatus>('in_transit')
const formLocation = ref('')
const formDescription = ref('')
const formBusy = ref(false)

const eventStatusOptions: TrackingEventStatus[] = [
  'pickup_requested', 'picked_up', 'sorting', 'in_transit',
  'out_for_delivery', 'delivered', 'failed', 'returned',
]

let source: EventSource | null = null

function closeStream() {
  if (source) {
    source.close()
    source = null
  }
  live.value = false
}

/** Membuka aliran SSE. Aman dipanggil ulang: koneksi lama ditutup dulu. */
function openStream(awb: string) {
  closeStream()
  const url = streamURL(awb)
  if (!url || typeof EventSource === 'undefined') return

  source = new EventSource(url)
  source.addEventListener('open', () => { live.value = true })
  source.addEventListener('tracking_update', (ev: MessageEvent) => {
    try {
      timeline.value = JSON.parse(ev.data) as TrackingTimeline
      live.value = true
    } catch {
      // Payload rusak bukan alasan memutus aliran; snapshot berikutnya benar.
    }
  })
  source.addEventListener('error', () => { live.value = false })
}

async function search() {
  const awb = awbInput.value.trim().toUpperCase()
  if (!awb) return
  router.replace({ query: { awb } })

  loading.value = true
  error.value = null
  message.value = null
  closeStream()
  try {
    timeline.value = await byAWB(awb)
    openStream(awb)
  } catch (err: unknown) {
    timeline.value = null
    error.value = err instanceof ApiRequestError && err.status === 404
      ? t('tracking.not_found')
      : err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function submitEvent() {
  if (!timeline.value) return
  formBusy.value = true
  error.value = null
  message.value = null
  try {
    const res = await append({
      shipment_id: timeline.value.shipment_id,
      status: formStatus.value,
      location: formLocation.value.trim(),
      description: formDescription.value.trim(),
    })
    message.value = res.duplicate ? t('tracking.duplicate') : t('tracking.recorded')
    formLocation.value = ''
    formDescription.value = ''
    showForm.value = false
    // Tarik ulang snapshot supaya status shipment ikut segar walau SSE mati.
    timeline.value = await byAWB(timeline.value.awb)
  } catch (err: unknown) {
    error.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    formBusy.value = false
  }
}

onMounted(() => {
  if (awbInput.value.trim()) search()
})

onBeforeUnmount(closeStream)
</script>

<template>
  <div class="tracking-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('tracking.title') }}
        </h1>
        <p class="page-sub">
          {{ t('tracking.subtitle') }}
        </p>
      </div>
      <button
        v-if="timeline"
        type="button"
        class="btn-secondary"
        @click="showForm = !showForm"
      >
        {{ t('tracking.manual_entry') }}
      </button>
    </header>

    <form
      class="search card"
      @submit.prevent="search"
    >
      <label
        class="sr-only"
        for="awb-search"
      >{{ t('tracking.awb') }}</label>
      <input
        id="awb-search"
        v-model="awbInput"
        type="text"
        class="input mono"
        :placeholder="t('tracking.awb_placeholder')"
        autocomplete="off"
        spellcheck="false"
      >
      <button
        type="submit"
        class="btn-primary"
        :disabled="loading || !awbInput.trim()"
      >
        {{ t('tracking.search') }}
      </button>
    </form>

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

    <form
      v-if="showForm && timeline"
      class="card form"
      @submit.prevent="submitEvent"
    >
      <h2 class="form-title">
        {{ t('tracking.manual_entry') }}
      </h2>
      <div class="form-grid">
        <label>
          <span>{{ t('tracking.field_status') }}</span>
          <select v-model="formStatus">
            <option
              v-for="s in eventStatusOptions"
              :key="s"
              :value="s"
            >
              {{ t(`tracking.status.${s}`) }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('tracking.field_location') }}</span>
          <input
            v-model="formLocation"
            type="text"
            placeholder="Kota / gudang"
          >
        </label>
        <label class="full">
          <span>{{ t('tracking.field_description') }}</span>
          <input
            v-model="formDescription"
            type="text"
          >
        </label>
      </div>
      <div class="form-actions">
        <button
          type="submit"
          class="btn-primary"
          :disabled="formBusy"
        >
          {{ t('tracking.save') }}
        </button>
        <button
          type="button"
          class="btn-secondary"
          @click="showForm = false"
        >
          {{ t('common.cancel') }}
        </button>
      </div>
    </form>

    <p
      v-if="loading"
      class="card state muted"
    >
      {{ t('common.loading') }}
    </p>

    <TrackingTimeline
      v-else-if="timeline"
      :timeline="timeline"
      :live="live"
    />
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
  margin-bottom: var(--space-5);
  flex-wrap: wrap;
}

.page-title {
  font-size: var(--text-xl);
  margin: 0;
}

.page-sub {
  margin: var(--space-1) 0 0;
  color: var(--color-ink-secondary);
}

.search {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3);
  margin-bottom: var(--space-4);
}

.input {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
}

.input:focus {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
}

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-3);
}

.alert.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.alert.ok {
  border: 1px solid #bbf7d0;
  background: #f0fdf4;
  color: #14532d;
}

.form {
  padding: var(--space-5);
  margin-bottom: var(--space-4);
}

.form-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-3);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}

.form-grid label {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--text-sm);
  color: var(--color-ink-muted);
}

.form-grid .full {
  grid-column: 1 / -1;
}

.form-grid input,
.form-grid select {
  padding: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-ink);
}

.form-actions {
  display: flex;
  gap: var(--space-2);
}

.state {
  padding: var(--space-6);
  text-align: center;
}

.btn-primary,
.btn-secondary {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: 0;
}

.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
