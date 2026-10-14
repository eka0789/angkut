<script setup lang="ts">
// Halaman pembuatan pengiriman. Wizard memegang Idempotency-Key sendiri
// sehingga retry tidak menduplikasi; halaman ini hanya menampilkan hasil dan
// mengarahkan ke detail setelah berhasil.
import type { Shipment } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const router = useRouter()

useHead({ title: () => t('wizard.title') })

const created = ref<Shipment | null>(null)

function onCreated(sh: Shipment) {
  created.value = sh
}
</script>

<template>
  <div class="shipment-new">
    <header class="page-head">
      <div>
        <NuxtLink
          to="/shipments"
          class="back"
        >
          ← {{ t('shipment.title') }}
        </NuxtLink>
        <h1 class="page-title">
          {{ t('wizard.title') }}
        </h1>
      </div>
    </header>

    <section
      v-if="created"
      class="card result"
    >
      <h2 class="result-title">
        {{ t('wizard.result_title') }}
      </h2>
      <p class="result-awb">
        <span class="muted">{{ t('wizard.result_awb') }}:</span>
        <span class="mono">{{ created.awb }}</span>
      </p>
      <p class="muted">
        {{ created.courier.code.toUpperCase() }} / {{ created.courier.service }}
      </p>
      <div class="result-actions">
        <button
          type="button"
          class="btn-primary"
          @click="router.push(`/shipments/${created.id}`)"
        >
          {{ t('wizard.open_detail') }}
        </button>
        <button
          type="button"
          class="btn-secondary"
          @click="created = null"
        >
          {{ t('wizard.new_again') }}
        </button>
      </div>
    </section>

    <ShipmentWizard
      v-else
      @created="onCreated"
    />
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

.result {
  padding: var(--space-6);
  text-align: center;
}

.result-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-3);
}

.result-awb {
  font-size: var(--text-lg);
  margin: 0 0 var(--space-2);
}

.result-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: center;
  margin-top: var(--space-4);
}

.btn-primary,
.btn-secondary {
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

.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}
</style>