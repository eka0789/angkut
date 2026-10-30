<script setup lang="ts">
// Halaman AI (TASK-048/054): chat penuh + kartu insight proaktif. Guardrail:
// aksi mutasi selalu butuh konfirmasi eksplisit.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { apiFetch } = useApi()

useHead({ title: () => t('ai.page_title') })

interface Insight { title: string, body: string, level: string }
interface Msg { role: 'user' | 'assistant', content: string }

const messages = ref<Msg[]>([])
const input = ref('')
const busy = ref(false)
const error = ref<string | null>(null)
const pending = ref<{ id: string, summary: string } | null>(null)
const insights = ref<Insight[]>([])
const bodyEl = ref<HTMLElement | null>(null)

const suggestions = [
  'Berapa biaya ongkir 30 hari terakhir?',
  'Bagaimana status pengiriman saya?',
  'Berapa saldo COD saya?',
]

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) {
    const fields = e.fields ? Object.values(e.fields) : []
    return [e.message, ...fields].filter(Boolean).join(' ')
  }
  return t('common.error')
}

function scrollBottom() {
  nextTick(() => {
    if (bodyEl.value) bodyEl.value.scrollTop = bodyEl.value.scrollHeight
  })
}

async function send(msg?: string) {
  const text = (msg ?? input.value).trim()
  if (!text || busy.value) return
  messages.value.push({ role: 'user', content: text })
  input.value = ''
  busy.value = true
  error.value = null
  try {
    const res = await apiFetch<{ content: string, requires_confirmation?: boolean, pending_action?: { id: string, summary: string } }>(
      '/api/v1/ai/chat', { method: 'POST', body: { message: text } })
    messages.value.push({ role: 'assistant', content: res.content })
    pending.value = res.requires_confirmation && res.pending_action
      ? { id: res.pending_action.id, summary: res.pending_action.summary }
      : null
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
    scrollBottom()
  }
}

async function confirmAction(confirm: boolean) {
  if (!pending.value) return
  busy.value = true
  error.value = null
  try {
    const res = await apiFetch<{ result: string }>(`/api/v1/ai/actions/${pending.value.id}/confirm`, {
      method: 'POST', body: { confirm },
    })
    messages.value.push({ role: 'assistant', content: res.result })
    pending.value = null
  } catch (e) {
    error.value = errText(e)
    pending.value = null
  } finally {
    busy.value = false
    scrollBottom()
  }
}

async function loadInsights() {
  try {
    insights.value = await apiFetch<Insight[]>('/api/v1/ai/insights')
  } catch {
    // Insight gagal: chat tetap berfungsi.
  }
}

onMounted(loadInsights)
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('ai.page_title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('ai.page_subtitle') }}
        </p>
      </div>
    </header>

    <div class="layout">
      <section class="card chat-panel">
        <div
          ref="bodyEl"
          class="chat-body"
        >
          <div
            v-if="messages.length === 0"
            class="chat-empty"
          >
            <p class="muted">
              {{ t('ai.chat_intro') }}
            </p>
            <div class="suggestions">
              <button
                v-for="s in suggestions"
                :key="s"
                type="button"
                class="btn-mini"
                @click="send(s)"
              >
                {{ s }}
              </button>
            </div>
          </div>

          <div
            v-for="(m, i) in messages"
            :key="i"
            class="msg"
            :class="m.role"
          >
            {{ m.content }}
          </div>

          <div
            v-if="pending"
            class="confirm-box"
          >
            <button
              type="button"
              class="btn-mini danger"
              :disabled="busy"
              @click="confirmAction(true)"
            >
              {{ t('ai.confirm_yes') }}
            </button>
            <button
              type="button"
              class="btn-mini"
              @click="confirmAction(false)"
            >
              {{ t('ai.confirm_no') }}
            </button>
          </div>

          <p
            v-if="error"
            class="chat-error"
          >
            {{ error }}
          </p>
        </div>

        <form
          class="chat-input"
          @submit.prevent="send()"
        >
          <input
            v-model="input"
            type="text"
            :placeholder="t('ai.widget_placeholder')"
            :disabled="busy"
          >
          <button
            type="submit"
            class="btn-primary"
            :disabled="busy || !input.trim()"
          >
            {{ t('ai.widget_send') }}
          </button>
        </form>
      </section>

      <aside class="side">
        <section class="card panel">
          <h2 class="panel-title">
            {{ t('dash.insights') }}
          </h2>
          <ul class="insights">
            <li
              v-for="(ins, i) in insights"
              :key="i"
              class="insight"
              :class="ins.level"
            >
              <strong>{{ ins.title }}</strong>
              <span>{{ ins.body }}</span>
            </li>
          </ul>
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: var(--space-5); }
.page-title { font-size: var(--text-xl); margin-bottom: var(--space-1); }
.page-subtitle { margin: 0; color: var(--color-ink-secondary); }

.layout {
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: var(--space-4);
}
@media (max-width: 900px) {
  .layout { grid-template-columns: 1fr; }
}

.chat-panel { display: flex; flex-direction: column; height: 560px; padding: 0; }
.chat-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.chat-empty { text-align: center; padding: var(--space-8) var(--space-4); }
.suggestions { display: flex; flex-direction: column; gap: var(--space-2); margin-top: var(--space-4); align-items: center; }

.msg {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  max-width: 80%;
  white-space: pre-wrap;
}
.msg.user { align-self: flex-end; background: var(--color-accent, #0f766e); color: #fff; }
.msg.assistant { align-self: flex-start; background: var(--color-border); color: var(--color-ink); }

.confirm-box { display: flex; gap: var(--space-2); }
.chat-error { color: #9f1239; font-size: var(--text-xs); }

.chat-input {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3);
  border-top: 1px solid var(--color-border);
}
.chat-input input {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

.btn-primary, .btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
  font: inherit;
}
.btn-primary { background: var(--color-accent, #0f766e); color: #fff; }
.btn-mini { border-color: var(--color-border); background: var(--color-surface); color: var(--color-ink); }
.btn-mini.danger { background: #9f1239; border-color: #9f1239; color: #fff; }
.btn-primary:disabled, .btn-mini:disabled { opacity: 0.55; cursor: not-allowed; }

.side .panel { padding: var(--space-4); }
.panel-title { font-size: var(--text-md); margin: 0 0 var(--space-3); }
.insights { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
.insight { display: flex; flex-direction: column; gap: 2px; padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); font-size: var(--text-sm); }
.insight.warning { border-color: #fde68a; background: #fffbeb; }
.insight span { color: var(--color-ink-secondary); }
.muted { color: var(--color-ink-muted); font-size: var(--text-sm); }
</style>
