// Composable money (TASK-041..046): COD, settlement, ledger, returns.
// Semua endpoint finance+/RBAC — 403 ditampilkan sebagai pesan role.
import type {
  CODSettlement, CODSummary, CODTransaction, FinancialReport,
  LedgerEntry, ProductReturn, ReturnPolicy,
} from '~/types/api'

export function useMoney() {
  const { apiFetch, apiFetchBlob } = useApi()

  async function summary(): Promise<CODSummary> {
    return apiFetch<CODSummary>('/api/v1/cod/summary')
  }

  async function transactions(status?: string): Promise<CODTransaction[]> {
    const query: Record<string, string> = {}
    if (status) query.status = status
    return apiFetch<CODTransaction[]>('/api/v1/cod/transactions', { query })
  }

  async function settlements(status?: string): Promise<CODSettlement[]> {
    const query: Record<string, string> = {}
    if (status) query.status = status
    return apiFetch<CODSettlement[]>('/api/v1/cod/settlements', { query })
  }

  async function submitSettlement(input: {
    courier_code: string, period_start: string, period_end: string, transaction_ids: string[],
  }, idempotencyKey: string): Promise<CODSettlement> {
    return apiFetch<CODSettlement>('/api/v1/cod/settlements', {
      method: 'POST', body: input, headers: { 'Idempotency-Key': idempotencyKey },
    })
  }

  async function markPaid(id: string, reference: string, idempotencyKey: string): Promise<CODSettlement> {
    return apiFetch<CODSettlement>(`/api/v1/cod/settlements/${id}/mark-paid`, {
      method: 'POST', body: { reference }, headers: { 'Idempotency-Key': idempotencyKey },
    })
  }

  async function exportSettlementsCSV(): Promise<void> {
    const { blob, filename } = await apiFetchBlob('/api/v1/cod/settlements/export')
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename ?? 'cod-settlements.csv'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  async function ledger(account?: string): Promise<LedgerEntry[]> {
    const query: Record<string, string> = {}
    if (account) query.account = account
    return apiFetch<LedgerEntry[]>('/api/v1/ledger/entries', { query })
  }

  async function financial(): Promise<FinancialReport> {
    return apiFetch<FinancialReport>('/api/v1/analytics/financial')
  }

  async function returnsList(status?: string): Promise<ProductReturn[]> {
    const query: Record<string, string> = {}
    if (status) query.status = status
    return apiFetch<ProductReturn[]>('/api/v1/returns', { query })
  }

  async function createReturn(input: {
    shipment_id?: string, order_id?: string, type?: string, reason: string,
    refund_amount?: number, items: { name: string, qty: number }[],
  }): Promise<ProductReturn> {
    return apiFetch<ProductReturn>('/api/v1/returns', { method: 'POST', body: input })
  }

  async function returnAction(id: string, action: 'approve' | 'reject' | 'receive' | 'inspect' | 'refund', body: Record<string, unknown> = {}, idempotencyKey?: string): Promise<ProductReturn> {
    const headers: Record<string, string> = {}
    if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
    return apiFetch<ProductReturn>(`/api/v1/returns/${id}/${action}`, {
      method: 'POST', body, headers,
    })
  }

  async function policy(): Promise<ReturnPolicy> {
    return apiFetch<ReturnPolicy>('/api/v1/return-policies')
  }

  async function updatePolicy(input: { window_days: number, auto_approve_limit: number, conditions?: string[] }): Promise<ReturnPolicy> {
    return apiFetch<ReturnPolicy>('/api/v1/return-policies', { method: 'PUT', body: input })
  }

  return {
    summary, transactions, settlements, submitSettlement, markPaid,
    exportSettlementsCSV, ledger, financial,
    returnsList, createReturn, returnAction, policy, updatePolicy,
  }
}
