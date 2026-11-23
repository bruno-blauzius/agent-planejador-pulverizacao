import { describe, expect, it } from 'vitest'
import type { ApplicationReview, DayConditions, SprayPlan, SprayWindowMap, TodayBrief } from '~/types/planner'
import {
  addDaysIso,
  abbreviateFarmName,
  buildDayConditionsView,
  buildPlanView,
  buildRegisteredNotice,
  buildReviewView,
  entryKey,
  buildTodayStatusView,
  buildTodayTaskView,
  buildWindowMapView,
  formatClock,
  formatDurationMinutes,
  onWeekday,
  splitParagraphs,
  todayLocationLabel,
  weekdayLong,
  windowProgress,
} from '~/utils/planner'

describe('dates and labels', () => {
  it('formats durations like the Figma', () => {
    expect([195, 240, 260, 45, 450].map(formatDurationMinutes)).toEqual(['3h15', '4h', '4h20', '45 min', '7h30'])
  })

  it('does calendar math on ISO dates', () => {
    expect(addDaysIso('2026-11-30', 1)).toBe('2026-12-01')
    expect(weekdayLong('2026-11-26')).toBe('quinta')
    expect(onWeekday('2026-11-28')).toBe('no sábado')
    expect(onWeekday('2026-11-25')).toBe('na quarta')
  })

  it('splits agent text in paragraphs and abbreviates the farm', () => {
    expect(splitParagraphs('A\n\n  B  \n\n\nC')).toEqual(['A', 'B', 'C'])
    expect(abbreviateFarmName('Fazenda Santa Rita')).toBe('Faz. Santa Rita')
    expect(abbreviateFarmName('Sítio Boa Esperança')).toBe('Sítio Boa Esperança')
  })
})

const plan: SprayPlan = {
  id: 'plan-1',
  farmId: 'f',
  productKind: 'fungicide',
  headline: '2 janelas aptas cobrem os 90 ha',
  totalAreaHa: 90,
  capacityHaPerHour: 12,
  entries: [
    { fieldId: 'a', fieldLabel: 'T3', areaHa: 38.6, window: { date: '2026-11-25', startTime: '06:00', endTime: '10:00', condition: 'suitable' }, durationMinutes: 195, alert: null },
    {
      fieldId: 'b',
      fieldLabel: 'T7',
      areaHa: 51.4,
      window: { date: '2026-11-27', startTime: '06:00', endTime: '10:00', condition: 'suitable' },
      durationMinutes: 260,
      alert: { kind: 'window_too_short', message: 'T7 precisa de 4h20 e a janela tem 4h.', requiredMinutes: 260, availableMinutes: 240 },
    },
  ],
}

describe('buildPlanView', () => {
  it('shows area, duration, window and the alert of each entry', () => {
    const view = buildPlanView(plan)
    expect(view.headline).toBe('2 janelas aptas cobrem os 90 ha')
    expect(view.entries.map(entry => [entry.fieldLabel, entry.detail, entry.windowLabel])).toEqual([
      ['T3', '38,6 ha · 3h15', 'Qua 25 · 06:00 – 10:00'],
      ['T7', '51,4 ha · 4h20', 'Sex 27 · 06:00 – 10:00'],
    ])
    expect(view.entries.map(entry => entry.alertMessage)).toEqual([null, 'T7 precisa de 4h20 e a janela tem 4h.'])
    expect(view.hasAlerts).toBe(true)
    expect(view.totalAreaLabel).toBe('90 ha')
  })
})

function mapOf(hours: ('suitable' | 'attention')[]): SprayWindowMap {
  return {
    farmId: 'f',
    generatedAt: new Date('2026-11-25T08:40:00Z'),
    forecastUpdatedAt: new Date('2026-11-25T08:40:00Z'),
    dayStartHour: 6,
    dayEndHour: 20,
    fields: [{ fieldId: 'a', fieldLabel: 'T3', days: [{ date: '2026-11-25', hours: hours.map((condition, index) => ({ hour: 6 + index, condition })) }] }],
    windows: [],
  }
}

describe('buildWindowMapView', () => {
  it('labels days and every third hour, and builds accessible cells', () => {
    const view = buildWindowMapView(mapOf(Array(15).fill('suitable')))
    expect(view.updatedLabel).toBe('dados de hoje 05:40')
    expect(view.days).toEqual([{ date: '2026-11-25', label: 'Qua 25', hourLabels: ['06', '09', '12', '15', '18'] }])
    expect(view.rows[0]?.days[0]?.cells[0]).toEqual({ hour: 6, condition: 'suitable', ariaLabel: 'T3, Qua 25, 06h: Apta' })
    expect(view.legend.map(item => item.label)).toEqual(['Apta', 'Atenção', 'Inapta', 'Chuva'])
    expect(view.isEmpty).toBe(false)
  })

  it('is empty without data', () => {
    expect(buildWindowMapView(null)).toMatchObject({ isEmpty: true, rows: [], days: [] })
  })
})

const conditions: DayConditions = {
  farmId: 'f',
  date: '2026-11-25',
  idealRanges: { deltaTC: { min: 2, max: 8 }, windKmh: { min: 3, max: 10 } },
  hours: [5, 6, 7, 8, 9, 10].map(hour => ({ hour, deltaTC: hour, windKmh: 6, gustKmh: 11, rainMm: 0, condition: 'suitable' as const })),
  windowSummary: {
    startTime: '06:00',
    endTime: '10:00',
    deltaTMinC: 2.6,
    deltaTMaxC: 6.2,
    windAvgKmh: 6.25,
    gustMaxKmh: 11,
    rainMm: 0,
    rainUntilHour: 18,
    nextRainAt: new Date('2026-11-26T06:00:00Z'),
  },
}

describe('buildDayConditionsView', () => {
  it('builds chart points, ideal band, window and the three stats', () => {
    const view = buildDayConditionsView(conditions)
    expect(view.title).toBe('Quarta 25/11 · condições hora a hora')
    expect(view.points.map(point => point.tickLabel)).toEqual(['5h', null, null, '8h', null, null])
    expect(view.points[1]).toMatchObject({ hour: 6, deltaT: 6, windKmh: 6, gustKmh: 11, deltaTRatio: 0.5, windRatio: 0.3, gustRatio: 0.55 })
    expect(view.windAxis).toEqual({ max: 20, ticks: [0, 5, 10, 15, 20] })
    expect(view.deltaTAxis).toEqual({ max: 12, ticks: [0, 4, 8, 12] })
    expect(view.idealRange.deltaT).toMatchObject({ min: 2, max: 8, minRatio: 2 / 12, maxRatio: 8 / 12 })
    expect(view.window).toEqual({ label: 'Janela 06–10h', startHour: 6, endHour: 10 })
    expect(view.stats).toEqual([
      { key: 'delta_t', label: 'Delta T na janela', value: '2,6 – 6,2 °C', caption: 'ideal 2 a 8' },
      { key: 'wind', label: 'Vento médio', value: '6 km/h', caption: 'rajada máx. 11' },
      { key: 'rain', label: 'Chuva até 18h', value: '0 mm', caption: 'próx. chuva qui 03h' },
    ])
  })

  it('grows the axis when the data goes beyond it and says when no rain is expected', () => {
    const hot = { ...conditions, hours: [{ hour: 12, deltaTC: 13.2, windKmh: 6, gustKmh: 24, rainMm: 0, condition: 'unsuitable' as const }], windowSummary: { ...conditions.windowSummary!, nextRainAt: null } }
    const view = buildDayConditionsView(hot)
    expect(view.deltaTAxis).toEqual({ max: 16, ticks: [0, 4, 8, 12, 16] })
    expect(view.windAxis).toEqual({ max: 25, ticks: [0, 5, 10, 15, 20, 25] })
    expect(view.stats[2]?.caption).toBe('sem chuva prevista')
  })

  it('scales delta T and wind on their own axes and a gust never overflows the wind axis', () => {
    const windy = {
      ...conditions,
      hours: [
        { hour: 12, deltaTC: 6, windKmh: 12, gustKmh: 37, rainMm: 0, condition: 'unsuitable' as const },
        { hour: 13, deltaTC: 3, windKmh: 5, gustKmh: 10, rainMm: 0, condition: 'suitable' as const },
      ],
    }
    const view = buildDayConditionsView(windy)

    expect(view.windAxis.max).toBe(40)
    expect(view.deltaTAxis.max).toBe(12)
    expect(view.points[0]).toMatchObject({ windRatio: 12 / 40, gustRatio: 37 / 40, deltaTRatio: 0.5 })
    for (const point of view.points) {
      for (const ratio of [point.deltaTRatio, point.windRatio, point.gustRatio]) {
        expect(ratio).toBeGreaterThanOrEqual(0)
        expect(ratio).toBeLessThanOrEqual(1)
      }
      expect(point.gustRatio).toBeGreaterThanOrEqual(point.windRatio)
    }
    expect(view.idealRange.windKmh).toMatchObject({ min: 3, max: 10, minRatio: 3 / 40, maxRatio: 10 / 40 })
  })

  it('keeps every ratio inside 0-1 even with odd data (negative or beyond the ideal band)', () => {
    const odd = {
      ...conditions,
      idealRanges: { deltaTC: { min: 2, max: 30 }, windKmh: { min: 3, max: 10 } },
      hours: [{ hour: 5, deltaTC: -1, windKmh: 0, gustKmh: 0, rainMm: 0, condition: 'attention' as const }],
    }
    const view = buildDayConditionsView(odd)

    expect(view.deltaTAxis.max).toBe(32)
    expect(view.points[0]).toMatchObject({ deltaTRatio: 0, windRatio: 0, gustRatio: 0 })
    expect(view.idealRange.deltaT.maxRatio).toBe(30 / 32)
  })

  it('has no window and no stats when the day has none', () => {
    const view = buildDayConditionsView({ ...conditions, windowSummary: null })
    expect(view.window).toBeNull()
    expect(view.stats).toEqual([])
    expect(buildDayConditionsView(null).isEmpty).toBe(true)
  })
})

const review: ApplicationReview = {
  id: 'r1',
  planId: 'plan-1',
  fieldId: 'a',
  fieldLabel: 'T3',
  variety: 'Catuaí 144',
  areaHa: 38.6,
  window: { date: '2026-11-25', startTime: '06:00', endTime: '10:00', condition: 'suitable' },
  product: { kind: 'fungicide', name: 'Fungicida', dose: { value: 0.5, unit: 'L/ha' }, mixRateLPerHa: 200 },
  mixVolumeL: 7720,
  tankCount: 4,
  tankCapacityL: 2000,
  checks: [
    { code: 'delta_t', status: 'ok', title: 'Delta T dentro da faixa', detail: '2,6 a 6,2 °C' },
    { code: 'window_fits', status: 'warning', title: 'Janela curta', detail: 'x' },
    { code: 'wind', status: 'blocked', title: 'Vento adequado', detail: 'y' },
  ],
  operator: { id: 'u', name: 'Carlos Mendes' },
  equipment: { id: 'e', name: 'Pulverizador 2.000 L' },
  canConfirm: false,
  expiresAt: new Date('2026-11-25T12:00:00Z'),
}

describe('buildReviewView', () => {
  it('formats the modal sections like the Figma', () => {
    const view = buildReviewView(review)
    expect(view).toMatchObject({
      fieldTitle: 'T3 · Catuaí 144',
      areaLabel: '38,6 ha',
      dayLabel: 'Qua · 25/11',
      timeLabel: '06:00 – 10:00',
      doseLabel: '0,5 L/ha · calda 200 L/ha',
      mixVolumeLabel: '7.720 L',
      tanksLabel: '· 4 tanques',
      operatorEquipment: 'Carlos Mendes · Pulverizador 2.000 L',
    })
    expect(view.checks.map(check => check.icon)).toEqual(['check', 'warning', 'blocked'])
    expect(view.warningCodes).toEqual(['window_fits'])
    expect(view.blockedTitles).toEqual(['Vento adequado'])
    expect(view.canConfirm).toBe(false)
  })

  it('uses the singular for one tank', () => {
    expect(buildReviewView({ ...review, tankCount: 1 }).tanksLabel).toBe('· 1 tanque')
  })
})

const brief: TodayBrief = {
  farm: { id: 'farm-santa-rita', name: 'Fazenda Santa Rita' },
  asOf: new Date('2026-11-25T09:48:00Z'),
  status: { condition: 'suitable', headline: 'Pode pulverizar', untilTime: '10:00', remainingMinutes: 192, windowStart: '06:00', windowEnd: '10:00' },
  metrics: [],
  hours: [],
  task: { id: 't', fieldId: 'a', fieldLabel: 'T3', fieldName: 'Talhão 3', operation: 'Fungicida', areaHa: 38.6, dose: { value: 0.5, unit: 'L/ha' }, tankCount: 4, tankCapacityL: 2000, status: 'planned' },
}

describe('buildRegisteredNotice', () => {
  it('formats the success toast with the day and month of the window', () => {
    const notice = buildRegisteredNotice({
      id: 'a-1',
      status: 'planned',
      fieldId: 'a',
      fieldLabel: 'T3',
      areaHa: 38.6,
      window: { date: '2026-11-25', startTime: '06:00', endTime: '10:00', condition: 'suitable' },
      registeredAt: new Date('2026-11-25T09:00:00Z'),
      registeredBy: { id: 'u', name: 'Marina Rezende' },
    })
    expect(notice).toEqual({ title: 'Aplicação registrada', message: 'T3 · 25/11 06:00 – 10:00. Ela entra como planejada.' })
    expect(entryKey('p', 'f')).toBe('p:f')
  })
})

describe('today helpers', () => {
  it('builds the status card', () => {
    expect(formatClock(brief.asOf)).toBe('06:48')
    expect(buildTodayStatusView(brief)).toMatchObject({
      headline: 'Pode pulverizar',
      nowLabel: 'AGORA · 06:48',
      untilLabel: 'até 10:00',
      remainingLabel: '· faltam 3h12',
      startLabel: '06:00',
      endLabel: '10:00',
    })
    expect(buildTodayStatusView(brief).progressRatio).toBeCloseTo(0.2)
  })

  it('clamps progress and tolerates no window', () => {
    expect(windowProgress(new Date('2026-11-25T08:00:00Z'), '06:00', '10:00')).toBe(0)
    expect(windowProgress(new Date('2026-11-25T18:00:00Z'), '06:00', '10:00')).toBe(1)
    expect(windowProgress(brief.asOf, null, null)).toBe(0)
    expect(buildTodayStatusView({ ...brief, status: { ...brief.status, untilTime: null, remainingMinutes: null } }))
      .toMatchObject({ untilLabel: null, remainingLabel: null })
  })

  it('describes the task and the location', () => {
    expect(buildTodayTaskView(brief)).toEqual({
      id: 't',
      fieldLabel: 'T3',
      title: 'Fungicida · 38,6 ha',
      detail: '0,5 L/ha · 4 tanques de 2.000 L',
      isStarted: false,
    })
    expect(todayLocationLabel(brief)).toBe('Faz. Santa Rita · Talhão 3')
    expect(buildTodayTaskView({ ...brief, task: null })).toBeNull()
    expect(todayLocationLabel({ ...brief, task: null })).toBe('Faz. Santa Rita')
  })
})
