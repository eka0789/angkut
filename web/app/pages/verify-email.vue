<script setup lang="ts">
// Verifikasi email. Dua mode: (a) tautan dari email → auto-verifikasi dengan
// ?token=…; (b) sesudah pendaftaran → tampilan "cek email" + kirim ulang.
import { ApiRequestError } from '~/composables/useApi'

definePageMeta({ layout: 'auth', middleware: 'guest-only' })

const { t } = useT()
const route = useRoute()
const router = useRouter()
const { verifyEmail, resendVerification } = useAuth()

useHead({ title: () => t('auth.verify_email_title') })

type Phase = 'checking' | 'success' | 'failed' | 'sent'
const phase = ref<Phase>(typeof route.query.token === 'string' ? 'checking' : 'sent')
const errorMessage = ref<string | null>(null)
const resending = ref(false)
const resendDone = ref(false)

const pendingEmail = computed(() =>
  import.meta.client ? sessionStorage.getItem('angkut_pending_email') ?? '' : '',
)

// Tautan dari email: verifikasi begitu halaman termuat.
onMounted(async () => {
  const token = route.query.token
  if (typeof token !== 'string') {
    // Kunjungan langsung tanpa tautan & tanpa sesi pendaftaran → bukan
    // tempat yang berguna; kembali ke pendaftaran.
    if (!sessionStorage.getItem('angkut_pending_email')) {
      await router.replace('/register')
    }
    return
  }
  try {
    await verifyEmail(token)
    phase.value = 'success'
    // Beri waktu membaca pesan, lalu ke halaman masuk.
    setTimeout(() => { void router.push('/login') }, 2500)
  } catch (err: unknown) {
    phase.value = 'failed'
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
})

async function onResend() {
  if (!pendingEmail.value) return
  resending.value = true
  errorMessage.value = null
  try {
    await resendVerification(pendingEmail.value)
    resendDone.value = true
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    resending.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <div class="auth-head">
      <h1 class="auth-title">
        {{ t('auth.verify_email_title') }}
      </h1>
    </div>

    <div
      v-if="phase === 'checking'"
      class="state-block"
      role="status"
    >
      <p>{{ t('auth.verifying') }}</p>
    </div>

    <div
      v-else-if="phase === 'success'"
      class="state-block state-ok"
      role="status"
    >
      <p>{{ t('auth.verify_success') }}</p>
      <p class="state-sub">
        {{ t('auth.redirecting_to_login') }}
      </p>
    </div>

    <template v-else-if="phase === 'failed'">
      <div
        class="form-alert"
        role="alert"
      >
        {{ errorMessage }}
      </div>
      <p class="state-sub">
        {{ t('auth.verify_failed_hint') }}
      </p>
      <div class="actions">
        <NuxtLink
          to="/login"
          class="btn-secondary"
        >
          {{ t('auth.login') }}
        </NuxtLink>
        <NuxtLink
          to="/register"
          class="btn-secondary"
        >
          {{ t('auth.register') }}
        </NuxtLink>
      </div>
    </template>

    <template v-else>
      <div
        v-if="resendDone"
        class="state-block state-ok"
        role="status"
      >
        <p>{{ t('auth.resend_done') }}</p>
      </div>

      <div
        v-if="errorMessage"
        class="form-alert"
        role="alert"
      >
        {{ errorMessage }}
      </div>

      <p class="state-sub">
        {{ t('auth.check_email_sent') }}
        <strong v-if="pendingEmail">{{ pendingEmail }}</strong>
      </p>
      <p class="state-sub">
        {{ t('auth.verify_hint') }}
      </p>

      <div class="actions">
        <button
          v-if="pendingEmail && !resendDone"
          type="button"
          class="btn-primary"
          :disabled="resending"
          @click="onResend"
        >
          {{ resending ? t('common.loading') : t('auth.resend') }}
        </button>
        <NuxtLink
          to="/login"
          class="btn-secondary"
        >
          {{ t('auth.login') }}
        </NuxtLink>
      </div>
    </template>
  </div>
</template>

<style scoped>
.auth-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-4);
}

.state-block {
  padding: var(--space-4);
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
  margin: var(--space-3) 0 0;
  color: var(--color-ink-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
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

.actions {
  display: flex;
  gap: var(--space-3);
  margin-top: var(--space-6);
}

.btn-primary {
  padding: var(--space-3) var(--space-5);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: none;
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  cursor: pointer;
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-secondary {
  padding: var(--space-3) var(--space-5);
  background: var(--color-surface);
  color: var(--color-ink);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  text-decoration: none;
}
</style>