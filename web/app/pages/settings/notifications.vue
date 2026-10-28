<script setup lang="ts">
// Pengaturan notifikasi: preferensi per event × kanal, dan template pesan
// (admin+ saja — server menolak dengan 403 bila role tidak berwenang).
import { ApiRequestError } from '~/composables/useApi'
import type {
  NotificationChannel,
  NotificationEventKey,
  NotificationPreferenceView,
  NotificationTemplateView,
} from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { preferences, updatePreference, templates, updateTemplate } = useNotifications()

useHead({ title: () => t('notif.settings_title') })

const prefs = ref<NotificationPreferenceView[]>([])
const tmpls = ref<NotificationTemplateView[]>([])
const loading = ref(true)
const saving = ref<string | null>(null)
const error = ref<string | null>(null)
const message = ref<string | null>(null)

// Editor template: key yang sedang dibuka.
const editing = ref<NotificationEventKey | null>(null)
const editTitle = ref('')
const editBody = ref('')

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) {
    if (e.status === 403) return t('notif.template_forbidden')
    const fields = e.fields ? Object.values(e.fields) : []
    return [e.message, ...fields].filter(Boolean).join(' ')
  }
  return t('common.error')
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const [p, tm] = await Promise.all([preferences(), templates()])
    prefs.value = p
    tmpls.value = tm
  } catch (e) {
    error.value = errText(e)
  } finally {
    loading.value = false
  }
}

async function toggle(p: NotificationPreferenceView) {
  const id = `${p.event_key}:${p.channel}`
  saving.value = id
  error.value = null
  try {
    await updatePreference(p.event_key, p.channel, !p.enabled)
    p.enabled = !p.enabled
    p.explicit = true
  } catch (e) {
    error.value = errText(e)
  } finally {
    saving.value = null
  }
}

function openEditor(t: NotificationTemplateView) {
  editing.value = t.key
  editTitle.value = t.title_template
  editBody.value = t.body_template
}

async function saveTemplate() {
  if (!editing.value) return
  saving.value = `tpl:${editing.value}`
  error.value = null
  message.value = null
  try {
    await updateTemplate(editing.value, editTitle.value, editBody.value)
    message.value = t('notif.template_saved')
    editing.value = null
    await load()
  } catch (e) {
    error.value = errText(e)
  } finally {
    saving.value = null
  }
}

const channelLabel = (ch: NotificationChannel): string =>
  ch === 'inapp' ? t('notif.channel_inapp') : t('notif.channel_email')

const eventLabel = (key: NotificationEventKey): string => t(`notif.event.${key}`)

onMounted(load)
</script>

<template>
  <div class="notif-settings-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('notif.settings_title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('notif.settings_subtitle') }}
        </p>
      </div>
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

    <section class="card panel">
      <h2 class="panel-title">
        {{ t('notif.prefs_title') }}
      </h2>
      <p
        v-if="loading"
        class="muted"
      >
        {{ t('common.loading') }}
      </p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">
              {{ t('notif.col_event') }}
            </th>
            <th scope="col">
              {{ t('notif.channel_inapp') }}
            </th>
            <th scope="col">
              {{ t('notif.channel_email') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="p in prefs.filter(x => x.channel === 'inapp')"
            :key="p.event_key"
          >
            <td>{{ eventLabel(p.event_key) }}</td>
            <td>
              <button
                type="button"
                class="toggle"
                :class="{ on: p.enabled }"
                :disabled="saving === `${p.event_key}:${p.channel}`"
                :aria-pressed="p.enabled"
                :aria-label="`${eventLabel(p.event_key)} — ${channelLabel(p.channel)}`"
                @click="toggle(p)"
              >
                {{ p.enabled ? t('notif.on') : t('notif.off') }}
              </button>
            </td>
            <td>
              <button
                type="button"
                class="toggle"
                :class="{ on: prefs.find(x => x.event_key === p.event_key && x.channel === 'email')?.enabled }"
                :disabled="saving === `${p.event_key}:email`"
                :aria-pressed="prefs.find(x => x.event_key === p.event_key && x.channel === 'email')?.enabled"
                :aria-label="`${eventLabel(p.event_key)} — ${channelLabel('email')}`"
                @click="toggle(prefs.find(x => x.event_key === p.event_key && x.channel === 'email')!)"
              >
                {{ prefs.find(x => x.event_key === p.event_key && x.channel === 'email')?.enabled ? t('notif.on') : t('notif.off') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p class="muted hint">
        {{ t('notif.email_optin_hint') }}
      </p>
    </section>

    <section class="card panel">
      <h2 class="panel-title">
        {{ t('notif.template_title') }}
      </h2>
      <p class="muted">
        {{ t('notif.template_hint') }}
      </p>
      <table>
        <thead>
          <tr>
            <th scope="col">
              {{ t('notif.col_event') }}
            </th>
            <th scope="col">
              {{ t('notif.col_template') }}
            </th>
            <th scope="col">
              {{ t('common.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="tpl in tmpls"
            :key="tpl.key"
          >
            <td>{{ eventLabel(tpl.key) }}</td>
            <td class="tpl-cell">
              <strong>{{ tpl.title_template }}</strong>
              <span class="tpl-body">{{ tpl.body_template }}</span>
            </td>
            <td>
              <button
                type="button"
                class="btn-mini"
                @click="openEditor(tpl)"
              >
                {{ t('common.edit') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <div
        v-if="editing"
        class="editor"
      >
        <h3 class="panel-title">
          {{ t('notif.edit_template', { event: eventLabel(editing) }) }}
        </h3>
        <label class="field">
          <span>{{ t('notif.col_title') }}</span>
          <input
            v-model="editTitle"
            type="text"
            class="input"
          >
        </label>
        <label class="field">
          <span>{{ t('notif.col_body') }}</span>
          <textarea
            v-model="editBody"
            rows="3"
            class="input"
          />
        </label>
        <p class="muted hint">
          {{ t('notif.vars_hint') }}
        </p>
        <div class="row">
          <button
            type="button"
            class="btn-primary"
            :disabled="saving === `tpl:${editing}`"
            @click="saveTemplate"
          >
            {{ t('common.save') }}
          </button>
          <button
            type="button"
            class="btn-mini"
            @click="editing = null"
          >
            {{ t('common.cancel') }}
          </button>
        </div>
      </div>
    </section>
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

.panel {
  padding: var(--space-4);
  margin-bottom: var(--space-4);
}

.panel-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-2);
}

.muted {
  color: var(--color-ink-muted);
  font-size: var(--text-sm);
}

.hint {
  margin-top: var(--space-2);
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

th,
td {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
}

th {
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: 0.02em;
  color: var(--color-ink-muted);
}

.toggle {
  padding: var(--space-1) var(--space-3);
  border-radius: 999px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-ink-muted);
  cursor: pointer;
  font: inherit;
  font-size: var(--text-xs);
  font-weight: 600;
}

.toggle.on {
  border-color: #bbf7d0;
  background: #f0fdf4;
  color: #14532d;
}

.toggle:disabled {
  opacity: 0.55;
  cursor: wait;
}

.tpl-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tpl-body {
  color: var(--color-ink-secondary);
  font-size: var(--text-xs);
}

.editor {
  margin-top: var(--space-4);
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-bottom: var(--space-3);
  font-size: var(--text-sm);
}

.input {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

.row {
  display: flex;
  gap: var(--space-2);
}

.btn-primary,
.btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
  font: inherit;
}

.btn-primary {
  background: var(--color-accent, #0f766e);
  color: #fff;
}

.btn-mini {
  border-color: var(--color-border);
  background: var(--color-surface);
  color: var(--color-ink);
}
</style>
