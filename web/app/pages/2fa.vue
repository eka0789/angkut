<script setup lang="ts">
// Langkah kedua login: kode TOTP 6 digit atau kode pemulihan.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'auth', middleware: 'guest-only' })

const { t } = useT()
const router = useRouter()
const { verifyMfa } = useAuth()

useHead({ title: () => t('auth.two_factor') })

const code = ref('')
const submitting = ref(false)
const errorMessage = ref<string | null>(null)

// Token MFA sekali pakai dari login; hilang berarti sesi login basi.
const mfaToken = import.meta.client
  ? sessionStorage.getItem('angkut_mfa_token') ?? ''
  : ''

onMounted(() => {
  if (!mfaToken) {
    void router.replace('/login')
  }
})

async function onSubmit() {
  errorMessage.value = null
  if (!mfaToken) return
  submitting.value = true
  try {
    await verifyMfa(mfaToken, code.value.trim())
    sessionStorage.removeItem('angkut_mfa_token')
    await router.push('/dashboard')
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <div class="auth-head">
      <h1 class="auth-title">
        {{ t('auth.two_factor') }}
      </h1>
      <p class="auth-sub">
        {{ t('auth.mfa_subtitle') }}
      </p>
    </div>

    <div
      v-if="errorMessage"
      class="form-alert"
      role="alert"
    >
      {{ errorMessage }}
    </div>

    <form
      class="auth-form"
      novalidate
      @submit.prevent="onSubmit"
    >
      <div class="field">
        <label for="mfa-code">{{ t('auth.mfa_code_label') }}</label>
        <input
          id="mfa-code"
          v-model="code"
          type="text"
          name="one-time-code"
          autocomplete="one-time-code"
          inputmode="text"
          maxlength="12"
          required
          class="code-input"
          :placeholder="t('auth.mfa_code_placeholder')"
        >
        <p class="field-hint">
          {{ t('auth.mfa_recovery_hint') }}
        </p>
      </div>

      <button
        type="submit"
        class="btn-submit"
        :disabled="submitting || !mfaToken"
      >
        {{ submitting ? t('common.loading') : t('common.verify') }}
      </button>
    </form>

    <p class="auth-foot">
      <NuxtLink to="/login">
        {{ t('auth.back_to_login') }}
      </NuxtLink>
    </p>
  </div>
</template>

<style scoped>
.auth-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.auth-sub {
  margin: 0 0 var(--space-6);
  color: var(--color-ink-secondary);
  font-size: var(--text-sm);
}

.form-alert {
  margin-bottom: var(--space-5);
  padding: var(--space-3) var(--space-4);
  background: var(--color-danger-soft);
  color: var(--color-danger);
  border: 1px solid currentColor;
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.auth-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.field label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 600;
  margin-bottom: var(--space-1);
}

.field input {
  width: 100%;
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  background: var(--color-surface);
  color: var(--color-ink);
}

.field input:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
}

.code-input {
  font-family: var(--font-mono, monospace);
  letter-spacing: 0.3em;
}

.field-hint {
  margin: var(--space-1) 0 0;
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.btn-submit {
  margin-top: var(--space-2);
  padding: var(--space-3);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: none;
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  cursor: pointer;
}

.btn-submit:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.btn-submit:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.auth-foot {
  margin-top: var(--space-6);
  text-align: center;
  font-size: var(--text-sm);
}

.auth-foot a {
  color: var(--color-primary);
  font-weight: 600;
}
</style>