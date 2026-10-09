<script setup lang="ts">
// Layout publik: navbar sederhana + footer. Dipakai landing, pricing, tracking
// publik, dan kalkulator ongkir.
const { t } = useT()
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
      <p>© {{ new Date().getFullYear() }} Angkut. Platform logistik cerdas untuk bisnis Indonesia.</p>
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
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(8px);
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

.public-links {
  display: flex;
  gap: var(--space-6);
  color: var(--color-ink-secondary);
  font-weight: 500;
}

.public-links a {
  color: inherit;
}

.public-links a:hover {
  color: var(--color-primary);
}

.public-actions {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}

.public-login {
  font-weight: 600;
  color: var(--color-ink-secondary);
}

.public-cta {
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  font-weight: 600;
}

.public-cta:hover {
  background: var(--color-primary-hover);
  color: var(--color-ink-inverse);
}

.public-content {
  flex: 1;
  max-width: var(--content-max);
  width: 100%;
  margin: 0 auto;
  padding: var(--space-10) var(--space-6);
}

.public-footer {
  border-top: 1px solid var(--color-border);
  padding: var(--space-6);
  text-align: center;
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}
</style>