// Util berat & dimensi. Cerminan rumus backend (platform/money.go) agar angka
// yang ditampilkan sama dengan yang ditagihkan — termasuk berat volumetrik.

export interface Dimensions {
  length: number
  width: number
  height: number
}

export const DEFAULT_VOLUMETRIC_DIVISOR = 6000

/** Volume dalam cm³. */
export function volumeCm3(d: Dimensions): number {
  return Math.max(0, d.length) * Math.max(0, d.width) * Math.max(0, d.height)
}

/** Berat volumetrik (kg) dari dimensi. */
export function volumetricWeight(d: Dimensions, divisor = DEFAULT_VOLUMETRIC_DIVISOR): number {
  const safeDivisor = divisor > 0 ? divisor : DEFAULT_VOLUMETRIC_DIVISOR
  return roomToThree(volumeCm3(d) / safeDivisor)
}

/** Berat yang ditagihkan: maksimum antara aktual dan volumetrik. */
export function chargeableWeight(
  actualKg: number,
  dims?: Dimensions,
  divisor = DEFAULT_VOLUMETRIC_DIVISOR,
): number {
  const actual = Math.max(0, actualKg)
  if (!dims) return roomToThree(actual)
  return roomToThree(Math.max(actual, volumetricWeight(dims, divisor)))
}

/** Format berat untuk tampilan: 1,5 kg · 800 g. */
export function formatWeight(kg: number): string {
  if (kg < 1) return `${Math.round(kg * 1000)} g`
  const oneDecimal = Math.round(kg * 10) / 10
  return `${Number.isInteger(oneDecimal) ? oneDecimal : oneDecimal.toFixed(1).replace('.', ',')} kg`
}

function roomToThree(v: number): number {
  return Math.round(v * 1000) / 1000
}