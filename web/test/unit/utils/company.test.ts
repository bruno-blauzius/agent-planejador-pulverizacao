import { describe, expect, it } from 'vitest'
import { applicationsTrendCaption, buildCompanyPagination, companyInitials } from '~/utils/company'

describe('company presentation', () => {
  const now = new Date('2026-11-21T12:00:00Z')

  it('describes the month-over-month change', () => {
    expect(applicationsTrendCaption(31, 26, now)).toBe('+19% vs. outubro')
    expect(applicationsTrendCaption(20, 25, now)).toBe('−20% vs. outubro')
    expect(applicationsTrendCaption(10, 10, now)).toBe('0% vs. outubro')
    expect(applicationsTrendCaption(3, 0, now)).toBe('Primeiro mês com aplicações')
  })

  it('labels the pagination range', () => {
    expect(buildCompanyPagination(2, 10, 23)).toEqual({
      rangeLabel: 'Mostrando 11–20 de 23', page: 2, pageCount: 3, canPrevious: true, canNext: true,
    })
    expect(buildCompanyPagination(1, 10, 0).rangeLabel).toBe('Nenhuma empresa')
  })

  it('skips the generic prefix in the initials', () => {
    expect(companyInitials('Fazenda Santa Rita')).toBe('SR')
    expect(companyInitials('Sítio Boa Esperança')).toBe('BE')
    expect(companyInitials('Fazenda Recanto')).toBe('RE')
  })
})
