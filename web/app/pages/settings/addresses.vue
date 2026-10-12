<script setup lang="ts">
// Daftar alamat tenant: cari, buat, ubah, hapus. Region cascade publik.
import { ApiRequestError } from '~/composables/useApi'
import type { Address, Province, City, District, Village, PostalCode } from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { list, search, create, update, remove, validate } = useAddresses()
const { listProvinces, listCities, listDistricts, listVillages, listPostalCodes } = useRegions()

useHead({ title: () => `${t('nav.settings')} · Alamat` })

const items = ref<Address[]>([])
const loading = ref(true)
const loadError = ref<string | null>(null)
const query = ref('')
const saving = ref(false)
const errorMessage = ref<string | null>(null)
const editing = ref<Address | null>(null)

const provinces = ref<Province[]>([])
const cities = ref<City[]>([])
const districts = ref<District[]>([])
const villages = ref<Village[]>([])
const postalCodes = ref<PostalCode[]>([])

const form = ref({
  recipient: '',
  phone: '',
  province_id: '',
  city_id: '',
  district_id: '',
  village_id: '',
  postal_code: '',
  street: '',
  notes: '',
  latitude: '',
  longitude: '',
  label: '',
  is_default: false,
})

async function loadList() {
  loading.value = true
  loadError.value = null
  try {
    items.value = query.value.trim()
      ? await search(query.value.trim())
      : await list('active')
  } catch (err: unknown) {
    loadError.value = err instanceof ApiRequestError ? err.message : t('common.error')
  } finally {
    loading.value = false
  }
}

async function loadProvinces() {
  try {
    provinces.value = await listProvinces()
  } catch {
    provinces.value = []
  }
}

watch(() => form.value.province_id, async (id) => {
  form.value.city_id = ''
  form.value.district_id = ''
  form.value.village_id = ''
  form.value.postal_code = ''
  cities.value = []
  districts.value = []
  villages.value = []
  postalCodes.value = []
  if (!id) return
  try { cities.value = await listCities(id) } catch { cities.value = [] }
})

watch(() => form.value.city_id, async (id) => {
  form.value.district_id = ''
  form.value.village_id = ''
  form.value.postal_code = ''
  districts.value = []
  villages.value = []
  postalCodes.value = []
  if (!id) return
  try { districts.value = await listDistricts(id) } catch { districts.value = [] }
})

watch(() => form.value.district_id, async (id) => {
  form.value.village_id = ''
  form.value.postal_code = ''
  villages.value = []
  postalCodes.value = []
  if (!id) return
  try {
    const [v, p] = await Promise.all([listVillages(id), listPostalCodes(id)])
    villages.value = v
    postalCodes.value = p
  } catch {
    villages.value = []
    postalCodes.value = []
  }
})

function resetForm() {
  editing.value = null
  form.value = {
    recipient: '',
    phone: '',
    province_id: '',
    city_id: '',
    district_id: '',
    village_id: '',
    postal_code: '',
    street: '',
    notes: '',
    latitude: '',
    longitude: '',
    label: '',
    is_default: false,
  }
  cities.value = []
  districts.value = []
  villages.value = []
  postalCodes.value = []
}

function startEdit(item: Address) {
  editing.value = item
  form.value = {
    recipient: item.recipient,
    phone: item.phone,
    province_id: item.province_id,
    city_id: item.city_id,
    district_id: item.district_id,
    village_id: item.village_id ?? '',
    postal_code: item.postal_code ?? '',
    street: item.street,
    notes: item.notes ?? '',
    latitude: item.latitude != null ? String(item.latitude) : '',
    longitude: item.longitude != null ? String(item.longitude) : '',
    label: item.label ?? '',
    is_default: item.is_default,
  }
}

function payload() {
  const lat = form.value.latitude.trim()
  const lng = form.value.longitude.trim()
  return {
    recipient: form.value.recipient.trim(),
    phone: form.value.phone.trim(),
    province_id: form.value.province_id,
    city_id: form.value.city_id,
    district_id: form.value.district_id,
    village_id: form.value.village_id || undefined,
    postal_code: form.value.postal_code.trim() || undefined,
    street: form.value.street.trim(),
    notes: form.value.notes.trim() || undefined,
    latitude: lat ? Number(lat) : undefined,
    longitude: lng ? Number(lng) : undefined,
    label: form.value.label.trim() || undefined,
    is_default: form.value.is_default,
  }
}

async function save() {
  saving.value = true
  errorMessage.value = null
  try {
    const body = payload()
    const check = await validate({
      province_id: body.province_id,
      city_id: body.city_id,
      district_id: body.district_id,
      village_id: body.village_id,
      postal_code: body.postal_code,
    })
    if (!check.valid) {
      errorMessage.value = check.issues.join(' ')
      return
    }
    if (editing.value) await update(editing.value.id, body)
    else await create(body)
    resetForm()
    await loadList()
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError
      ? (err.fields ? Object.values(err.fields).join(' ') : err.message)
      : t('common.error')
  } finally {
    saving.value = false
  }
}

async function drop(item: Address) {
  if (!confirm(`Hapus alamat ${item.recipient}?`)) return
  errorMessage.value = null
  try {
    await remove(item.id)
    if (editing.value?.id === item.id) resetForm()
    await loadList()
  } catch (err: unknown) {
    errorMessage.value = err instanceof ApiRequestError ? err.message : t('common.error')
  }
}

onMounted(() => {
  void loadList()
  void loadProvinces()
})
</script>

<template>
  <div class="addresses">
    <header class="page-head">
      <h1 class="page-title">
        Alamat
      </h1>
      <p class="page-subtitle">
        Buku alamat organisasi aktif. Wilayah dipilih dari data referensi.
      </p>
    </header>

    <p
      v-if="errorMessage"
      class="banner"
    >
      {{ errorMessage }}
    </p>

    <div class="layout">
      <section class="card panel">
        <div class="toolbar">
          <input
            v-model="query"
            type="search"
            placeholder="Cari penerima, jalan, atau label"
            @keyup.enter="loadList"
          >
          <button
            type="button"
            class="btn-secondary"
            @click="loadList"
          >
            Cari
          </button>
        </div>

        <p
          v-if="loading"
          class="muted"
        >
          Memuat…
        </p>
        <p
          v-else-if="loadError"
          class="banner"
        >
          {{ loadError }}
        </p>
        <p
          v-else-if="items.length === 0"
          class="muted"
        >
          Belum ada alamat.
        </p>
        <ul
          v-else
          class="list"
        >
          <li
            v-for="item in items"
            :key="item.id"
            class="row"
          >
            <div>
              <strong>{{ item.recipient }}</strong>
              <span
                v-if="item.is_default"
                class="badge"
              >default</span>
              <p class="muted">
                {{ item.phone }} · {{ item.street }}
              </p>
            </div>
            <div class="row-actions">
              <button
                type="button"
                class="btn-secondary"
                @click="startEdit(item)"
              >
                Ubah
              </button>
              <button
                type="button"
                class="btn-secondary"
                @click="drop(item)"
              >
                Hapus
              </button>
            </div>
          </li>
        </ul>
      </section>

      <section class="card panel">
        <h2>{{ editing ? 'Ubah alamat' : 'Alamat baru' }}</h2>
        <form
          class="form"
          @submit.prevent="save"
        >
          <label>Penerima <input
            v-model="form.recipient"
            required
          ></label>
          <label>Telepon <input
            v-model="form.phone"
            required
          ></label>
          <label>Label <input v-model="form.label"></label>
          <label>
            Provinsi
            <select
              v-model="form.province_id"
              required
            >
              <option value="">
                Pilih
              </option>
              <option
                v-for="p in provinces"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }}
              </option>
            </select>
          </label>
          <label>
            Kota
            <select
              v-model="form.city_id"
              required
              :disabled="!form.province_id"
            >
              <option value="">
                Pilih
              </option>
              <option
                v-for="c in cities"
                :key="c.id"
                :value="c.id"
              >
                {{ c.name }}
              </option>
            </select>
          </label>
          <label>
            Kecamatan
            <select
              v-model="form.district_id"
              required
              :disabled="!form.city_id"
            >
              <option value="">
                Pilih
              </option>
              <option
                v-for="d in districts"
                :key="d.id"
                :value="d.id"
              >
                {{ d.name }}
              </option>
            </select>
          </label>
          <label>
            Desa/Kelurahan
            <select
              v-model="form.village_id"
              :disabled="!form.district_id"
            >
              <option value="">
                Opsional
              </option>
              <option
                v-for="v in villages"
                :key="v.id"
                :value="v.id"
              >
                {{ v.name }}
              </option>
            </select>
          </label>
          <label>
            Kode pos
            <input
              v-model="form.postal_code"
              list="postal-codes"
            >
            <datalist id="postal-codes">
              <option
                v-for="p in postalCodes"
                :key="p.code"
                :value="p.code"
              />
            </datalist>
          </label>
          <label>Jalan <textarea
            v-model="form.street"
            required
            rows="2"
          /></label>
          <label>Catatan <textarea
            v-model="form.notes"
            rows="2"
          /></label>
          <div class="pin">
            <label>Latitude <input
              v-model="form.latitude"
              inputmode="decimal"
              placeholder="-6.2"
            ></label>
            <label>Longitude <input
              v-model="form.longitude"
              inputmode="decimal"
              placeholder="106.8"
            ></label>
          </div>
          <label class="check">
            <input
              v-model="form.is_default"
              type="checkbox"
            >
            Jadikan default
          </label>
          <div class="form-actions">
            <button
              type="submit"
              class="btn-primary"
              :disabled="saving"
            >
              {{ saving ? 'Menyimpan…' : 'Simpan' }}
            </button>
            <button
              type="button"
              class="btn-secondary"
              @click="resetForm"
            >
              Batal
            </button>
          </div>
        </form>
      </section>
    </div>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: var(--space-6); }
.page-title { font-size: var(--text-xl); margin-bottom: var(--space-1); }
.page-subtitle { margin: 0; color: var(--color-ink-secondary); }
.layout { display: grid; grid-template-columns: 1.1fr 0.9fr; gap: var(--space-4); align-items: start; }
.panel { padding: var(--space-4); }
.toolbar, .form, .pin, .form-actions, .row, .row-actions { display: flex; gap: var(--space-2); }
.toolbar { margin-bottom: var(--space-4); }
.form { flex-direction: column; margin-top: var(--space-4); }
.form label { display: flex; flex-direction: column; gap: var(--space-1); font-weight: 600; }
.form input, .form select, .form textarea {
  font: inherit;
  font-weight: 400;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
}
.list { list-style: none; margin: 0; padding: 0; }
.row { justify-content: space-between; align-items: center; padding: var(--space-3) 0; border-top: 1px solid var(--color-border); }
.row p { margin: var(--space-1) 0 0; }
.badge { margin-left: var(--space-2); color: var(--color-primary); font-size: var(--text-xs); }
.check { flex-direction: row; align-items: center; }
.banner { color: #9f1239; background: #fff1f2; border: 1px solid #fecdd3; border-radius: var(--radius-md); padding: var(--space-3); }
.btn-primary, .btn-secondary { padding: var(--space-2) var(--space-4); border-radius: var(--radius-md); font-weight: 600; }
.btn-primary { background: var(--color-primary); color: var(--color-ink-inverse); border: 0; }
.btn-secondary { background: var(--color-surface); color: var(--color-ink); border: 1px solid var(--color-border); }
@media (max-width: 900px) { .layout { grid-template-columns: 1fr; } }
</style>
