<script setup lang="ts">
// /track/[awb] — halaman pelacakan publik (TASK-035).
//
// Tanpa middleware auth: tautan dibagikan ke penerima paket yang tidak punya
// akun. Endpoint backend-nya (`GET /api/v1/public/track/{awb}`) juga publik
// dan rate-limited per IP, sehingga enumerasi resi tidak murah.
import { ApiRequestError } from '~/composables/useApi'
import type { TrackingTimeline } from '~/types/api'

definePageMeta({ layout: 'public' })

const { t } = useT()
const route = useRoute()
const { publicByAWB } = useTracking()

const timeline = ref<TrackingTimeline | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)

const awb = computed(() => String(route.params.awb ?? '').trim().toUpperCase())

useHead({ title: () => (awb.value ? `Lacak ${awb.value}` : t('tracking.public_title')) })

async function load() {
  if (!awb.value) {
    loading.value = false
    return
  }
  loading.value = true
  error.value = null
  try {
    timeline.value = await publicByAWB(awb.value)
  } catch (err: unknown) {
    // 404 = resi tidak ada. Pesan khusus supaya pengguna tidak menebak-nebak
    // apakah sistemnya rusak atau resinya salah ketik.
    if (err instanceof ApiRequestError && err.status === 404) {
      error.value = t('tracking.not_found')
    } else {
      error.value = err instanceof ApiRequestError ? err.message : t('common.error')
    }
    timeline.value = null
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => route.params.awb, load)
</script>

<template>
  <div class="track-page">
    <NuxtLink
      to="/track"
      class="back"
    >
      ← {{ t('tracking.public_title') }}
    </NuxtLink>

    <p
      v-if="loading"
      class="card state muted"
    >
      {{ t('common.loading') }}
    </p>

    <div
      v-else-if="error"
      class="card state error"
      role="alert"
    >
      <p class="error-title">
        {{ error }}
      </p>
      <NuxtLink
        to="/track"
        class="btn btn-secondary"
      >
        {{ t('tracking.search') }}
      </NuxtLink>
    </div>

    <TrackingTimeline
      v-else-if="timeline"
      :timeline="timeline"
    />
  </div>
</template>

<style scoped>
.track-page {
  max-width: 760px;
  margin: 0 auto;
  padding: var(--space-8) var(--space-4);
}

.back {
  display: inline-block;
  margin-bottom: var(--space-4);
  font-size: var(--text-sm);
}

.state {
  padding: var(--space-6);
  text-align: center;
}

.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
}

.error-title {
  margin: 0 0 var(--space-4);
  color: #9f1239;
  font-weight: 600;
}

.btn {
  display: inline-block;
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  font-weight: 600;
}

.btn-secondary {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-ink);
}
</style>
