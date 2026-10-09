import { describe, expect, it } from 'vitest'

import { formatIDR, formatIDRCompact, parseIDR } from '../app/utils/money'

describe('formatIDR', () => {
  it('memberi pemisah ribuan titik', () => {
    expect(formatIDR(1_234_567)).toBe('Rp1.234.567')
  })

  it('menangani nol dan nilai kecil', () => {
    expect(formatIDR(0)).toBe('Rp0')
    expect(formatIDR(500)).toBe('Rp500')
  })

  it('menandai nilai negatif di depan simbol', () => {
    expect(formatIDR(-18_500)).toBe('-Rp18.500')
  })

  it('membulatkan desimal rupiah', () => {
    expect(formatIDR(18_500.4)).toBe('Rp18.500')
    expect(formatIDR(18_500.6)).toBe('Rp18.501')
  })

  it('tidak jatuh pada nilai bukan angka', () => {
    expect(formatIDR(Number.NaN)).toBe('Rp0')
  })
})

describe('formatIDRCompact', () => {
  it('memakai satuan jt, M, dan T', () => {
    expect(formatIDRCompact(1_500_000)).toBe('Rp1,5 jt')
    expect(formatIDRCompact(3_400_000_000)).toBe('Rp3,4 M')
    expect(formatIDRCompact(2_000_000_000_000)).toBe('Rp2 T')
  })

  it('memakai rb untuk ribuan', () => {
    expect(formatIDRCompact(18_500)).toBe('Rp18,5 rb')
  })

  it('tidak menampilkan desimal bila utuh', () => {
    expect(formatIDRCompact(2_000_000)).toBe('Rp2 jt')
  })
})

describe('parseIDR', () => {
  it('membaca angka polos', () => {
    expect(parseIDR('1500')).toBe(1500)
  })

  it('membaca format bertitik gaya Indonesia', () => {
    expect(parseIDR('Rp1.500.000')).toBe(1_500_000)
  })

  it('membaca singkatan jt dan rb', () => {
    expect(parseIDR('1,5jt')).toBe(1_500_000)
    expect(parseIDR('350 rb')).toBe(350_000)
  })

  it('mengembalikan 0 untuk masukan kosong/tak sah', () => {
    expect(parseIDR('')).toBe(0)
    expect(parseIDR('abc')).toBe(0)
  })
})