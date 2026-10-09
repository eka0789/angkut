<script setup lang="ts">
definePageMeta({ layout: 'default' })

const { t } = useT()

useHead({ title: () => t('dashboard.title') })
</script>

<template>
  <div class="dashboard">
    <header class="page-head">
      <h1 class="page-title">
        {{ t('dashboard.title') }}
      </h1>
      <p class="page-subtitle">
        {{ t('dashboard.subtitle') }}
      </p>
    </header>

    <section class="card empty">
      <p class="empty-icon" aria-hidden="true">▦</p>
      <h2 class="empty-title">
        {{ t('dashboard.emptyTitle') }}
      </h2>
      <p class="empty-body">
        {{ t('dashboard.emptyBody') }}
      </p>
      <div class="empty-actions">
        <NuxtLink
          to="/shipments/new"
          class="btn-primary"
        >
          Buat pengiriman
        </NuxtLink>
        <NuxtLink
          to="/stores"
          class="btn-secondary"
        >
          Hubungkan toko
        </NuxtLink>
      </div>
    </section>
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: var(--space-6);
}

.page-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.page-subtitle {
  margin: 0;
  color: var(--color-ink-secondary);
}

.empty {
  padding: var(--space-16) var(--space-6);
  text-align: center;
}

.empty-icon {
  font-size: 40px;
  color: var(--color-ink-muted);
  margin: 0 0 var(--space-4);
}

.empty-title {
  font-size: var(--text-lg);
  margin-bottom: var(--space-2);
}

.empty-body {
  max-width: 54ch;
  margin: 0 auto var(--space-6);
  color: var(--color-ink-secondary);
  line-height: var(--leading-relaxed);
}

.empty-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: center;
  flex-wrap: wrap;
}

.btn-primary,
.btn-secondary {
  padding: var(--space-3) var(--space-5);
  border-radius: var(--radius-md);
  font-weight: 600;
}

.btn-primary {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
}

.btn-primary:hover {
  background: var(--color-primary-hover);
  color: var(--color-ink-inverse);
}

.btn-secondary {
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
}
</style>