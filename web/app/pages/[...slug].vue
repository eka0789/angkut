<script setup lang="ts">
// Catch-all untuk rute modul yang sudah direncanakan tetapi belum
// diimplementasikan. Rute di luar daftar modul terencana ditolak oleh
// middleware `planned-module-only` dan menghasilkan 404 sungguhan.
import { sectionTitleForPath } from '~/utils/navigation'

definePageMeta({ middleware: 'planned-module-only' })

const { t } = useT()
const route = useRoute()

const title = computed(() => {
  const key = sectionTitleForPath(route.path)
  return key ? t(key) : 'Modul'
})

const moduleName = computed(() => route.path.split('/').filter(Boolean)[0] ?? 'modul')

useHead({ title: () => title.value })
</script>

<template>
  <div class="placeholder">
    <header class="page-head">
      <h1 class="page-title">
        {{ title }}
      </h1>
    </header>

    <section class="card body">
      <p class="badge">
        Belum diimplementasikan
      </p>
      <h2 class="body-title">
        Modul “{{ moduleName }}” sedang dalam pengembangan
      </h2>
      <p class="body-text">
        Rute ini sudah terdaftar dan siap diisi. Sesuai rencana fase,
        implementasinya menyusul setelah fondasi (autentikasi &amp; tenant) selesai —
        lihat <span class="mono">dev-docs/ai/TASKS.md</span>.
      </p>
      <NuxtLink
        to="/dashboard"
        class="btn"
      >
        Kembali ke dasbor
      </NuxtLink>
    </section>
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: var(--space-6);
}

.page-title {
  font-size: var(--text-xl);
}

.body {
  padding: var(--space-10);
  text-align: center;
}

.badge {
  display: inline-block;
  margin: 0 0 var(--space-4);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-status-delayed);
  background: #fef6e7;
  border-radius: var(--radius-pill);
}

.body-title {
  font-size: var(--text-lg);
  margin-bottom: var(--space-3);
}

.body-text {
  max-width: 56ch;
  margin: 0 auto var(--space-6);
  color: var(--color-ink-secondary);
  line-height: var(--leading-relaxed);
}

.btn {
  display: inline-block;
  padding: var(--space-3) var(--space-5);
  border-radius: var(--radius-md);
  font-weight: 600;
  background: var(--color-primary);
  color: var(--color-ink-inverse);
}

.btn:hover {
  background: var(--color-primary-hover);
  color: var(--color-ink-inverse);
}
</style>