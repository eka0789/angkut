<script setup lang="ts">
import AppIcon from '~/components/AppIcon.vue'
// Halaman docs publik (TASK-064): ringkasan REST API + contoh cURL.
definePageMeta({ layout: 'public' })
useHead({ title: () => 'Dokumentasi API — Angkut' })

const groups = [
  {
    title: 'Autentikasi',
    desc: 'JWT bearer token dari /auth/login, atau API key via header X-API-Key (scope terbatas).',
    endpoints: ['POST /api/v1/auth/register', 'POST /api/v1/auth/login', 'POST /api/v1/auth/refresh'],
  },
  {
    title: 'Pengiriman',
    desc: 'Semua mutasi wajib header Idempotency-Key — retry dengan key sama tidak menghasilkan duplikat.',
    endpoints: ['POST /api/v1/shipments', 'GET /api/v1/shipments', 'GET /api/v1/shipments/{id}/label?format=a4|thermal', 'POST /api/v1/shipments/bulk (CSV)'],
  },
  {
    title: 'Pelacakan',
    desc: 'Timeline kejadian per resi; endpoint publik untuk dibagikan ke pembeli tanpa akun.',
    endpoints: ['GET /api/v1/tracking/{awb}', 'GET /api/v1/tracking/{awb}/stream (SSE)', 'GET /api/v1/public/track/{awb}'],
  },
  {
    title: 'COD & Keuangan',
    desc: 'Tagihan COD, settlement per kurir, buku besar append-only.',
    endpoints: ['GET /api/v1/cod/summary', 'POST /api/v1/cod/settlements', 'POST /api/v1/cod/settlements/{id}/mark-paid', 'GET /api/v1/analytics/financial'],
  },
  {
    title: 'Inventory',
    desc: 'Produk, varian SKU, saldo per gudang, movement append-only, transfer atomik.',
    endpoints: ['GET /api/v1/products', 'POST /api/v1/inventory/adjust', 'GET /api/v1/inventory/low-stock', 'POST /api/v1/warehouses/transfers'],
  },
  {
    title: 'Webhook & AI',
    desc: 'Terima event kurir dengan verifikasi HMAC; asisten AI dengan konfirmasi aksi.',
    endpoints: ['POST /api/v1/webhooks/{provider} (HMAC)', 'POST /api/v1/ai/chat', 'POST /api/v1/ai/actions/{id}/confirm'],
  },
]

const curlExample = `curl -X POST https://api.angkut.id/api/v1/shipments \\
  -H "Authorization: Bearer <token>" \\
  -H "Idempotency-Key: order-12345" \\
  -H "Content-Type: application/json" \\
  -d '{
    "warehouse_id": "0192...",
    "courier_code": "jne",
    "service_code": "REG",
    "origin_address_id": "0192...",
    "destination_address_id": "0192...",
    "weight": 1.2,
    "package_type": "box"
  }'`
</script>

<template>
  <div class="docs">
    <header class="hero">
      <span class="hero-badge"><AppIcon :name='"book"' :size="14" /> Dokumentasi API</span>
      <h1>Dokumentasi API</h1>
      <p>REST API dengan envelope JSON konsisten. Semua mutasi kritis idempoten.</p>
    </header>

    <section class="wrap">
      <h2>Contoh cepat</h2>
      <pre class="code">{{ curlExample }}</pre>
    </section>

    <section
      v-for="g in groups"
      :key="g.title"
      class="wrap"
    >
      <h2>{{ g.title }}</h2>
      <p class="desc">
        {{ g.desc }}
      </p>
      <ul class="endpoints">
        <li
          v-for="e in g.endpoints"
          :key="e"
          class="mono"
        >
          {{ e }}
        </li>
      </ul>
    </section>

    <section class="wrap">
      <h2>Format respons</h2>
      <pre class="code">{
  "success": true,
  "data": { ... },
  "error": null,        // { code, message, fields? } bila gagal
  "meta": { "total": 42, "limit": 20, "offset": 0 }
}</pre>
    </section>
  </div>
</template>

<style scoped>
.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-4);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-xs);
  font-weight: 700;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  border: 1px solid color-mix(in srgb, var(--color-primary) 25%, transparent);
  border-radius: var(--radius-pill);
}
.hero { text-align: center; padding: var(--space-10) var(--space-4) var(--space-6); }
.hero h1 { font-size: 32px; margin: 0 0 var(--space-3); }
.hero p { color: var(--color-ink-secondary); margin: 0; }

.wrap { max-width: 780px; margin: 0 auto var(--space-8); padding: 0 var(--space-4); }
.wrap h2 { margin-bottom: var(--space-2); }
.desc { color: var(--color-ink-secondary); font-size: var(--text-sm); margin: 0 0 var(--space-3); }

.code {
  background: #0f172a;
  color: #e2e8f0;
  padding: var(--space-4);
  border-radius: var(--radius-md);
  font-family: ui-monospace, monospace;
  font-size: var(--text-xs);
  overflow-x: auto;
  white-space: pre;
  margin: 0;
}

.endpoints { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-1); }
.endpoints li {
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
}
</style>
