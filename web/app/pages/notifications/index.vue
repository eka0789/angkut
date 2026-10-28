<script setup lang="ts">
// Pusat notifikasi: daftar lengkap, filter belum-dibaca, tandai dibaca.
import { ApiRequestError } from '~/composables/useApi'
import type { AppNotification, NotificationPriority } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { list, markRead, markAllRead } = useNotifications()

useHead({ title: () => t('notif.title') })

const items = ref<AppNotification[]>([])
const total = ref(0)
const limit = 20
const offset = ref(0)
const loading = ref(true)
const error = ref<string | null>(null)
const message = ref<string | null>(null)
const onlyUnread = ref(false)
const priorityFilter = ref<'' | NotificationPriority>('')

async function load() {
  loading.value = true
  error.value = null
  try {
    const res = await list({
      unread: onlyUnread.value,
      priority: priorityFilter.value || undefined,
      limit,
      offset: offset.value,
    })
    items.value = res.items
    total.value = res.meta.total
  } catch (e) {
    error.value = e instanceof ApiRequestError ? e.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function onRead(n: AppNotification) {
  if (n.read_at) return
  try {
    await markRead(n.id)
    n.read_at = new Date().toISOString()
    total.value = Math.max(0, total.value - 1)
  } catch (e) {
    error.value = e instanceof ApiRequestError ? e.message : t('common.error')
  }
}

async function onReadAll() {
  error.value = null
  try {
    const marked = await markAllRead()
    message.value = t('notif.all_marked', { n: marked })
    await load()
  } catch (e) {
    error.value = e instanceof ApiRequestError ? e.message : t('common.error')
  }
}

function toggleUnread() {
  onlyUnread.value = !onlyUnread.value
  offset.value = 0
  void load()
}

function onPriorityChange() {
  offset.value = 0
  void load()
}

function nextPage() {
  if (offset.value + limit < total.value) {
    offset.value += limit
    void load()
  }
}

const hasPrev = computed(() => offset.value > 0)
const hasNext = computed(() => offset.value + limit < total.value)

onMounted(load)
</script>

<template>
  <div class="notif-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('notif.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('notif.subtitle') }}
        </p>
      </div>
      <button
        type="button"
        class="btn-secondary"
        @click="onReadAll"
      >
        {{ t('notif.mark_all') }}
      </button>
    </header>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>
    <div
      v-if="message"
      class="alert ok"
      role="status"
    >
      {{ message }}
    </div>

    <section class="card toolbar">
      <button
        type="button"
        class="btn-mini"
        :class="{ active: onlyUnread }"
        @click="toggleUnread"
      >
        {{ t('notif.unread_only') }}
      </button>
      <select
        v-model="priorityFilter"
        class="filter"
        :aria-label="t('notif.priority')"
        @change="onPriorityChange"
      >
        <option value="">
          {{ t('notif.priority_all') }}
        </option>
        <option value="high">
          {{ t('notif.priority_high') }}
        </option>
        <option value="normal">
          {{ t('notif.priority_normal') }}
        </option>
        <option value="low">
          {{ t('notif.priority_low') }}
        </option>
      </select>
    </section>

    <section class="card table-wrap">
      <p
        v-if="loading"
        class="state muted"
      >
        {{ t('common.loading') }}
      </p>
      <p
        v-else-if="items.length === 0"
        class="state muted"
      >
        {{ t('notif.empty') }}
      </p>
      <ul
        v-else
        class="notif-rows"
      >
        <li
          v-for="n in items"
          :key="n.id"
        >
          <button
            type="button"
            class="notif-row"
            :class="{ unread: !n.read_at, [`p-${n.priority}`]: true }"
            @click="onRead(n)"
          >
            <span class="row-main">
              <span class="row-title">
                {{ n.title }}
                <span
                  v-if="!n.read_at"
                  class="dot"
                  :title="t('notif.unread_dot')"
                />
              </span>
              <span class="row-body">{{ n.body }}</span>
            </span>
            <time class="row-time">{{ new Date(n.created_at).toLocaleString('id-ID') }}</time>
          </button>
        </li>
      </ul>
    </section>

    <div
      v-if="total > limit"
      class="pager"
    >
      <button
        type="button"
        class="btn-mini"
        :disabled="!hasPrev"
        @click="offset = Math.max(0, offset - limit); load()"
      >
        {{ t('common.back') }}
      </button>
      <span class="muted">{{ offset + 1 }}–{{ Math.min(offset + limit, total) }} / {{ total }}</span>
      <button
        type="button"
        class="btn-mini"
        :disabled="!hasNext"
        @click="nextPage"
      >
        {{ t('common.next') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
  margin-bottom: var(--space-5);
}

.page-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.page-subtitle {
  margin: 0;
  color: var(--color-ink-secondary);
}

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-3);
}

.alert.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.alert.ok {
  border: 1px solid #bbf7d0;
  background: #f0fdf4;
  color: #14532d;
}

.toolbar {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  margin-bottom: var(--space-4);
  align-items: center;
}

.filter {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

.btn-secondary,
.btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-ink);
  font: inherit;
}

.btn-mini.active {
  border-color: var(--color-accent, #0f766e);
  color: var(--color-accent, #0f766e);
}

.table-wrap {
  padding: var(--space-2) 0;
}

.state {
  padding: var(--space-6) 0;
  text-align: center;
}

.muted {
  color: var(--color-ink-muted);
}

.notif-rows {
  list-style: none;
  margin: 0;
  padding: 0;
}

.notif-row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  width: 100%;
  text-align: left;
  padding: var(--space-3) var(--space-4);
  border: none;
  border-bottom: 1px solid var(--color-border);
  background: transparent;
  cursor: pointer;
  font: inherit;
}

.notif-row:hover {
  background: var(--color-border);
}

.notif-row.unread {
  background: #eff6ff;
}

.notif-row.p-high .row-title {
  color: #9f1239;
}

.row-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.row-title {
  font-weight: 600;
  font-size: var(--text-sm);
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #2563eb;
  display: inline-block;
}

.row-body {
  font-size: var(--text-sm);
  color: var(--color-ink-secondary);
}

.row-time {
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
  white-space: nowrap;
}

.pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  margin-top: var(--space-3);
  font-size: var(--text-sm);
}
</style>
