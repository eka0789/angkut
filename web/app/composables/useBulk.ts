// Composable untuk pengiriman massal (bulk, TASK-039). Semua endpoint
// tenant-scoped dan rate-limited per organisasi (5 req/menit).
//
// Alur: upload CSV → validate (laporan per baris) → submit (Idempotency-Key
// wajib, balas 202 job_id) → poll status → unduh label massal (ZIP).
import type {
  BulkJob,
  BulkReport,
  BulkRow,
  BulkSubmitResult,
  BulkUploadResult,
} from '~/types/api'

export function useBulk() {
  const { apiFetch, apiFetchBlob } = useApi()

  /** Unggah file CSV (multipart, field "file"). XLSX ditolak server dengan pesan jelas. */
  async function upload(file: File): Promise<BulkUploadResult> {
    const form = new FormData()
    form.append('file', file)
    return apiFetch<BulkUploadResult>('/api/v1/shipments/bulk/upload', {
      method: 'POST',
      body: form,
    })
  }

  /** Validasi tanpa efek samping — dipakai sebelum submit. */
  async function validate(payload: {
    rows?: BulkRow[]
    upload_id?: string
    default_courier_code: string
    default_service_code: string
    warehouse_id: string
  }): Promise<BulkReport> {
    return apiFetch<BulkReport>('/api/v1/shipments/bulk/validate', {
      method: 'POST',
      body: payload,
    })
  }

  /** Submit job massal; key dibuat pemanggil supaya retry aman. */
  async function submit(payload: {
    rows?: BulkRow[]
    upload_id?: string
    default_courier_code: string
    default_service_code: string
    warehouse_id: string
  }, idempotencyKey: string): Promise<BulkSubmitResult> {
    return apiFetch<BulkSubmitResult>('/api/v1/shipments/bulk', {
      method: 'POST',
      body: payload,
      headers: { 'Idempotency-Key': idempotencyKey },
    })
  }

  /** Status job + hasil per baris. */
  async function job(jobID: string): Promise<BulkJob> {
    return apiFetch<BulkJob>(`/api/v1/shipments/bulk/${jobID}`)
  }

  /** Unduh label massal: ZIP berisi satu PDF per pengiriman. */
  async function downloadLabels(ids: string[], format: 'a4' | 'thermal' = 'a4') {
    return apiFetchBlob('/api/v1/shipments/labels/bulk', {
      method: 'POST',
      body: { ids, format },
    })
  }

  return { upload, validate, submit, job, downloadLabels }
}
