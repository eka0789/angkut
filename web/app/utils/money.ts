// Util uang. Nominal IDR selalu bilangan bulat rupiah (tanpa sen) — sama
// seperti BIGINT di backend. Format mengikuti kebiasaan Indonesia: Rp1.234.567.

/** Format rupiah penuh. */
export function formatIDR(amount: number): string {
  if (!Number.isFinite(amount)) return 'Rp0'
  const rounded = Math.round(amount)
  const sign = rounded < 0 ? '-' : ''
  const digits = Math.abs(rounded).toString()
  const withDots = digits.replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  return `${sign}Rp${withDots}`
}

/** Format rupiah ringkas untuk kartu KPI: Rp1,2 jt · Rp3,4 M. */
export function formatIDRCompact(amount: number): string {
  const abs = Math.abs(amount)
  const sign = amount < 0 ? '-' : ''

  if (abs >= 1_000_000_000_000) return `${sign}Rp${trim(abs / 1_000_000_000_000)} T`
  if (abs >= 1_000_000_000) return `${sign}Rp${trim(abs / 1_000_000_000)} M`
  if (abs >= 1_000_000) return `${sign}Rp${trim(abs / 1_000_000)} jt`
  if (abs >= 1_000) return `${sign}Rp${trim(abs / 1_000)} rb`
  return `${sign}Rp${Math.round(abs)}`
}

function trim(v: number): string {
  // Satu desimal bila perlu, tanpa trailing nol: 1,5 jt bukan 1,50 jt.
  const oneDecimal = Math.round(v * 10) / 10
  return Number.isInteger(oneDecimal)
    ? oneDecimal.toString()
    : oneDecimal.toFixed(1).replace('.', ',')
}

/** Parse input pengguna ("Rp 1.500", "1500", "1,5jt") menjadi rupiah utuh. */
export function parseIDR(input: string): number {
  const cleaned = input.trim().toLowerCase().replace(/rp/gi, '').replace(/\s/g, '')
  if (!cleaned) return 0

  const multiplier = cleaned.endsWith('jt')
    ? 1_000_000
    : cleaned.endsWith('rb')
      ? 1_000
      : cleaned.endsWith('m')
        ? 1_000_000_000
        : 1

  const numeric = cleaned
    .replace(/(jt|rb|m)$/, '')
    .replace(/\./g, '')   // pemisah ribuan gaya Indonesia
    .replace(',', '.')    // desimal gaya Indonesia

  const value = Number.parseFloat(numeric)
  return Number.isFinite(value) ? Math.round(value * multiplier) : 0
}