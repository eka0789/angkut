<script setup lang="ts">
import type { NuxtError } from '#app'

// Halaman error global: ramah pengguna, tanpa stack trace. Detail teknis
// (request_id) ditampilkan agar bisa dirujuk ke dukungan.
const props = defineProps<{ error: NuxtError }>()

const isNotFound = computed(() => props.error?.statusCode === 404)

const title = computed(() => (isNotFound.value ? 'Halaman tidak ditemukan' : 'Terjadi kesalahan'))

const message = computed(() =>
  isNotFound.value
    ? 'Alamat yang Anda buka tidak ada atau sudah dipindahkan.'
    : 'Kami tidak dapat memproses permintaan ini. Silakan coba lagi sebentar lagi.',
)

function goHome() {
  clearError({ redirect: '/' })
}
</script>

<template>
  <div class="error-page">
    <div class="card error-card">
      <p class="error-code mono">
        {{ error?.statusCode || 500 }}
      </p>
      <h1 class="error-title">
        {{ title }}
      </h1>
      <p class="error-message">
        {{ message }}
      </p>
      <p
        v-if="error?.statusCode && !isNotFound"
        class="error-request mono"
      >
        {{ error.statusMessage || '' }}
      </p>
      <button
        type="button"
        class="error-button"
        @click="goHome"
      >
        Kembali ke beranda
      </button>
    </div>
  </div>
</template>

<style scoped>
.error-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: var(--space-6);
  background: var(--color-surface-subtle);
}

.error-card {
  max-width: 460px;
  width: 100%;
  padding: var(--space-10);
  text-align: center;
}

.error-code {
  margin: 0 0 var(--space-3);
  font-size: var(--text-2xl);
  color: var(--color-ink-muted);
}

.error-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-3);
}

.error-message {
  color: var(--color-ink-secondary);
  margin: 0 0 var(--space-6);
}

.error-request {
  color: var(--color-ink-muted);
  font-size: var(--text-xs);
  margin: 0 0 var(--space-6);
}

.error-button {
  font: inherit;
  font-weight: 600;
  color: var(--color-ink-inverse);
  background: var(--color-primary);
  border: none;
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-6);
  cursor: pointer;
}

.error-button:hover {
  background: var(--color-primary-hover);
}
</style>