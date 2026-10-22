<script setup lang="ts">
// Pengiriman massal (bulk, TASK-039): unggah CSV → validasi per baris →
// submit idempoten → pantau progres job → unduh label massal (ZIP).
//
// Prinsip server yang diteruskan ke UI: baris error TIDAK memblokir baris
// valid. Laporan menampilkan masalah per baris, dan job tetap berjalan
// meski sebagian baris gagal.
import { ApiRequestError } from '~/composables/useApi'
import type {
  BulkJob,
  BulkReport,
  BulkSubmitResult,
  BulkUploadResult,
  Courier,
  CourierService,
  Warehouse,
} from '~/types/api'

definePageMeta({ layout: 'default', middleware: 'require-auth' })

const { t } = useT()
const { upload, validate, submit, job, downloadLabels } = useBulk()
const { list: listWarehouses } = useWarehouses()
const { list: listCouriers, getServices } = useCourierServices()

useHead({ title: () => t('bulk.title') })

type Step = 'upload' | 'review' | 'job'

const step = ref<Step>('upload')
const error = ref<string | null>(null)
const message = ref<string | null>(null)
const busy = ref(false)

// ── Unggahan ──
const fileInput = ref<HTMLInputElement | null>(null)
const file = ref<File | null>(null)
const uploaded = ref<BulkUploadResult | null>(null)

// ── Pilihan batch ──
const warehouses = ref<Warehouse[]>([])
const couriers = ref<Courier[]>([])
const services = ref<CourierService[]>([])
const warehouseId = ref('')
const courierCode = ref('')
const serviceCode = ref('')

// ── Laporan validasi ──
const report = ref<BulkReport | null>(null)
const showAll = ref(false)

// ── Job ──
const submitResult = ref<BulkSubmitResult | null>(null)
const currentJob = ref<BulkJob | null>(null)
const labelBusy = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null
let idemKey = newIdempotencyKey()

function errText(e: unknown): string {
  if (e instanceof ApiRequestError) {
    if (e.status === 429) return t('bulk.rate_limited')
    const fieldMsgs = e.fields ? Object.values(e.fields) : []
    return [e.message, ...fieldMsgs].filter(Boolean).join(' ')
  }
  return t('common.error')
}

function onPickFile(e: Event) {
  const input = e.target as HTMLInputElement
  const picked = input.files?.[0]
  if (!picked) return
  file.value = picked
  void doUpload()
}

async function doUpload() {
  if (!file.value) return
  busy.value = true
  error.value = null
  message.value = null
  report.value = null
  try {
    uploaded.value = await upload(file.value)
    message.value = t('bulk.uploaded', {
      count: uploaded.value.row_count,
      filename: file.value.name,
    })
  } catch (e) {
    uploaded.value = null
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function loadOptions() {
  try {
    const [whs, crs] = await Promise.all([listWarehouses(), listCouriers()])
    warehouses.value = whs
    couriers.value = crs
    if (!warehouseId.value && whs.length > 0) {
      const def = whs.find(w => w.is_default) ?? whs[0]
      warehouseId.value = def?.id ?? ''
    }
  } catch (e) {
    error.value = errText(e)
  }
}

async function onCourierChange() {
  serviceCode.value = ''
  services.value = []
  if (!courierCode.value) return
  try {
    services.value = await getServices(courierCode.value)
    if (services.value.length > 0) {
      serviceCode.value = services.value[0]?.code ?? ''
    }
  } catch (e) {
    error.value = errText(e)
  }
}

function batchPayload() {
  return {
    upload_id: uploaded.value?.upload_id,
    default_courier_code: courierCode.value,
    default_service_code: serviceCode.value,
    warehouse_id: warehouseId.value,
  }
}

async function runValidate() {
  if (!uploaded.value) return
  busy.value = true
  error.value = null
  message.value = null
  try {
    report.value = await validate(batchPayload())
    showAll.value = report.value.error > 0 || report.value.warning > 0
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function runSubmit() {
  if (!uploaded.value) return
  busy.value = true
  error.value = null
  message.value = null
  try {
    submitResult.value = await submit(batchPayload(), idemKey)
    message.value = submitResult.value.replayed
      ? t('bulk.submit_replayed')
      : t('bulk.submit_ok', {
          id: submitResult.value.job_id.slice(0, 8),
          accepted: submitResult.value.accepted,
          rejected: submitResult.value.rejected,
        })
    step.value = 'job'
    currentJob.value = null
    await refreshJob()
    startPolling()
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function refreshJob() {
  if (!submitResult.value) return
  try {
    currentJob.value = await job(submitResult.value.job_id)
  } catch (e) {
    error.value = errText(e)
    stopPolling()
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => {
    void refreshJob().then(() => {
      const s = currentJob.value?.status
      if (s === 'completed' || s === 'failed') stopPolling()
    })
  }, 1500)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function resetAll() {
  stopPolling()
  step.value = 'upload'
  file.value = null
  uploaded.value = null
  report.value = null
  submitResult.value = null
  currentJob.value = null
  showAll.value = false
  message.value = null
  error.value = null
  idemKey = newIdempotencyKey()
  if (fileInput.value) fileInput.value.value = ''
}

async function runDownloadLabels() {
  const ids = (currentJob.value?.rows ?? [])
    .map(r => r.shipment_id)
    .filter((id): id is string => Boolean(id))
  if (ids.length === 0) return
  labelBusy.value = true
  error.value = null
  try {
    const { blob, filename } = await downloadLabels(ids)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename ?? 'labels-bulk.zip'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    message.value = t('bulk.download_done')
  } catch (e) {
    error.value = errText(e)
  } finally {
    labelBusy.value = false
  }
}

/** Template CSV agar pengirim tidak menebak-nebak nama kolom. */
function downloadTemplate() {
  const header = 'nama penerima,telepon,alamat,kode pos,berat (kg),panjang,lebar,tinggi,COD,nilai barang,asuransi,catatan'
  const row1 = 'Budi Santoso,081234567890,"Jl. Melati No. 1, Semarang",50241,1.5,20,15,10,50000,150000,tidak,'
  const row2 = 'Siti Aminah,081122334455,"Jl. Kenanga No. 9, Bandung",40111,2.5,,,,0,250000,ya,'
  const csv = [header, row1, row2].join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'template-pengiriman-massal.csv'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

const canSubmit = computed(() =>
  Boolean(uploaded.value && warehouseId.value && courierCode.value && serviceCode.value && !busy.value))

const problemRows = computed(() => {
  const rows = report.value?.rows ?? []
  if (showAll.value) return rows
  return rows.filter(r => r.status !== 'valid')
})

const createdRows = computed(() =>
  (currentJob.value?.rows ?? []).filter(r => r.status === 'created'))
const failedRows = computed(() =>
  (currentJob.value?.rows ?? []).filter(r => r.status === 'failed'))

const progressPct = computed(() => {
  const j = currentJob.value
  if (!j || j.total_rows === 0) return 0
  return Math.round((j.processed_rows / j.total_rows) * 100)
})

const jobStatusLabel = computed(() => {
  switch (currentJob.value?.status) {
    case 'pending': return t('bulk.job_pending')
    case 'processing': return t('bulk.job_processing')
    case 'completed': return t('bulk.job_completed')
    case 'failed': return t('bulk.job_failed')
    default: return '—'
  }
})

const submitCount = computed(() => report.value?.valid ?? uploaded.value?.row_count ?? 0)

onMounted(loadOptions)
onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="bulk">
    <header class="page-head">
      <div>
        <h1 class="page-title">
          {{ t('bulk.title') }}
        </h1>
        <p class="page-subtitle">
          {{ t('bulk.subtitle') }}
        </p>
      </div>
      <NuxtLink
        to="/shipments"
        class="btn-secondary"
      >
        {{ t('bulk.back') }}
      </NuxtLink>
    </header>

    <div
      v-if="error"
      class="alert error"
      role="alert"
    >
      {{ error }}
    </div>
    <div
      v-if="message"
      class="alert ok"
      role="status"
    >
      {{ message }}
    </div>

    <!-- Langkah 1: unggah -->
    <section
      v-if="step === 'upload'"
      class="card panel"
    >
      <h2 class="panel-title">
        {{ t('bulk.step_upload') }}
      </h2>
      <p class="muted">
        {{ t('bulk.upload_hint') }}
      </p>
      <div class="row actions">
        <input
          ref="fileInput"
          type="file"
          accept=".csv,text/csv"
          :aria-label="t('bulk.choose_file')"
          @change="onPickFile"
        >
        <button
          type="button"
          class="btn-secondary"
          @click="downloadTemplate"
        >
          {{ t('bulk.download_template') }}
        </button>
      </div>
      <p
        v-if="busy"
        class="muted"
      >
        {{ t('common.loading') }}
      </p>
      <template v-if="uploaded">
        <p class="ok-line">
          {{ t('bulk.uploaded', { count: uploaded.row_count, filename: file?.name ?? '' }) }}
        </p>
        <button
          type="button"
          class="btn-primary"
          @click="step = 'review'"
        >
          {{ t('common.next') }}
        </button>
      </template>
    </section>

    <!-- Langkah 2: validasi & submit -->
    <section
      v-else-if="step === 'review'"
      class="card panel"
    >
      <h2 class="panel-title">
        {{ t('bulk.step_review') }}
      </h2>

      <div class="form-grid">
        <label class="field">
          <span>{{ t('bulk.warehouse') }}</span>
          <select
            v-model="warehouseId"
            class="filter"
          >
            <option value="">
              {{ t('bulk.select_warehouse') }}
            </option>
            <option
              v-for="w in warehouses"
              :key="w.id"
              :value="w.id"
            >
              {{ w.name }}
            </option>
          </select>
        </label>
        <label class="field">
          <span>{{ t('bulk.courier') }}</span>
          <select
            v-model="courierCode"
            class="filter"
            @change="onCourierChange"
          >
            <option value="">
              {{ t('bulk.select_courier') }}
            </option>
            <option
              v-for="c in couriers"
              :key="c.code"
              :value="c.code"
            >
              {{ c.name }}
            </option>
          </select>
        </label>
        <label class="field">
          <span>{{ t('bulk.service') }}</span>
          <select
            v-model="serviceCode"
            class="filter"
          >
            <option value="">
              {{ t('bulk.select_service') }}
            </option>
            <option
              v-for="s in services"
              :key="s.code"
              :value="s.code"
            >
              {{ s.name }}
            </option>
          </select>
        </label>
      </div>

      <div class="row actions">
        <button
          type="button"
          class="btn-secondary"
          :disabled="busy"
          @click="runValidate"
        >
          {{ busy ? t('bulk.validating') : t('bulk.validate') }}
        </button>
        <button
          type="button"
          class="btn-primary"
          :disabled="!canSubmit"
          @click="runSubmit"
        >
          {{ busy ? t('bulk.submitting') : t('bulk.submit', { n: submitCount }) }}
        </button>
        <button
          type="button"
          class="btn-mini"
          @click="resetAll"
        >
          {{ t('bulk.change_file') }}
        </button>
      </div>

      <p
        v-if="!report"
        class="muted"
      >
        {{ t('bulk.report_none') }}
      </p>
      <template v-else>
        <p class="report-line">
          <span class="chip ok">{{ t('bulk.report_valid', { n: report.valid }) }}</span>
          <span class="chip warn">{{ t('bulk.report_warning', { n: report.warning }) }}</span>
          <span class="chip bad">{{ t('bulk.report_error', { n: report.error }) }}</span>
        </p>

        <div class="table-wrap">
          <p
            v-if="problemRows.length === 0"
            class="state muted"
          >
            {{ t('bulk.empty_rows') }}
          </p>
          <table v-else>
            <thead>
              <tr>
                <th scope="col">
                  {{ t('bulk.row_no') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_recipient') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_phone') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_postal') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_weight') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_status') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_issues') }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="r in problemRows"
                :key="r.row_no"
              >
                <td class="num">
                  {{ r.row_no }}
                </td>
                <td>{{ r.row.recipient_name }}</td>
                <td>{{ r.row.recipient_phone }}</td>
                <td>{{ r.row.postal_code }}</td>
                <td class="num">
                  {{ r.row.weight_kg }}
                </td>
                <td>
                  <span
                    class="chip"
                    :class="r.status === 'error' ? 'bad' : r.status === 'warning' ? 'warn' : 'ok'"
                  >
                    {{ r.status }}
                  </span>
                </td>
                <td>
                  <ul
                    v-if="r.issues?.length"
                    class="issues"
                  >
                    <li
                      v-for="(is, i) in r.issues"
                      :key="i"
                    >
                      {{ is.message }}
                    </li>
                  </ul>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <button
          v-if="(report.rows.length > problemRows.length)"
          type="button"
          class="btn-mini"
          @click="showAll = !showAll"
        >
          {{ showAll ? t('bulk.show_problems') : t('bulk.show_all') }}
        </button>
      </template>
    </section>

    <!-- Langkah 3: hasil job -->
    <section
      v-else
      class="card panel"
    >
      <h2 class="panel-title">
        {{ t('bulk.step_job') }}
      </h2>

      <p
        v-if="!currentJob"
        class="muted"
      >
        {{ t('common.loading') }}
      </p>
      <template v-else>
        <p class="report-line">
          <span class="chip">{{ t('bulk.job_status') }}: {{ jobStatusLabel }}</span>
        </p>

        <div
          class="progress"
          role="progressbar"
          :aria-valuenow="progressPct"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <div
            class="progress-fill"
            :style="{ width: progressPct + '%' }"
          />
        </div>
        <p class="muted">
          {{ t('bulk.progress', {
            processed: currentJob.processed_rows,
            total: currentJob.total_rows,
            created: currentJob.created_rows,
            failed: currentJob.failed_rows,
          }) }}
        </p>
        <p
          v-if="currentJob.error_summary"
          class="alert error"
        >
          {{ currentJob.error_summary }}
        </p>

        <div
          v-if="createdRows.length > 0"
          class="table-wrap"
        >
          <h3 class="panel-title">
            {{ t('bulk.created_awb') }}
          </h3>
          <table>
            <thead>
              <tr>
                <th scope="col">
                  {{ t('bulk.row_no') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_recipient') }}
                </th>
                <th scope="col">
                  {{ t('shipment.col_awb') }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="r in createdRows.slice(0, 100)"
                :key="r.row_no"
              >
                <td class="num">
                  {{ r.row_no }}
                </td>
                <td>{{ r.row.recipient_name }}</td>
                <td>
                  <NuxtLink
                    class="link"
                    :to="`/shipments/${r.shipment_id}`"
                  >
                    {{ r.awb }}
                  </NuxtLink>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div
          v-if="failedRows.length > 0"
          class="table-wrap"
        >
          <h3 class="panel-title">
            {{ t('bulk.rows_failed_title') }}
          </h3>
          <table>
            <thead>
              <tr>
                <th scope="col">
                  {{ t('bulk.row_no') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_recipient') }}
                </th>
                <th scope="col">
                  {{ t('bulk.col_issues') }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="r in failedRows"
                :key="r.row_no"
              >
                <td class="num">
                  {{ r.row_no }}
                </td>
                <td>{{ r.row.recipient_name }}</td>
                <td>{{ r.error || t('bulk.issue_not_found') }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="row actions">
          <button
            v-if="createdRows.length > 0"
            type="button"
            class="btn-primary"
            :disabled="labelBusy"
            @click="runDownloadLabels"
          >
            {{ labelBusy ? t('bulk.downloading') : t('bulk.download_labels') }}
          </button>
          <button
            type="button"
            class="btn-secondary"
            @click="resetAll"
          >
            {{ t('bulk.new_job') }}
          </button>
        </div>
      </template>
    </section>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
  margin-bottom: var(--space-5);
}

.page-title {
  font-size: var(--text-xl);
  margin-bottom: var(--space-1);
}

.page-subtitle {
  margin: 0;
  color: var(--color-ink-secondary);
}

.alert {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-3);
}

.alert.error {
  border: 1px solid #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.alert.ok {
  border: 1px solid #bbf7d0;
  background: #f0fdf4;
  color: #14532d;
}

.panel {
  padding: var(--space-4);
  margin-bottom: var(--space-4);
}

.panel-title {
  font-size: var(--text-md);
  margin-bottom: var(--space-2);
}

.muted {
  color: var(--color-ink-muted);
}

.ok-line {
  color: #14532d;
  font-weight: 600;
}

.row {
  display: flex;
  gap: var(--space-3);
  flex-wrap: wrap;
  align-items: center;
  margin: var(--space-3) 0;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--text-sm);
}

.filter,
input[type='file'] {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: inherit;
  background: var(--color-surface);
  color: var(--color-ink);
}

.btn-primary,
.btn-secondary,
.btn-mini {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
  font: inherit;
}

.btn-primary {
  background: var(--color-accent, #0f766e);
  color: #fff;
}

.btn-secondary {
  background: var(--color-surface);
  border-color: var(--color-border);
  color: var(--color-ink);
}

.btn-mini {
  padding: var(--space-1) var(--space-2);
  font-size: var(--text-sm);
  background: var(--color-surface);
  border-color: var(--color-border);
  color: var(--color-ink);
}

.btn-primary:disabled,
.btn-secondary:disabled,
.btn-mini:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.report-line {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.chip {
  display: inline-block;
  padding: 2px var(--space-2);
  border-radius: 999px;
  font-size: var(--text-xs);
  border: 1px solid var(--color-border);
  background: var(--color-surface);
}

.chip.ok {
  border-color: #bbf7d0;
  background: #f0fdf4;
  color: #14532d;
}

.chip.warn {
  border-color: #fde68a;
  background: #fffbeb;
  color: #78350f;
}

.chip.bad {
  border-color: #fecdd3;
  background: #fff1f2;
  color: #9f1239;
}

.table-wrap {
  padding: var(--space-2) 0;
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

th,
td {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
}

th {
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: 0.02em;
  color: var(--color-ink-muted);
}

.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.link {
  font-weight: 600;
}

.issues {
  margin: 0;
  padding-left: var(--space-4);
  color: #9f1239;
}

.actions {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.progress {
  height: 10px;
  border-radius: 999px;
  background: var(--color-border);
  overflow: hidden;
  margin-bottom: var(--space-2);
}

.progress-fill {
  height: 100%;
  background: var(--color-accent, #0f766e);
  transition: width 0.4s ease;
}

.state {
  padding: var(--space-6) 0;
  text-align: center;
}
</style>
