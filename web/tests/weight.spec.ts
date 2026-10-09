import { describe, expect, it } from 'vitest'

import { chargeableWeight, formatWeight, volumeCm3, volumetricWeight } from '../app/utils/weight'

describe('volumeCm3', () => {
  it('mengalikan tiga sisi', () => {
    expect(volumeCm3({ length: 30, width: 20, height: 10 })).toBe(6000)
  })

  it('menganggap sisi negatif sebagai nol', () => {
    expect(volumeCm3({ length: -30, width: 20, height: 10 })).toBe(0)
  })
})

describe('volumetricWeight', () => {
  it('memakai divisor 6000 secara default', () => {
    // 30×20×10 = 6000 cm³ → 1 kg
    expect(volumetricWeight({ length: 30, width: 20, height: 10 })).toBe(1)
  })

  it('menghormati divisor kustom', () => {
    expect(volumetricWeight({ length: 30, width: 20, height: 10 }, 5000)).toBe(1.2)
  })

  it('menolak divisor tidak wajar dengan default', () => {
    expect(volumetricWeight({ length: 30, width: 20, height: 10 }, 0)).toBe(1)
  })
})

describe('chargeableWeight', () => {
  it('memilih berat aktual bila lebih besar', () => {
    expect(chargeableWeight(3, { length: 30, width: 20, height: 10 })).toBe(3)
  })

  it('memilih berat volumetrik bila lebih besar', () => {
    // Kotak besar ringan: volumetrik menang.
    expect(chargeableWeight(0.5, { length: 60, width: 40, height: 40 })).toBe(16)
  })

  it('tanpa dimensi memakai berat aktual', () => {
    expect(chargeableWeight(2.5)).toBe(2.5)
  })

  it('mencegah berat negatif', () => {
    expect(chargeableWeight(-5)).toBe(0)
  })
})

describe('formatWeight', () => {
  it('memakai gram di bawah 1 kg', () => {
    expect(formatWeight(0.8)).toBe('800 g')
  })

  it('memakai kg dengan koma desimal', () => {
    expect(formatWeight(1.5)).toBe('1,5 kg')
    expect(formatWeight(2)).toBe('2 kg')
  })
})