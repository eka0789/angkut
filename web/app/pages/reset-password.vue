<script setup lang="ts">
// Atur ulang kata sandi dengan token sekali pakai dari email.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'auth', middleware: 'guest-only' })

const { t } = useT()
const route = useRoute()
const router = useRouter()
const { resetPassword } = useAuth()

useHead({ title: () => t('auth.resetPassword') })

const token = computed(() => typeof route.query.token === 'string' ? route.query.token : '')
const password = ref('')
const passwordConfirm = ref('')
const submitting = ref(false)
const errorMessage = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})

async function onSubmit() {
  errorMessage.value = null
  fieldErrors.value = {}

  if (!token.value) {
    errorMessage.value = t('auth.invalid_token')
    return
  }
  if (password.value !== passwordConfirm.value) {
    fieldErrors.value = { confirm: t('auth.passwords_do_not_match') }
    errorMessage.value = t('auth.passwords_do_not_match')
    return
  }

  submitting.value = true
  try {
    await resetPassword(token.value, password.value)
    await router.push({ path: '/login', query: { reset: '1' } })
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
        {{ t('auth.resetPassword') }}
      </h1>
      <p class="auth-sub">
        {{ t('auth.reset_subtitle') }}
      </p>
    </div>

    <div
      v-if="errorMessage"
      class="form-alert"
      role="alert"
    >
      {{ errorMessage }}
    </div>

    <div
      v-if="!token"
      class="state-block"
      role="alert"
    >
      <p>{{ t('auth.invalid_token') }}</p>
      <NuxtLink
        to="/forgot-password"
        class="btn-secondary"
      >
        {{ t('auth.forgotPassword') }}
      </NuxtLink>
    </div>

    <form
      v-else
      class="auth-form"
      novalidate
      @submit.prevent="onSubmit"
    >
      <div class="field">
        <label for="reset-password">{{ t('auth.newPassword') }}</label>
        <input
          id="reset-password"
          v-model="password"
          type="password"
          name="new-password"
          autocomplete="new-password"
          minlength="8"
          required
          :aria-invalid="Boolean(fieldErrors.password)"
        >
        <p
          v-if="fieldErrors.password"
          class="field-error"
        >
          {{ fieldErrors.password }}
        </p>
        <p class="field-hint">
          {{ t('auth.password_hint') }}
        </p>
      </div>

      <div class="field">
        <label for="reset-confirm">{{ t('auth.confirm_password') }}</label>
        <input
          id="reset-confirm"
          v-model="passwordConfirm"
          type="password"
          name="confirm-password"
          autocomplete="new-password"
          minlength="8"
          required
          :aria-invalid="Boolean(fieldErrors.confirm)"
        >
        <p
          v-if="fieldErrors.confirm"
          class="field-error"
        >
          {{ fieldErrors.confirm }}
        </p>
      </div>

      <button
        type="submit"
        class="btn-submit"
        :disabled="submitting"
      >
        {{ submitting ? t('common.loading') : t('auth.save_new_password') }}
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

.field input[aria-invalid='true'] {
  border-color: var(--color-danger);
}

.field-error {
  margin: var(--space-1) 0 0;
  font-size: var(--text-xs);
  color: var(--color-danger);
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

.state-block {
  padding: var(--space-5) var(--space-4);
  background: var(--color-surface-subtle);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  text-align: center;
}

.state-block p {
  margin: 0 0 var(--space-4);
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