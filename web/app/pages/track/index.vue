<script setup lang="ts">
// /track — pintu masuk pelacakan publik. Halaman ini tidak butuh akun:
// penerima paket biasanya hanya memegang nomor resi dari label.
definePageMeta({ layout: 'public' })

const { t } = useT()
const router = useRouter()
const awb = ref('')

useHead({ title: () => t('tracking.public_title') })

function submit() {
  const clean = awb.value.trim().toUpperCase()
  if (!clean) return
  router.push(`/track/${encodeURIComponent(clean)}`)
}
</script>

<template>
  <div class="track-home">
    <section class="hero">
      <h1 class="title">
        {{ t('tracking.public_title') }}
      </h1>
      <p class="sub">
        {{ t('tracking.public_subtitle') }}
      </p>

      <form
        class="search"
        @submit.prevent="submit"
      >
        <label
          class="sr-only"
          for="awb"
        >{{ t('tracking.awb') }}</label>
        <input
          id="awb"
          v-model="awb"
          type="text"
          class="input mono"
          :placeholder="t('tracking.awb_placeholder')"
          autocomplete="off"
          autocapitalize="characters"
          spellcheck="false"
        >
        <button
          type="submit"
          class="btn btn-primary"
          :disabled="!awb.trim()"
        >
          {{ t('tracking.search') }}
        </button>
      </form>
    </section>
  </div>
</template>

<style scoped>
.track-home {
  max-width: 720px;
  margin: 0 auto;
  padding: var(--space-10) var(--space-4);
}

.hero {
  text-align: center;
}

.title {
  font-size: var(--text-2xl);
  margin-bottom: var(--space-2);
}

.sub {
  color: var(--color-ink-secondary);
  margin-bottom: var(--space-6);
}

.search {
  display: flex;
  gap: var(--space-2);
  max-width: 520px;
  margin: 0 auto;
}

.input {
  flex: 1;
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  font-size: var(--text-md);
}

.input:focus {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
}

.btn {
  padding: var(--space-3) var(--space-5);
  border-radius: var(--radius-md);
  font-weight: 600;
  border: 0;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
}

.btn:disabled {
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

@media (max-width: 480px) {
  .search {
    flex-direction: column;
  }
}
</style>
