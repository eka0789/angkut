<script setup lang="ts">
// Layout publik: navbar kaca + footer dua kolom. Dipakai landing, pricing,
// tracking publik, kalkulator ongkir, fitur, dan docs API.
const { t } = useT()
const year = new Date().getFullYear()
</script>

<template>
  <div class="public-layout">
    <header class="public-nav">
      <div class="public-nav-inner">
        <NuxtLink
          to="/"
          class="public-brand"
        >
          <img
            src="~/assets/brand/logo.svg"
            alt="Angkut"
            height="26"
          >
        </NuxtLink>

        <nav class="public-links">
          <NuxtLink to="/features">
            {{ t('nav.features') }}
          </NuxtLink>
          <NuxtLink to="/pricing">
            {{ t('nav.pricing') }}
          </NuxtLink>
          <NuxtLink to="/track">
            {{ t('nav.trackPackage') }}
          </NuxtLink>
        </nav>

        <div class="public-actions">
          <NuxtLink
            to="/login"
            class="public-login"
          >
            {{ t('auth.login') }}
          </NuxtLink>
          <NuxtLink
            to="/register"
            class="public-cta"
          >
            {{ t('auth.register') }}
          </NuxtLink>
        </div>
      </div>
    </header>

    <main class="public-content">
      <slot />
    </main>

    <footer class="public-footer">
      <div class="footer-inner">
        <div class="footer-brand">
          <img
            src="~/assets/brand/logo.svg"
            alt="Angkut"
            height="24"
          >
          <p>Platform logistik cerdas untuk bisnis Indonesia.</p>
        </div>
        <nav class="footer-links">
          <NuxtLink to="/features">
            Fitur
          </NuxtLink>
          <NuxtLink to="/pricing">
            Harga
          </NuxtLink>
          <NuxtLink to="/kalkulator">
            Kalkulator Ongkir
          </NuxtLink>
          <NuxtLink to="/docs">
            Dokumentasi API
          </NuxtLink>
          <NuxtLink to="/track">
            Lacak Paket
          </NuxtLink>
        </nav>
      </div>
      <p class="footer-copy">
        © {{ year }} Angkut. Dibangun dengan teliti untuk pengiriman Indonesia.
      </p>
    </footer>
  </div>
</template>

<style scoped>
.public-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--color-surface);
}

.public-nav {
  border-bottom: 1px solid var(--color-border);
  position: sticky;
  top: 0;
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(12px) saturate(1.4);
  -webkit-backdrop-filter: blur(12px) saturate(1.4);
  z-index: 10;
}

.public-nav-inner {
  max-width: var(--content-max);
  margin: 0 auto;
  height: var(--topbar-height);
  padding: 0 var(--space-6);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-6);
}

.public-brand img {
  display: block;
  transition: transform 200ms var(--ease-spring);
}
.public-brand:hover img { transform: scale(1.05); }

.public-links {
  display: flex;
  gap: var(--space-2);
  font-weight: 500;
  color: var(--color-ink-secondary);
}

.public-links a {
  color: inherit;
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-md);
  position: relative;
  transition: color 180ms ease, background-color 180ms ease;
}
.public-links a:hover {
  color: var(--color-primary);
  background: var(--color-primary-soft);
}
.public-links a.router-link-exact-active {
  color: var(--color-primary);
  font-weight: 700;
}

.public-actions {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}

.public-login {
  font-weight: 600;
  color: var(--color-ink-secondary);
  transition: color 180ms ease;
}
.public-login:hover { color: var(--color-primary); }

.public-cta {
  background: var(--gradient-brand);
  color: var(--color-ink-inverse);
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  font-weight: 600;
  position: relative;
  overflow: hidden;
  box-shadow: var(--shadow-xs);
}
.public-cta:hover {
  color: var(--color-ink-inverse);
  box-shadow: var(--shadow-primary);
  transform: translateY(-1px);
}
.public-cta:active { transform: translateY(1px); }

.public-content {
  flex: 1;
  max-width: var(--content-max);
  width: 100%;
  margin: 0 auto;
  padding: var(--space-10) var(--space-6);
}

/* ── Footer ───────────────────────────────────────────────────────────── */
.public-footer {
  border-top: 1px solid var(--color-border);
  background: var(--color-surface-subtle);
  padding: var(--space-8) var(--space-6) var(--space-6);
}

.footer-inner {
  max-width: var(--content-max);
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-8);
  flex-wrap: wrap;
}

.footer-brand img { display: block; margin-bottom: var(--space-2); }
.footer-brand p {
  margin: 0;
  color: var(--color-ink-secondary);
  font-size: var(--text-sm);
  max-width: 32ch;
}

.footer-links {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-5);
  font-size: var(--text-sm);
}
.footer-links a {
  color: var(--color-ink-secondary);
  transition: color 160ms ease;
}
.footer-links a:hover { color: var(--color-primary); }

.footer-copy {
  max-width: var(--content-max);
  margin: var(--space-6) auto 0;
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
  color: var(--color-ink-muted);
  font-size: var(--text-xs);
}
</style>
