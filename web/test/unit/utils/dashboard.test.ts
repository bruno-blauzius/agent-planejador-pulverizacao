import { describe, expect, it } from 'vitest'
import type { Dashboard } from '~/types/dashboard'
import {
  activityTimeLabel,
  buildDashboardChart,
  buildDashboardStatCards,
  dashboardDateLine,
  toUpcomingWindowView,
} from '~/utils/dashboard'

const now = new Date('2026-11-21T12:00:00Z')

describe('dashboard presentation', () => {
  it('writes the date line in the display time zone', () => {
    expect(dashboardDateLine(now, 'Fazenda Santa Rita')).toBe('Sábado, 21 de novembro · Fazenda Santa Rita')
    // 01:00 UTC is still the previous day in São Paulo.
    expect(dashboardDateLine(new Date('2026-11-22T01:00:00Z'), null)).toBe('Sábado, 21 de novembro')
  })

  it('shows placeholders while loading and the Figma captions with data', () => {
    expect(buildDashboardStatCards(null).every(card => card.value === '—' && card.caption === null)).toBe(true)
    const indicators: Dashboard['indicators'] = {
      suitableWindowsToday: { count: 3, fieldLabels: ['T3', 'T5', 'T7'], nextStartAt: new Date('2026-11-21T09:00:00Z') },
      applicationsThisMonth: { count: 12, confirmedThisWeek: 4 },
      sprayedArea: { areaHa: 412, productiveAreaHa: 982 },
      pendingItems: { count: 2, headline: '1 carência vence amanhã' },
    }
    const cards = buildDashboardStatCards({ indicators } as Dashboard)
    expect(cards.map(card => [card.value, card.caption])).toEqual([
      ['3', 'T3 · T5 · T7 · próxima às 06:00'],
      ['12', '4 confirmadas esta semana'],
      ['412 ha', 'de 982 ha produtivos · 42%'],
      ['2', '1 carência vence amanhã'],
    ])
  })

  it('scales each chart series on its own and keeps real values for screen readers', () => {
    const chart = buildDashboardChart([
      { date: '2026-11-20', suitableHours: 10, applications: 1 },
      { date: '2026-11-21', suitableHours: 5, applications: 2 },
    ], null)
    expect(chart.days.map(day => [day.hoursRatio, day.applicationsRatio])).toEqual([[1, 0.5], [0.5, 1]])
    expect(chart.days[0]?.ariaLabel).toBe('20/11: 10 h aptas, 1 aplicação')
    expect(buildDashboardChart([{ date: '2026-11-21', suitableHours: 0, applications: 0 }], null).isEmpty).toBe(true)
  })

  it('labels upcoming windows relative to today', () => {
    const base = { fieldId: 'f', fieldLabel: 'T2', farmId: 'x', cropVariety: 'Catuaí 144', operation: 'fungicida' }
    expect(toUpcomingWindowView({ ...base, date: '2026-11-21', startTime: '06:00', endTime: '09:30', condition: 'suitable' }, '2026-11-21'))
      .toMatchObject({ whenLabel: 'Hoje · 06:00 – 09:30', detail: 'Catuaí 144 · fungicida', isHighlighted: true, condition: { label: 'Apta' } })
    expect(toUpcomingWindowView({ ...base, date: '2026-11-22', startTime: '06:00', endTime: '11:00', condition: 'suitable' }, '2026-11-21').whenLabel)
      .toBe('Amanhã · 06:00 – 11:00')
    expect(toUpcomingWindowView({ ...base, date: '2026-11-26', startTime: null, endTime: null, condition: 'rain' }, '2026-11-21'))
      .toMatchObject({ whenLabel: 'Qui · —', isHighlighted: false })
  })

  it('labels activity times', () => {
    expect(activityTimeLabel(new Date('2026-11-21T13:12:00Z'), now)).toBe('hoje, 10:12')
    expect(activityTimeLabel(new Date('2026-11-20T20:40:00Z'), now)).toBe('ontem, 17:40')
    expect(activityTimeLabel(new Date('2026-11-18T12:00:00Z'), now)).toBe('18/11, 09:00')
  })
})
