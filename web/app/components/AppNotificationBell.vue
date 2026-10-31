<script setup lang="ts">
// Lonceng notifikasi di topbar: badge belum-dibaca + panel daftar singkat.
// Polling 30 detik cukup untuk pusat notifikasi (realtime SSE menyusul saat
// infrastruktur Redis tersedia — TASK-034).
import { ApiRequestError } from '~/composables/useApi'
import type { AppNotification } from '~/types/api'

const { t } = useT()
const { unreadCount, list, markRead } = useNotifications()
const route = useRoute()

const open = ref(false)
const unread = ref(0)
const items = ref<AppNotification[]>([])
const loading = ref(false)
const wrap = ref<HTMLElement | null>(null)

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) return e.message
  return t('common.error')
}

async function refreshBadge() {
  try {
    unread.value = await unreadCount()
  } catch {
    // Badge gagal memuat tidak boleh mengganggu halaman.
  }
}

async function openPanel() {
  open.value = !open.value
  if (!open.value) return
  loading.value = true
  try {
    const res = await list({ limit: 10 })
    items.value = res.items
  } catch (e) {
    items.value = []
    // Kegagalan panel dibiarkan senyap; badge tetap sumber kebenaran.
    void errText(e)
  } finally {
    loading.value = false
  }
}

async function onOpenItem(n: AppNotification) {
  if (!n.read_at) {
    try {
      await markRead(n.id)
      n.read_at = new Date().toISOString()
      unread.value = Math.max(0, unread.value - 1)
    } catch {
      // Tandai-baca gagal tidak memblokir navigasi.
    }
  }
  open.value = false
  const target = (n.data as { shipment_id?: string } | null)?.shipment_id
  if (target) void navigateTo(`/shipments/${target}`)
}

function onClickOutside(e: MouseEvent) {
  if (open.value && wrap.value && !wrap.value.contains(e.target as Node)) {
    open.value = false
  }
}

onMounted(() => {
  void refreshBadge()
  document.addEventListener('click', onClickOutside)
})
onBeforeUnmount(() => document.removeEventListener('click', onClickOutside))

// Segarkan badge saat pindah halaman (murah dan membuat badge terasa hidup).
watch(() => route.fullPath, () => void refreshBadge())

const relativeTime = (iso: string): string => {
  const d = new Date(iso)
  const diff = Date.now() - d.getTime()
  const m = Math.floor(diff / 60000)
  if (m < 1) return t('notif.just_now')
  if (m < 60) return t('notif.minutes_ago', { n: m })
  const h = Math.floor(m / 60)
  if (h < 24) return t('notif.hours_ago', { n: h })
  return d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' })
}
</script>

<template>
  <div
    ref="wrap"
    class="notif-bell"
  >
    <button
      type="button"
      class="bell-btn"
      :aria-label="t('notif.bell_aria')"
      @click="openPanel"
    >
      <AppIcon name="bell" :size="20" />
      <span
        v-if="unread > 0"
        class="bell-badge"
      >
        {{ unread > 99 ? '99+' : unread }}
      </span>
    </button>

    <div
      v-if="open"
      class="notif-panel"
      role="dialog"
      :aria-label="t('notif.title')"
    >
      <div class="notif-head">
        <strong>{{ t('notif.title') }}</strong>
        <NuxtLink
          to="/settings/notifications"
          class="notif-settings"
          @click="open = false"
        >
          {{ t('notif.settings') }}
        </NuxtLink>
      </div>
      <p
        v-if="loading"
        class="notif-state"
      >
        {{ t('common.loading') }}
      </p>
      <p
        v-else-if="items.length === 0"
        class="notif-state"
      >
        {{ t('notif.empty') }}
      </p>
      <ul
        v-else
        class="notif-list"
      >
        <li
          v-for="n in items"
          :key="n.id"
        >
          <button
            type="button"
            class="notif-item"
            :class="{ unread: !n.read_at, [`p-${n.priority}`]: true }"
            @click="onOpenItem(n)"
          >
            <span class="notif-item-title">{{ n.title }}</span>
            <span class="notif-item-body">{{ n.body }}</span>
            <span class="notif-item-time">{{ relativeTime(n.created_at) }}</span>
          </button>
        </li>
      </ul>
      <NuxtLink
        v-if="items.length > 0"
        to="/notifications"
        class="notif-all"
        @click="open = false"
      >
        {{ t('notif.see_all') }}
      </NuxtLink>
    </div>
  </div>
</template>

<style scoped>
.notif-bell {
  position: relative;
}

.bell-btn {
  position: relative;
  border: none;
  background: transparent;
  font-size: 18px;
  cursor: pointer;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-md);
}

.bell-btn:hover {
  background: var(--color-border);
}

.bell-badge {
  position: absolute;
  top: -2px;
  right: -2px;
  background: #dc2626;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  min-width: 16px;
  height: 16px;
  line-height: 16px;
  text-align: center;
  border-radius: 999px;
  padding: 0 3px;
}

.notif-panel {
  position: absolute;
  right: 0;
  top: calc(100% + var(--space-2));
  width: 340px;
  max-width: 90vw;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
  z-index: 50;
  overflow: hidden;
}

.notif-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-3);
  border-bottom: 1px solid var(--color-border);
}

.notif-settings {
  font-size: var(--text-xs);
  color: var(--color-ink-secondary);
}

.notif-state {
  padding: var(--space-4);
  text-align: center;
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}

.notif-list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 320px;
  overflow-y: auto;
}

.notif-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  text-align: left;
  padding: var(--space-2) var(--space-3);
  border: none;
  border-bottom: 1px solid var(--color-border);
  background: transparent;
  cursor: pointer;
  font: inherit;
}

.notif-item:hover {
  background: var(--color-border);
}

.notif-item.unread {
  background: #eff6ff;
}

.notif-item.p-high .notif-item-title {
  color: #9f1239;
}

.notif-item-title {
  font-weight: 600;
  font-size: var(--text-sm);
}

.notif-item-body {
  font-size: var(--text-xs);
  color: var(--color-ink-secondary);
}

.notif-item-time {
  font-size: 10px;
  color: var(--color-ink-muted);
}

.notif-all {
  display: block;
  text-align: center;
  padding: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  border-top: 1px solid var(--color-border);
}
</style>
