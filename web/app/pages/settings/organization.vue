<script setup lang="ts">
// Pengaturan organisasi: profil + daftar anggota (undang / ubah role / hapus).
// Sumber kebenaran ada di server; halaman ini hanya membaca/menulis lewat API
// dengan tenant aktif dari sesi (header X-Organization-Id).
import { ApiRequestError } from '~/composables/useApi'
import type { MemberResponse, OrganizationDetail } from '~/types/auth'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const route = useRoute()
const { session, isAuthenticated } = useAuth()
const { apiFetch } = useApi()

useHead({ title: () => `${t('nav.settings')} · ${t('settings.organization_title')}` })

const orgId = computed(() => route.params.id as string || session.value?.active_org_id || '')

const org = ref<OrganizationDetail | null>(null)
const members = ref<MemberResponse[]>([])
const loading = ref(true)
const loadError = ref<string | null>(null)
const saving = ref(false)
const saveMessage = ref<string | null>(null)
const errorMessage = ref<string | null>(null)

// Form profil — diisi setelah muat.
const form = ref({
  name: '',
  legal_name: '',
  business_type: '',
  phone: '',
  timezone: 'Asia/Jakarta',
})

// Form undang anggota.
const inviteForm = ref({ email: '', role: 'staff' })
const inviting = ref(false)
const inviteMessage = ref<string | null>(null)

async function loadAll() {
  loading.value = true
  loadError.value = null
  try {
    const [orgRes, memberRes] = await Promise.all([
      apiFetch<OrganizationDetail>(`/api/v1/organizations/${orgId.value}`),
      apiFetch<MemberResponse[]>(`/api/v1/organizations/${orgId.value}/members`),
    ])
    org.value = orgRes
    members.value = memberRes
    form.value = {
      name: orgRes.name,
      legal_name: orgRes.legal_name ?? '',
      business_type: orgRes.business_type ?? '',
      phone: orgRes.phone ?? '',
      timezone: orgRes.timezone ?? 'Asia/Jakarta',
    }
  } catch (err: unknown) {
    loadError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (isAuthenticated.value && orgId.value) void loadAll()
})

const canManage = computed(() => {
  const role = org.value?.your_role ?? session.value?.active_role
  return role === 'owner' || role === 'admin'
})

async function saveProfile() {
  saving.value = true
  errorMessage.value = null
  saveMessage.value = null
  try {
    const updated = await apiFetch<OrganizationDetail>(
      `/api/v1/organizations/${orgId.value}`,
      {
        method: 'PATCH',
        body: {
          name: form.value.name,
          legal_name: form.value.legal_name,
          business_type: form.value.business_type,
          phone: form.value.phone,
          timezone: form.value.timezone,
        },
      },
    )
    org.value = updated
    saveMessage.value = t('settings.saved')
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError
      ? (err.fields ? Object.values(err.fields).join(' ') : err.message)
      : t('common.error')
  } finally {
    saving.value = false
  }
}

async function inviteMember() {
  if (!inviteForm.value.email) return
  inviting.value = true
  inviteMessage.value = null
  errorMessage.value = null
  try {
    const res = await apiFetch<{ member_id: string, email: string, role: string, already_registered: boolean }>(
      `/api/v1/organizations/${orgId.value}/members`,
      { method: 'POST', body: { email: inviteForm.value.email, role: inviteForm.value.role } },
    )
    inviteMessage.value = res.already_registered
      ? t('settings.invite_existing', { email: res.email })
      : t('settings.invited', { email: res.email })
    inviteForm.value = { email: '', role: 'staff' }
    await loadAll()
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError
      ? (err.fields ? Object.values(err.fields).join(' ') : err.message)
      : t('common.error')
  } finally {
    inviting.value = false
  }
}

async function changeRole(member: MemberResponse, role: string) {
  errorMessage.value = null
  try {
    await apiFetch<null>(
      `/api/v1/organizations/${orgId.value}/members/${member.id}`,
      { method: 'PATCH', body: { role } },
    )
    await loadAll()
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

async function removeMember(member: MemberResponse) {
  if (!confirm(t('settings.confirm_remove', { name: member.name || member.email }))) return
  errorMessage.value = null
  try {
    await apiFetch<null>(
      `/api/v1/organizations/${orgId.value}/members/${member.id}`,
      { method: 'DELETE' },
    )
    await loadAll()
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

/** Peta role → label Indonesia (ringkas; role kanonik ada di backend). */
const roleLabels: Record<string, string> = {
  owner: 'Pemilik',
  admin: 'Admin',
  finance: 'Keuangan',
  warehouse: 'Gudang',
  staff: 'Staf',
  viewer: 'Peninjau',
}
</script>

<template>
  <div class="settings">
    <header class="page-head">
      <h1 class="page-title">
        {{ t('settings.organization_title') }}
      </h1>
      <p class="page-subtitle">
        {{ t('settings.organization_subtitle') }}
      </p>
    </header>

    <div
      v-if="loadError"
      class="form-alert"
      role="alert"
    >
      {{ loadError }}
    </div>

    <div
      v-else-if="loading"
      class="state-block"
      role="status"
    >
      {{ t('common.loading') }}
    </div>

    <template v-else>
      <!-- Profil organisasi -->
      <section class="card section">
        <h2 class="section-title">
          {{ t('settings.profile') }}
        </h2>

        <div
          v-if="saveMessage"
          class="state-ok"
          role="status"
        >
          {{ saveMessage }}
        </div>
        <div
          v-if="errorMessage"
          class="form-alert"
          role="alert"
        >
          {{ errorMessage }}
        </div>

        <form
          class="form-grid"
          novalidate
          @submit.prevent="saveProfile"
        >
          <div class="field">
            <label for="org-name">{{ t('settings.field_name') }}</label>
            <input
              id="org-name"
              v-model="form.name"
              type="text"
              required
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="org-legal">{{ t('settings.field_legal_name') }}</label>
            <input
              id="org-legal"
              v-model="form.legal_name"
              type="text"
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="org-biz">{{ t('settings.field_business_type') }}</label>
            <input
              id="org-biz"
              v-model="form.business_type"
              type="text"
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="org-phone">{{ t('settings.field_phone') }}</label>
            <input
              id="org-phone"
              v-model="form.phone"
              type="tel"
              :disabled="!canManage"
            >
          </div>
          <div class="field">
            <label for="org-tz">{{ t('settings.field_timezone') }}</label>
            <select
              id="org-tz"
              v-model="form.timezone"
              :disabled="!canManage"
            >
              <option value="Asia/Jakarta">
                Asia/Jakarta (WIB)
              </option>
              <option value="Asia/Makassar">
                Asia/Makassar (WITA)
              </option>
              <option value="Asia/Jayapura">
                Asia/Jayapura (WIT)
              </option>
            </select>
          </div>

          <div
            v-if="canManage"
            class="form-actions"
          >
            <button
              type="submit"
              class="btn-primary"
              :disabled="saving"
            >
              {{ saving ? t('common.loading') : t('common.save') }}
            </button>
          </div>
          <p
            v-else
            class="hint"
          >
            {{ t('settings.read_only') }}
          </p>
        </form>
      </section>

      <!-- Anggota -->
      <section class="card section">
        <h2 class="section-title">
          {{ t('settings.members') }}
        </h2>

        <form
          v-if="canManage"
          class="invite-row"
          novalidate
          @submit.prevent="inviteMember"
        >
          <div class="field invite-field">
            <label for="invite-email">{{ t('settings.invite_email') }}</label>
            <input
              id="invite-email"
              v-model="inviteForm.email"
              type="email"
              required
              :placeholder="t('settings.invite_placeholder')"
            >
          </div>
          <div class="field invite-field">
            <label for="invite-role">{{ t('settings.invite_role') }}</label>
            <select
              id="invite-role"
              v-model="inviteForm.role"
            >
              <option
                v-for="(label, value) in roleLabels"
                :key="value"
                :value="value"
              >
                {{ label }}
              </option>
            </select>
          </div>
          <button
            type="submit"
            class="btn-primary invite-btn"
            :disabled="inviting"
          >
            {{ inviting ? t('common.loading') : t('settings.invite') }}
          </button>
        </form>

        <p
          v-if="inviteMessage"
          class="state-ok"
          role="status"
        >
          {{ inviteMessage }}
        </p>

        <p
          v-if="members.length === 0"
          class="hint"
        >
          {{ t('settings.no_members') }}
        </p>
        <ul
          v-else
          class="member-list"
        >
          <li
            v-for="member in members"
            :key="member.id"
            class="member-row"
          >
            <div class="member-info">
              <span class="member-name">
                {{ member.name || member.email }}
                <span
                  v-if="member.is_you"
                  class="badge-you"
                >{{ t('settings.you') }}</span>
              </span>
              <span class="member-email">{{ member.email }}</span>
            </div>
            <div class="member-actions">
              <select
                v-if="canManage && !member.is_you"
                :value="member.role"
                :aria-label="`${t('settings.role_for')} ${member.email}`"
                @change="changeRole(member, ($event.target as HTMLSelectElement).value)"
              >
                <option
                  v-for="(label, value) in roleLabels"
                  :key="value"
                  :value="value"
                >
                  {{ label }}
                </option>
              </select>
              <span
                v-else
                class="member-role"
              >{{ roleLabels[member.role] ?? member.role }}</span>
              <button
                v-if="canManage && !member.is_you"
                type="button"
                class="btn-danger"
                @click="removeMember(member)"
              >
                {{ t('common.remove') }}
              </button>
            </div>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: var(--space-6);
}

.page-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.page-subtitle {
  margin: 0;
  color: var(--color-ink-secondary);
}

.section + .section {
  margin-top: var(--space-6);
}

.section {
  padding: var(--space-6);
}

.section-title {
  font-size: var(--text-md);
  margin: 0 0 var(--space-5);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-4);
}

.form-grid .form-actions {
  grid-column: 1 / -1;
}

.field label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 600;
  margin-bottom: var(--space-1);
}

.field input,
.field select {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  background: var(--color-surface);
  color: var(--color-ink);
}

.field input:disabled,
.field select:disabled {
  background: var(--color-surface-subtle);
  color: var(--color-ink-muted);
}

.form-actions {
  margin-top: var(--space-2);
}

.btn-primary {
  padding: var(--space-2) var(--space-5);
  background: var(--color-primary);
  color: var(--color-ink-inverse);
  border: none;
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-sm);
  cursor: pointer;
}

.btn-primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.invite-row {
  display: flex;
  gap: var(--space-3);
  align-items: flex-end;
  flex-wrap: wrap;
  margin-bottom: var(--space-4);
}

.invite-field {
  flex: 1;
  min-width: 180px;
}

.invite-btn {
  height: 38px;
}

.member-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.member-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--color-border);
  flex-wrap: wrap;
}

.member-row:last-child {
  border-bottom: none;
}

.member-info {
  display: flex;
  flex-direction: column;
}

.member-name {
  font-weight: 600;
  font-size: var(--text-sm);
}

.member-email {
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}

.member-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.member-actions select {
  padding: var(--space-1) var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  background: var(--color-surface);
}

.member-role {
  font-size: var(--text-xs);
  color: var(--color-ink-secondary);
}

.btn-danger {
  padding: var(--space-1) var(--space-3);
  background: transparent;
  color: var(--color-danger);
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  cursor: pointer;
}

.badge-you {
  margin-left: var(--space-2);
  padding: 0 var(--space-2);
  font-size: 10px;
  font-weight: 700;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  border-radius: var(--radius-pill);
  vertical-align: middle;
}

.form-alert {
  margin-bottom: var(--space-4);
  padding: var(--space-3) var(--space-4);
  background: var(--color-danger-soft);
  color: var(--color-danger);
  border: 1px solid currentColor;
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.state-ok {
  margin-bottom: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--color-success-soft);
  color: var(--color-success);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.state-block {
  padding: var(--space-8);
  text-align: center;
  color: var(--color-ink-secondary);
}

.hint {
  margin: var(--space-2) 0 0;
  font-size: var(--text-xs);
  color: var(--color-ink-muted);
}
</style>