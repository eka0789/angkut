<script setup lang="ts">
// App shell: sidebar + topbar. Navigasi diringkas ke bagian inti; item lain
// ditambahkan saat modul terkait diimplementasikan (Phase 3+).
import { navSections } from '~/utils/navigation'

const route = useRoute()
const router = useRouter()
const { t } = useT()
const { session, isAuthenticated, logout } = useAuth()

const isActive = (to: string): boolean =>
  to === '/dashboard' ? route.path === to : route.path.startsWith(to)

/** Nama organisasi aktif (untuk penanda tenant di topbar). */
const activeOrgName = computed(() =>
  session.value?.organizations.find(o => o.id === session.value?.active_org_id)?.name ?? '')

async function onLogout() {
  await logout()
  router.push('/login')
}
</script>

<template>
  <div class="shell">
    <aside class="shell-sidebar">
      <div class="shell-brand">
        <img
          src="~/assets/brand/logo-mark.svg"
          alt=""
          width="28"
          height="28"
        >
        <span class="shell-brand-name">Angkut</span>
      </div>

      <nav class="shell-nav">
        <div
          v-for="section in navSections"
          :key="section.title"
          class="shell-nav-section"
        >
          <p class="shell-nav-title">
            {{ t(section.title) }}
          </p>
          <NuxtLink
            v-for="item in section.items"
            :key="item.to"
            :to="item.to"
            class="shell-nav-link"
            :class="{ 'is-active': isActive(item.to) }"
          >
            <span class="shell-nav-icon" aria-hidden="true">{{ item.icon }}</span>
            <span>{{ t(item.label) }}</span>
          </NuxtLink>
        </div>
      </nav>
    </aside>

    <div class="shell-main">
      <header class="shell-topbar">
        <div class="shell-topbar-title">
          <slot name="title">
            {{ t('nav.dashboard') }}
          </slot>
        </div>
        <div class="shell-topbar-actions">
          <slot name="actions" />
          <div
            v-if="isAuthenticated && session"
            class="shell-user"
          >
            <span class="shell-user-meta">
              <span class="shell-user-name">{{ session.user.name || session.user.email }}</span>
              <span class="shell-user-org">{{ activeOrgName }}</span>
            </span>
            <button
              type="button"
              class="shell-logout"
              @click="onLogout"
            >
              {{ t('auth.logout') }}
            </button>
          </div>
        </div>
      </header>

      <main class="shell-content">
        <slot />
      </main>
    </div>
  </div>
</template>

<style scoped>
.shell {
  display: grid;
  grid-template-columns: var(--sidebar-width) 1fr;
  min-height: 100vh;
}

.shell-sidebar {
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
  padding: var(--space-4) var(--space-3);
  position: sticky;
  top: 0;
  height: 100vh;
  overflow-y: auto;
}

.shell-brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3) var(--space-5);
}

.shell-brand-name {
  font-size: var(--text-md);
  font-weight: 700;
  letter-spacing: -0.02em;
}

.shell-nav-section + .shell-nav-section {
  margin-top: var(--space-5);
}

.shell-nav-title {
  margin: 0 0 var(--space-2);
  padding: 0 var(--space-3);
  font-size: var(--text-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--color-ink-muted);
}

.shell-nav-link {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  color: var(--color-ink-secondary);
  font-weight: 500;
}

.shell-nav-link:hover {
  background: var(--color-surface-sunken);
  color: var(--color-ink);
}

.shell-nav-link.is-active {
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-weight: 600;
}

.shell-nav-icon {
  width: 20px;
  text-align: center;
}

.shell-main {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.shell-topbar {
  height: var(--topbar-height);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: 0 var(--space-6);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  position: sticky;
  top: 0;
  z-index: 10;
}

.shell-topbar-title {
  font-weight: 600;
}

.shell-user {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.shell-user-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  line-height: 1.2;
}

.shell-user-name {
  font-size: var(--text-sm);
  font-weight: 600;
}

.shell-user-org {
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.shell-logout {
  padding: var(--space-1) var(--space-3);
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-ink-secondary);
  cursor: pointer;
}

.shell-logout:hover {
  background: var(--color-surface-sunken);
  color: var(--color-ink);
}

.shell-content {
  padding: var(--space-6);
  max-width: var(--content-max);
  width: 100%;
}

/* Layar kecil: sidebar mengecil jadi baris ikon agar tetap bisa dipakai. */
@media (max-width: 1024px) {
  .shell {
    grid-template-columns: 72px 1fr;
  }

  .shell-brand-name,
  .shell-nav-title,
  .shell-nav-link span:last-child {
    display: none;
  }

  .shell-nav-link {
    justify-content: center;
  }
}
</style>