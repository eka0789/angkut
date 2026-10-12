<script setup lang="ts">
// Halaman masuk. Alur: login → (bila 2FA aktif) → /2fa → dasbor.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'auth', middleware: 'guest-only' })

const { t } = useT()
const route = useRoute()
const router = useRouter()
const { login } = useAuth()
const config = useRuntimeConfig()

useHead({ title: () => t('auth.login') })

const email = ref('')
const password = ref('')
const submitting = ref(false)
const errorMessage = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})

async function onSubmit() {
  errorMessage.value = null
  fieldErrors.value = {}
  submitting.value = true
  try {
    const result = await login(email.value, password.value)
    if (result.mfaRequired) {
      // Token MFA sekali pakai; simpan sementara untuk langkah kedua.
      sessionStorage.setItem('angkut_mfa_token', result.mfaToken)
      await router.push('/2fa')
      return
    }
    const redirect = route.query.redirect
    await router.push(typeof redirect === 'string' && redirect.startsWith('/') ? redirect : '/dashboard')
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
        {{ t('auth.login') }}
      </h1>
      <p class="auth-sub">
        {{ t('auth.login_subtitle') }}
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
        <label for="login-email">{{ t('auth.email') }}</label>
        <input
          id="login-email"
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

      <div class="field">
        <div class="field-label-row">
          <label for="login-password">{{ t('auth.password') }}</label>
          <NuxtLink
            to="/forgot-password"
            class="field-link"
          >
            {{ t('auth.forgotPassword') }}
          </NuxtLink>
        </div>
        <input
          id="login-password"
          v-model="password"
          type="password"
          name="password"
          autocomplete="current-password"
          required
          :aria-invalid="Boolean(fieldErrors.password)"
        >
        <p
          v-if="fieldErrors.password"
          class="field-error"
        >
          {{ fieldErrors.password }}
        </p>
      </div>

      <button
        type="submit"
        class="btn-submit"
        :disabled="submitting"
      >
        {{ submitting ? t('common.loading') : t('auth.login') }}
      </button>
    </form>

    <div class="auth-divider">
      <span>{{ t('auth.or') }}</span>
    </div>

    <a
      :href="`${config.public.apiBaseUrl}/api/v1/auth/google`"
      class="btn-google"
    >
      {{ t('auth.continue_with_google') }}
    </a>

    <p class="auth-foot">
      {{ t('auth.no_account') }}
      <NuxtLink to="/register">
        {{ t('auth.register') }}
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

.field-label-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.field-link {
  font-size: var(--text-xs);
  color: var(--color-primary);
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

.auth-divider {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin: var(--space-5) 0;
  color: var(--color-ink-muted);
  font-size: var(--text-xs);
}

.auth-divider::before,
.auth-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--color-border);
}

.btn-google {
  display: block;
  width: 100%;
  padding: var(--space-3);
  text-align: center;
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  text-decoration: none;
}

.btn-google:hover {
  background: var(--color-surface-subtle);
}

.auth-foot {
  margin-top: var(--space-6);
  text-align: center;
  font-size: var(--text-sm);
  color: var(--color-ink-secondary);
}

.auth-foot a {
  color: var(--color-primary);
  font-weight: 600;
}
</style>