<script setup lang="ts">
// Halaman pendaftaran: akun + organisasi pertama + token verifikasi email.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'auth', middleware: 'guest-only' })

const { t } = useT()
const router = useRouter()
const { register } = useAuth()

useHead({ title: () => t('auth.register') })

const name = ref('')
const email = ref('')
const password = ref('')
const organizationName = ref('')
const submitting = ref(false)
const errorMessage = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})

async function onSubmit() {
  errorMessage.value = null
  fieldErrors.value = {}
  submitting.value = true
  try {
    await register({
      name: name.value,
      email: email.value,
      password: password.value,
      organizationName: organizationName.value,
    })
    // Email verifikasi dikirim server-side; simpan email untuk halaman
    // berikutnya (resend + tampilan status).
    sessionStorage.setItem('angkut_pending_email', email.value)
    await router.push({ path: '/verify-email', query: { sent: '1' } })
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
        {{ t('auth.register') }}
      </h1>
      <p class="auth-sub">
        {{ t('auth.register_subtitle') }}
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
        <label for="reg-name">{{ t('auth.name') }}</label>
        <input
          id="reg-name"
          v-model="name"
          type="text"
          name="name"
          autocomplete="name"
          required
          :aria-invalid="Boolean(fieldErrors.name)"
        >
        <p
          v-if="fieldErrors.name"
          class="field-error"
        >
          {{ fieldErrors.name }}
        </p>
      </div>

      <div class="field">
        <label for="reg-email">{{ t('auth.email') }}</label>
        <input
          id="reg-email"
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
        <label for="reg-password">{{ t('auth.password') }}</label>
        <input
          id="reg-password"
          v-model="password"
          type="password"
          name="password"
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
        <label for="reg-org">{{ t('auth.organization_name') }}</label>
        <input
          id="reg-org"
          v-model="organizationName"
          type="text"
          name="organization"
          required
          :aria-invalid="Boolean(fieldErrors.organization_name)"
        >
        <p
          v-if="fieldErrors.organization_name"
          class="field-error"
        >
          {{ fieldErrors.organization_name }}
        </p>
        <p class="field-hint">
          {{ t('auth.organization_hint') }}
        </p>
      </div>

      <button
        type="submit"
        class="btn-submit"
        :disabled="submitting"
      >
        {{ submitting ? t('common.loading') : t('auth.create_account') }}
      </button>
    </form>

    <p class="auth-foot">
      {{ t('auth.already_account') }}
      <NuxtLink to="/login">
        {{ t('auth.login') }}
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