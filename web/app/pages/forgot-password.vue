<script setup lang="ts">
// Permintaan tautan reset kata sandi. Respons server selalu sukses agar
// endpoint ini tidak dapat dipakai memetakan email yang terdaftar.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'auth', middleware: 'guest-only' })

const { t } = useT()
const { forgotPassword } = useAuth()

useHead({ title: () => t('auth.forgotPassword') })

const email = ref('')
const submitting = ref(false)
const sent = ref(false)
const errorMessage = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})

async function onSubmit() {
  errorMessage.value = null
  fieldErrors.value = {}
  submitting.value = true
  try {
    await forgotPassword(email.value)
    sent.value = true
  } catch (err: unknown) {
    if (err instanceof ApiRequestError && err.fields) {
      fieldErrors.value = err.fields
      errorMessage.value = t('auth.check_fields')
    } else if (err instanceof ApiRequestError) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('common.error')
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <div class="auth-head">
      <h1 class="auth-title">
        {{ t('auth.forgotPassword') }}
      </h1>
      <p
        v-if="!sent"
        class="auth-sub"
      >
        {{ t('auth.forgot_subtitle') }}
      </p>
    </div>

    <div
      v-if="sent"
      class="state-block state-ok"
      role="status"
    >
      <p>{{ t('auth.email_sent_generic') }}</p>
      <p class="state-sub">
        {{ t('auth.email_sent_hint') }}
      </p>
      <NuxtLink
        to="/login"
        class="btn-secondary"
      >
        {{ t('auth.back_to_login') }}
      </NuxtLink>
    </div>

    <template v-else>
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
          <label for="forgot-email">{{ t('auth.email') }}</label>
          <input
            id="forgot-email"
            v-model="email"
            type="email"
            name="email"
            autocomplete="email"
            required
            :aria-invalid="Boolean(fieldErrors.email)"
          >
          <p
            v-if="fieldErrors.email"
            class="field-error"
          >
            {{ fieldErrors.email }}
          </p>
        </div>

        <button
          type="submit"
          class="btn-submit"
          :disabled="submitting"
        >
          {{ submitting ? t('common.loading') : t('auth.send_reset_link') }}
        </button>
      </form>

      <p class="auth-foot">
        <NuxtLink to="/login">
          {{ t('auth.back_to_login') }}
        </NuxtLink>
      </p>
    </template>
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

.field-error {
  margin: var(--space-1) 0 0;
  font-size: var(--text-xs);
  color: var(--color-danger);
}

.btn-submit {
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

.state-block {
  padding: var(--space-5) var(--space-4);
  background: var(--color-surface-subtle);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  text-align: center;
}

.state-block p {
  margin: 0;
}

.state-ok {
  background: var(--color-success-soft);
  border-color: var(--color-success);
}

.state-sub {
  margin: var(--space-3) 0 var(--space-5);
  color: var(--color-ink-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.btn-secondary {
  display: inline-block;
  padding: var(--space-3) var(--space-5);
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  text-decoration: none;
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