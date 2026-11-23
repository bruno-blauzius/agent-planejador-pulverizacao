import { describe, expect, it } from 'vitest'
import type { ApplicationDetail, ApplicationSummary } from '~/types/application'
import {
  applicationRangeLabel,
  buildApplicationDetailView,
  buildConditionsCell,
  buildApplicationSummaryCards,
  firstName,
  formatDateTimeLabel,
  formatDose,
  formatLiters,
  formatMixRate,
  formatTableArea,
  formatTimeRange,
  formatWeatherLabel,
  formatWeekdayDate,
  hasActiveApplicationFilters,
  monthContextLabel,
  productKindOptions,
  statusView,
  toApplicationRowView,
} from '~/utils/applications'
import { APPLICATION_STATUS_LABELS } from '~/utils/labels'

const summary: ApplicationSummary = {
  id: 'a-5',
  status: 'completed',
  farmId: 'farm-santa-rita',
  window: { date: '2026-11-21', startTime: '06:12', endTime: '09:40' },
  field: { id: 'field-str-5', label: 'T5', name: 'Talhão 5', variety: 'Catuaí 144', block: 'Área Centro' },
  product: { kind: 'insecticide', name: 'Inseticida', dose: { value: 0.3, unit: 'L/ha' }, mixRateLPerHa: 180 },
  areaHa: 56.3,
  conditions: { source: 'recorded', deltaTC: 4.2, windKmh: 6, rainMm: null },
  operator: { id: 'user-carlos', name: 'Carlos Mendes' },
}

describe('formatters', () => {
  it('formats the Figma strings', () => {
    expect(formatWeekdayDate('2026-11-21')).toBe('Sáb 21/11')
    expect(formatTimeRange('06:12', '09:40')).toBe('06:12 – 09:40')
    expect(formatDose({ value: 0.3, unit: 'L/ha' })).toBe('0,3 L/ha')
    expect(formatDose({ value: 2, unit: 'L/ha' })).toBe('2 L/ha')
    expect(formatMixRate(180)).toBe('calda 180 L/ha')
    expect([10134, 16.9, 7720].map(formatLiters)).toEqual(['10.134 L', '16,9 L', '7.720 L'])
    expect([56.3, 44, 51.4].map(formatTableArea)).toEqual(['56,3 ha', '44,0 ha', '51,4 ha'])
    expect(firstName('  Carlos Mendes ')).toBe('Carlos')
  })

  it('formats instants in the display zone, the weather chip and the month context', () => {
    expect(formatDateTimeLabel(new Date('2026-11-21T12:52:00Z'))).toBe('21/11 às 09:52')
    expect(formatWeatherLabel({ farmId: 'f', city: 'Guaxupé', state: 'MG', temperatureC: 24.2, windKmh: 7, observedAt: new Date() }))
      .toBe('Guaxupé · 24 °C · vento 7 km/h')
    expect(monthContextLabel(new Date('2026-11-21T12:00:00Z'), 'Fazenda Santa Rita')).toBe('Novembro de 2026 · Fazenda Santa Rita')
    expect(monthContextLabel(new Date('2026-11-21T12:00:00Z'), null)).toBe('Novembro de 2026')
  })

  it('labels status and the range of the page', () => {
    expect(Object.values(APPLICATION_STATUS_LABELS)).toEqual(['Planejada', 'Em andamento', 'Concluída', 'Cancelada'])
    expect(statusView('completed')).toEqual({ value: 'completed', label: 'Concluída', tone: 'suitable' })
    expect(statusView('cancelled').tone).toBe('muted')
    expect(applicationRangeLabel(1, 10, 6)).toBe('1–6 de 6')
    expect(applicationRangeLabel(2, 10, 14)).toBe('11–14 de 14')
    expect(applicationRangeLabel(1, 10, 0)).toBe('0 de 0')
  })

  it('knows when a filter is active (the default period alone is not)', () => {
    expect(hasActiveApplicationFilters({ periodDays: 30 })).toBe(false)
    expect(hasActiveApplicationFilters({ periodDays: 7 })).toBe(true)
    expect(hasActiveApplicationFilters({ productKind: 'foliar' })).toBe(true)
    expect(productKindOptions().map(option => option.label)).toEqual(['Todos', 'Fungicida', 'Inseticida', 'Herbicida', 'Foliar', 'Outro'])
  })
})

describe('buildApplicationSummaryCards', () => {
  const summary = {
    monthCount: { total: 12, planned: 2, inProgress: 0, completed: 9, cancelled: 1 },
    appliedArea: { areaHa: 412, productiveAreaHa: 982, percent: 42 },
    withinWindow: { percent: 92, within: 11, total: 12 },
    next: { applicationId: 'a', date: '2026-11-26', startTime: '06:00', endTime: '10:00', fieldLabel: 'T3', productName: 'Fungicida' },
  }

  it('builds the cards of the Figma', () => {
    expect(buildApplicationSummaryCards(summary).map(card => [card.label, card.value, card.caption, card.tone])).toEqual([
      ['No mês', '12', '9 concluídas · 2 planejadas · 1 cancelada', 'default'],
      ['Área aplicada', '412 ha', '42% da área produtiva', 'default'],
      ['Dentro da janela', '92%', '11 de 12 com Delta T e vento ideais', 'highlight'],
      ['Próxima', 'Qui 26/11', 'T3 · 06:00 – 10:00 · fungicida', 'default'],
    ])
  })

  it('leaves out zero counts, keeps the singular and shows in progress', () => {
    const cards = buildApplicationSummaryCards({ ...summary, monthCount: { total: 3, planned: 1, inProgress: 1, completed: 1, cancelled: 0 } })
    expect(cards[0]?.caption).toBe('1 concluída · 1 em andamento · 1 planejada')
    expect(buildApplicationSummaryCards({ ...summary, monthCount: { total: 0, planned: 0, inProgress: 0, completed: 0, cancelled: 0 } })[0])
      .toMatchObject({ value: '0', caption: null })
  })

  it('shows dashes where the API has nothing', () => {
    const cards = buildApplicationSummaryCards({
      ...summary,
      appliedArea: { areaHa: 0, productiveAreaHa: 0, percent: null },
      withinWindow: { percent: null, within: 0, total: 0 },
      next: null,
    })
    expect(cards.map(card => [card.value, card.caption])).toEqual([
      ['12', '9 concluídas · 2 planejadas · 1 cancelada'],
      ['0 ha', null],
      ['—', null],
      ['—', null],
    ])
    expect(buildApplicationSummaryCards(null).map(card => card.value)).toEqual(['—', '—', '—', '—'])
  })
})

describe('buildConditionsCell', () => {
  it('shows recorded or forecast numbers', () => {
    expect(buildConditionsCell(summary)).toEqual({ kind: 'values', parts: ['4,2 °C', '6 km/h'] })
    expect(buildConditionsCell({ status: 'planned', conditions: { source: 'forecast', deltaTC: 3.1, windKmh: 5, rainMm: null } }))
      .toEqual({ kind: 'values', parts: ['3,1 °C', '5 km/h'] })
  })

  it('says "Previsão ao vivo" when there are no numbers yet, and the rain that cancelled it', () => {
    expect(buildConditionsCell({ status: 'planned', conditions: { source: 'live', deltaTC: null, windKmh: null, rainMm: null } }))
      .toEqual({ kind: 'live', label: 'Previsão ao vivo' })
    expect(buildConditionsCell({ status: 'cancelled', conditions: { source: 'recorded', deltaTC: null, windKmh: null, rainMm: 18 } }))
      .toEqual({ kind: 'rain', label: 'Chuva 18 mm' })
  })

  it('falls back to a dash and tolerates a single number', () => {
    expect(buildConditionsCell({ status: 'completed', conditions: { source: 'recorded', deltaTC: null, windKmh: null, rainMm: null } }))
      .toEqual({ kind: 'none', label: '—' })
    expect(buildConditionsCell({ status: 'completed', conditions: { source: 'recorded', deltaTC: null, windKmh: 9, rainMm: null } }))
      .toEqual({ kind: 'values', parts: ['9 km/h'] })
  })
})

describe('toApplicationRowView', () => {
  it('builds the row of the Figma list', () => {
    expect(toApplicationRowView(summary)).toEqual({
      id: 'a-5',
      status: { value: 'completed', label: 'Concluída', tone: 'suitable' },
      dateLabel: 'Sáb 21/11',
      timeLabel: '06:12 – 09:40',
      fieldCode: 'T5',
      fieldName: 'Talhão 5',
      fieldMeta: 'Catuaí 144 · Área Centro',
      productLabel: 'Inseticida',
      doseLabel: '0,3 L/ha · calda 180 L/ha',
      areaLabel: '56,3 ha',
      conditions: { kind: 'values', parts: ['4,2 °C', '6 km/h'] },
      operatorName: 'Carlos',
      operatorInitials: 'CM',
      operatorFullName: 'Carlos Mendes',
    })
  })

  it('has no meta without variety and block', () => {
    expect(toApplicationRowView({ ...summary, field: { ...summary.field, variety: null, block: null } }).fieldMeta).toBeNull()
    expect(toApplicationRowView({ ...summary, field: { ...summary.field, block: null } }).fieldMeta).toBe('Catuaí 144')
  })
})

const detail: ApplicationDetail = {
  ...summary,
  registeredAt: new Date('2026-11-21T12:52:00Z'),
  recordedConditions: { headline: 'Dentro da janela', deltaTC: 4.2, windKmh: 6, rainMm: 0, rainHours: 8, durationMinutes: 208 },
  usage: { lot: '2409-B', doseTotalL: 16.9, mixTotalL: 10134, tankCount: 6, tankCapacityL: 2000 },
  preHarvest: { days: 14, releasedFrom: '2026-12-05' },
  equipment: { id: 'e', name: 'Turbo 2.000 L · bico cone vazio' },
  confirmedBy: { person: { id: 'user-marina', name: 'Marina Rezende' }, at: new Date('2026-11-20T20:40:00Z') },
  signed: true,
  cancellation: null,
}

describe('buildApplicationDetailView', () => {
  it('builds the panel of the Figma (T5)', () => {
    const view = buildApplicationDetailView(detail)

    expect(view.status.label).toBe('Concluída')
    expect(view.registeredLabel).toBe('Registrada em 21/11 às 09:52')
    expect(view.title).toBe('Inseticida · Talhão 5')
    expect(view.subtitle).toBe('Sáb 21/11 · 06:12 – 09:40 · 56,3 ha · Catuaí 144')
    expect(view.conditions).toEqual({
      headline: 'Dentro da janela',
      stats: [
        { key: 'delta_t', label: 'Delta T', value: '4,2 °C' },
        { key: 'wind', label: 'Vento', value: '6 km/h' },
        { key: 'rain', label: 'Chuva 8h', value: '0 mm' },
        { key: 'duration', label: 'Duração', value: '3h28' },
      ],
    })
    expect(view.productRows).toEqual([
      { label: 'Produto', value: 'Inseticida · lote 2409-B' },
      { label: 'Dose', value: '0,3 L/ha · 16,9 L no total' },
      { label: 'Calda', value: '180 L/ha · 10.134 L · 6 tanques' },
      { label: 'Carência', value: '14 dias · colheita liberada a partir de 05/12' },
    ])
    expect(view.teamRows).toEqual([
      { label: 'Operador', value: 'Carlos Mendes' },
      { label: 'Pulverizador', value: 'Turbo 2.000 L · bico cone vazio' },
      { label: 'Confirmado por', value: 'Marina Rezende · 20/11 às 17:40' },
    ])
    expect(view.signatureLabel).toBe('Registro assinado digitalmente · válido para o caderno de campo')
    expect(view.cancellationLabel).toBeNull()
  })

  it('handles a planned application: nothing recorded, no lot, no release date, not signed', () => {
    const view = buildApplicationDetailView({
      ...detail,
      status: 'planned',
      recordedConditions: null,
      usage: { ...detail.usage, lot: null, tankCount: 1 },
      preHarvest: { days: 1, releasedFrom: null },
      signed: false,
      confirmedBy: null,
    })

    expect(view.conditions).toBeNull()
    expect(view.productRows[0]).toEqual({ label: 'Produto', value: 'Inseticida' })
    expect(view.productRows[2]?.value).toBe('180 L/ha · 10.134 L · 1 tanque')
    expect(view.productRows[3]).toEqual({ label: 'Carência', value: '1 dia' })
    expect(view.teamRows.map(row => row.label)).toEqual(['Operador', 'Pulverizador'])
    expect(view.signatureLabel).toBeNull()
  })

  it('shows why a cancelled one was cancelled and omits carência when there is none', () => {
    const view = buildApplicationDetailView({ ...detail, status: 'cancelled', preHarvest: null, cancellation: { reason: 'Chuva de 18 mm prevista na janela' } })
    expect(view.cancellationLabel).toBe('Chuva de 18 mm prevista na janela')
    expect(view.productRows.map(row => row.label)).toEqual(['Produto', 'Dose', 'Calda'])
  })
})
