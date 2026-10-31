<script setup lang="ts">
// Widget AI mengambang (TASK-056): chat heuristik dengan guardrail — aksi
// mutasi selalu butuh konfirmasi eksplisit (TASK-050).
import { ApiRequestError } from '~/composables/useApi'

const { t } = useT()
const { apiFetch } = useApi()

const open = ref(false)
const input = ref('')
const busy = ref(false)
const error = ref<string | null>(null)
const pending = ref<{ id: string, summary: string } | null>(null)
const messages = ref<Array<{ role: 'user' | 'assistant', content: string }>>([])

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) {
    const fields = e.fields ? Object.values(e.fields) : []
    return [e.message, ...fields].filter(Boolean).join(' ')
  }
  return t('common.error')
}

async function send() {
  const msg = input.value.trim()
  if (!msg || busy.value) return
  messages.value.push({ role: 'user', content: msg })
  input.value = ''
  busy.value = true
  error.value = null
  try {
    const res = await apiFetch<{ content: string, requires_confirmation?: boolean, pending_action?: { id: string, summary: string } }>(
      '/api/v1/ai/chat', { method: 'POST', body: { message: msg } })
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

function scrollBottom() {
  nextTick(() => {
    const el = document.querySelector('.ai-body')
    if (el) el.scrollTop = el.scrollHeight
  })
}

// Muat ulang riwayat saat pindah halaman tidak perlu — sesi chat per buka.
</script>

<template>
  <div class="ai-widget">
    <button
      type="button"
      class="ai-fab"
      :aria-label="t('ai.widget_title')"
      @click="open = !open"
    >
      <AppIcon name="sparkles" :size="24" />
    </button>

    <div
      v-if="open"
      class="ai-panel"
      role="dialog"
      :aria-label="t('ai.widget_title')"
    >
      <div class="ai-head">
        <strong>{{ t('ai.widget_title') }}</strong>
        <button
          type="button"
          class="ai-close"
          aria-label="Tutup"
          @click="open = false"
        >
          ×
        </button>
      </div>
      <div
        ref="aiBody"
        class="ai-body"
      >
        <p
          v-if="messages.length === 0"
          class="muted"
        >
          {{ t('ai.widget_placeholder') }}
        </p>
        <div
          v-for="(m, i) in messages"
          :key="i"
          class="ai-msg"
          :class="m.role"
        >
          {{ m.content }}
        </div>
        <div
          v-if="pending"
          class="ai-confirm"
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
          class="ai-error"
        >
          {{ error }}
        </p>
      </div>
      <form
        class="ai-input"
        @submit.prevent="send"
      >
        <input
          v-model="input"
          type="text"
          :placeholder="t('ai.widget_placeholder')"
          :disabled="busy"
        >
        <button
          type="submit"
          class="btn-mini"
          :disabled="busy || !input.trim()"
        >
          {{ t('ai.widget_send') }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.ai-widget { position: fixed; right: var(--space-5); bottom: var(--space-5); z-index: 70; }

.ai-fab {
  width: 52px;
  height: 52px;
  border-radius: 999px;
  border: none;
  background: var(--color-cta, #f97316);
  color: #fff;
  font-size: 22px;
  cursor: pointer;
  box-shadow: 0 6px 18px rgba(0,0,0,0.2);
}

.ai-panel {
  position: absolute;
  bottom: 64px;
  right: 0;
  width: 340px;
  max-width: 92vw;
  height: 420px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 10px 30px rgba(0,0,0,0.18);
}

.ai-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-3);
  border-bottom: 1px solid var(--color-border);
}
.ai-close { border: none; background: none; font-size: 18px; cursor: pointer; }

.ai-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ai-msg {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  max-width: 85%;
  white-space: pre-wrap;
}
.ai-msg.user { align-self: flex-end; background: var(--color-cta, #f97316); color: #fff; }
.ai-msg.assistant { align-self: flex-start; background: var(--color-border); color: var(--color-ink); }

.ai-confirm { display: flex; gap: var(--space-2); }
.ai-error { color: #9f1239; font-size: var(--text-xs); }
.muted { color: var(--color-ink-muted); font-size: var(--text-sm); }

.ai-input {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3);
  border-top: 1px solid var(--color-border);
}
.ai-input input {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}
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
.btn-mini.danger { background: #9f1239; border-color: #9f1239; color: #fff; }
.btn-mini:disabled { opacity: 0.55; cursor: not-allowed; }
</style>
