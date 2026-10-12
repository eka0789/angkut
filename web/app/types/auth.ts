// Tipe yang mencerminkan kontrak API auth (api/internal/interfaces/http/dto).
// Backend memakai snake_case — jangan diterjemahkan ke camelCase agar tidak
// ada lapisan konversi yang bisa salah saat menambah field baru.

/** Ringkasan pengguna (aman dikirim ke klien; tanpa hash/secret). */
export interface UserSummary {
  id: string
  name: string
  email: string
  status: string
  email_verified: boolean
  two_factor_enabled: boolean
  avatar_url?: string
  locale?: string
  timezone?: string
}

/** Organisasi + role pengguna di dalamnya. */
export interface MembershipSummary {
  id: string
  name: string
  slug: string
  role: string
}

/** Respons sukses login (tanpa 2FA). */
export interface SessionResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
  user: UserSummary
  organizations: MembershipSummary[]
  active_organization_id?: string
  active_role?: string
}

/** Respons login yang tertahan langkah kedua. */
export interface MFARequiredResponse {
  mfa_required: boolean
  mfa_token: string
}

/** Respons pendaftaran. */
export interface RegisterResponse {
  user: UserSummary
  organization: { id: string; name: string; slug: string }
  next_step: string
}

/** Profil organisasi lengkap (GET /organizations/{id}). */
export interface OrganizationDetail {
  id: string
  name: string
  slug: string
  legal_name?: string
  business_type?: string
  phone?: string
  email?: string
  logo_url?: string
  timezone?: string
  currency?: string
  status: string
  your_role: string
}

/** Satu baris daftar anggota. */
export interface MemberResponse {
  id: string
  user_id: string
  name: string
  email: string
  role: string
  status: string
  joined_at?: string
  is_you: boolean
}

/** Hasil penambahan anggota. */
export interface InviteMemberResponse {
  member_id: string
  email: string
  role: string
  already_registered: boolean
}

/** Isi sesi yang disimpan di klien (localStorage). */
export interface StoredSession {
  access_token: string
  refresh_token: string
  expires_at: number
  user: UserSummary
  active_org_id: string
  active_role: string
  organizations: MembershipSummary[]
}