import { describe, expect, it } from 'vitest'
import { totalSharePercent, cropNames } from '~/utils/farm'
import {
  formatCnpj,
  formatCoordinates,
  formatCpf,
  formatDate,
  formatDaysAgo,
  formatDocument,
  formatHectares,
  formatMonthYear,
  formatPercent,
  formatPhone,
  formatPostalCode,
  initials,
  maskDocument,
} from '~/utils/format'

describe('format', () => {
  it.each([
    [486.2, '486,2 ha'],
    [1284, '1.284 ha'],
    [41.25, '41,3 ha'],
    [0, '0 ha'],
  ])('formatHectares(%s) = %s', (value, expected) => {
    expect(formatHectares(value)).toBe(expected)
  })

  it('formats percentages and the empty share', () => {
    expect(formatPercent(60)).toBe('60%')
    expect(formatPercent(null)).toBe('—')
  })

  it('formats CPF and CNPJ, leaving malformed input untouched', () => {
    expect(formatCpf('12345678900')).toBe('123.456.789-00')
    expect(formatCnpj('12345678000190')).toBe('12.345.678/0001-90')
    expect(formatDocument('98765432000110')).toBe('98.765.432/0001-10')
    expect(formatCpf('123')).toBe('123')
  })

  it('masks CPF in lists but shows CNPJ', () => {
    expect(maskDocument('12327891605')).toBe('***.278.916-**')
    expect(maskDocument('12345678000190')).toBe('12.345.678/0001-90')
  })

  it('formats phone, postal code and coordinates', () => {
    expect(formatPhone('35998124470')).toBe('(35) 99812-4470')
    expect(formatPhone('3532140000')).toBe('(35) 3214-0000')
    expect(formatPostalCode('37800000')).toBe('37800-000')
    expect(formatCoordinates({ lat: -21.3, lng: -46.71 })).toBe('-21,30 · -46,71')
    expect(formatCoordinates(null)).toBe('—')
  })

  it('builds avatar initials from the first two words', () => {
    expect(initials('Bruno Blauzius')).toBe('BB')
    expect(initials('Cooperativa Vale Verde')).toBe('CV')
    expect(initials('  marina  ')).toBe('M')
  })

  it('formats dates without timezone drift', () => {
    expect(formatMonthYear('2019-03-15')).toBe('mar/2019')
    expect(formatDate('1979-03-12')).toBe('12/03/1979')
  })

  it('formats elapsed days', () => {
    const now = new Date('2026-11-21T12:00:00Z')
    expect(formatDaysAgo(new Date('2026-11-09T12:00:00Z'), now)).toBe('há 12 dias')
    expect(formatDaysAgo(new Date('2026-11-20T12:00:00Z'), now)).toBe('ontem')
    expect(formatDaysAgo(null, now)).toBe('—')
  })
})

describe('farm display helpers', () => {
  it('sums only owner shares for the participation bar', () => {
    expect(totalSharePercent([
      { role: 'owner', sharePercent: 60 },
      { role: 'owner', sharePercent: 40 },
      { role: 'tenant', sharePercent: null },
    ])).toBe(100)
  })

  it('joins crop names for the card', () => {
    expect(cropNames({ crops: [{ code: 'coffee_arabica', name: 'Café arábica' }, { code: 'corn', name: 'Milho' }] }))
      .toBe('Café arábica · Milho')
  })
})
