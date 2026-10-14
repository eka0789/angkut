<script setup lang="ts">
// Pickup (Phase 4) belum diimplementasikan. Halaman ini eksplisit karena
// /shipments/pickup akan tertangkap oleh /shipments/[id] bila tidak ada —
// dan pesan "belum tersedia" jauh lebih jujur daripada error "residu
// tidak ditemukan".
definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()

useHead({ title: () => t('nav.pickup') })
</script>

<template>
  <div class="placeholder">
    <header class="page-head">
      <h1 class="page-title">
        {{ t('nav.pickup') }}
      </h1>
    </header>

    <section class="card body">
      <p class="badge">
        Belum diimplementasikan
      </p>
      <p class="body-text">
        Permintaan penjemputan paket oleh kurir dijadwalkan pada Phase 4
        (TASK-036). Sampai saat itu, pengiriman dibuat lewat halaman
        <NuxtLink
          to="/shipments"
          class="link"
        >
          Pengiriman
        </NuxtLink>.
      </p>
    </section>
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: var(--space-5);
}

.page-title {
  font-size: var(--text-xl);
}

.body {
  padding: var(--space-6);
}

.badge {
  display: inline-block;
  margin: 0 0 var(--space-3);
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-surface-sunken);
  color: var(--color-ink-secondary);
  font-size: var(--text-xs);
  font-weight: 600;
}

.body-text {
  margin: 0;
  color: var(--color-ink-secondary);
}

.link {
  font-weight: 600;
}
</style>