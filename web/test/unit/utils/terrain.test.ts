import { describe, expect, it } from 'vitest'
import type { Field } from '~/types/field'
import type {
  FarmTerrain,
  FieldTerrain,
  FieldTerrainSummary,
  MethodFit,
  TerrainMeasurement,
} from '~/types/terrain'
import { groupFieldsByBlock } from '~/utils/field'
import {
  elevationBandOf,
  elevationRangeOf,
  emptyTerrainAdjustmentValues,
  formatElevationRange,
  formatSlopePct,
  methodFitWarning,
  normalizeProfile,
  parseDecimalInput,
  terrainAdjustmentErrorsFromApi,
  terrainFailures,
  terrainFieldIdFrom,
  terrainLegend,
  terrainMapKey,
  terrainTabState,
  terrainViewFrom,
  toFarmTerrainIndicators,
  toFieldTerrainDetailView,
  toTerrainAdjustmentHints,
  toTerrainAdjustmentInput,
  toTerrainBlockViews,
  toTerrainMapFeatures,
  toTerrainMissingBanner,
  toTerrainNotCalculatedView,
  toTerrainPanelHeader,
  toTerrainPopoverView,
  toTerrainProgressView,
  validateTerrainAdjustment,
  weightedSlopeMean,
  indexTerrainSummaries,
  type ElevationRange,
  type TerrainAdjustmentFormValues,
} from '~/utils/terrain'

const polygon = { type: 'Polygon', coordinates: [] } as const

function field(id: string, name: string, block: string | null, areaHa: number, variety: string | null = 'Arara'): Field {
  return {
    id,
    farmId: 'farm-1',
    name,
    block,
    areaHa,
    crop: { code: 'coffee_arabica', name: 'Café arábica' },
    variety,
    season: null,
    plantingDate: null,
    rowSpacingM: null,
    plantSpacingM: null,
    plantsPerHa: null,
    polygon,
    sprayCondition: null,
    lastApplicationAt: null,
  }
}

function measurement(overrides: Partial<TerrainMeasurement> = {}): TerrainMeasurement {
  return {
    slopeMeanPct: 6.24,
    slopeP90Pct: 8.4,
    slopeMaxPct: 9.8,
    slopeClass: 'gently_undulating',
    position: 'slope',
    aspect: 'northwest',
    ...overrides,
  }
}

function summary(fieldId: string, overrides: Partial<FieldTerrainSummary> = {}): FieldTerrainSummary {
  return {
    fieldId,
    fieldName: `Talhão ${fieldId}`,
    block: null,
    areaHa: 10,
    status: 'ready',
    failureReason: null,
    source: 'calculated',
    measurement: measurement(),
    elevationMinM: 900,
    elevationMaxM: 920,
    hasRestrictedMethod: false,
    ...overrides,
  }
}

const notCalculatedSummary = (fieldId: string): FieldTerrainSummary => summary(fieldId, {
  status: 'not_calculated',
  source: null,
  measurement: null,
  elevationMinM: null,
  elevationMaxM: null,
})

function farmTerrain(overrides: Partial<FarmTerrain> = {}): FarmTerrain {
  return {
    farmId: 'farm-1',
    summary: {
      fieldCount: 3,
      calculatedFieldCount: 2,
      slopeMeanPct: 7.94,
      restrictedFieldCount: 1,
      elevationMinM: 872,
      elevationMaxM: 948,
      calculatedAt: new Date('2026-11-12T15:00:00Z'),
    },
    progress: null,
    items: [],
    ...overrides,
  }
}

const fit = (overrides: Partial<MethodFit>): MethodFit => ({
  method: 'tractor_boom',
  slopeLimitPct: 20,
  limitSource: 'reference',
  status: 'compatible',
  areaAboveLimitPct: null,
  ...overrides,
})

describe('formatting', () => {
  it('writes percentages with one decimal and a space before the sign', () => {
    expect(formatSlopePct(23.5)).toBe('23,5 %')
    expect(formatSlopePct(6.24)).toBe('6,2 %')
    expect(formatSlopePct(0)).toBe('0,0 %')
    expect(formatSlopePct(null)).toBe('—')
  })

  it('writes the elevation range with an en dash and whole meters', () => {
    expect(formatElevationRange(872.2, 940.6)).toBe('872–941 m')
    expect(formatElevationRange(900.1, 899.9)).toBe('900 m')
    expect(formatElevationRange(null, 900)).toBe('—')
  })

  it('reads the view and the selected field from the query', () => {
    expect(terrainViewFrom('detail')).toBe('detail')
    expect(terrainViewFrom(['detail', 'map'])).toBe('detail')
    expect(terrainViewFrom('other')).toBe('map')
    expect(terrainViewFrom(undefined)).toBe('map')
    expect(terrainFieldIdFrom(['field-1'])).toBe('field-1')
    expect(terrainFieldIdFrom(undefined)).toBeNull()
  })
})

describe('elevation bands and map keys', () => {
  const range: ElevationRange = { min: 872, max: 948 }

  it('takes the range from the farm summary only when both ends exist', () => {
    expect(elevationRangeOf(farmTerrain().summary)).toEqual(range)
    expect(elevationRangeOf({ elevationMinM: 872, elevationMaxM: null })).toBeNull()
    expect(elevationRangeOf(null)).toBeNull()
  })

  it('puts the mean elevation of a field in one of the equal bands', () => {
    expect(elevationBandOf({ elevationMinM: 872, elevationMaxM: 874 }, range, 5)).toBe('elevation-0')
    expect(elevationBandOf({ elevationMinM: 888, elevationMaxM: 896 }, range, 5)).toBe('elevation-1')
    expect(elevationBandOf({ elevationMinM: 946, elevationMaxM: 948 }, range, 5)).toBe('elevation-4')
  })

  it('keeps the top of the range in the last band and clamps values outside it', () => {
    expect(elevationBandOf({ elevationMinM: 948, elevationMaxM: 948 }, range, 5)).toBe('elevation-4')
    expect(elevationBandOf({ elevationMinM: 1000, elevationMaxM: 1100 }, range, 5)).toBe('elevation-4')
    expect(elevationBandOf({ elevationMinM: 500, elevationMaxM: 600 }, range, 5)).toBe('elevation-0')
  })

  it('treats a zero-width range as a single band and a field without elevation as none', () => {
    expect(elevationBandOf({ elevationMinM: 900, elevationMaxM: 900 }, { min: 900, max: 900 }, 5)).toBe('elevation-0')
    expect(elevationBandOf({ elevationMinM: null, elevationMaxM: null }, range, 5)).toBeNull()
    expect(elevationBandOf({ elevationMinM: 900, elevationMaxM: 910 }, null, 5)).toBeNull()
  })

  it('chooses the key by layer', () => {
    const adjusted = summary('a', { source: 'manual', measurement: measurement({ aspect: null }), elevationMinM: null, elevationMaxM: null })
    expect(terrainMapKey('slope', summary('a'), range)).toBe('gently_undulating')
    expect(terrainMapKey('aspect', summary('a'), range)).toBe('northwest')
    expect(terrainMapKey('elevation', summary('a', { elevationMinM: 888, elevationMaxM: 896 }), range)).toBe('elevation-1')
    // Aspect and elevation are calculated only: an adjustment has no aspect and no elevation.
    expect(terrainMapKey('aspect', adjusted, range)).toBe('no_aspect')
    expect(terrainMapKey('elevation', adjusted, range)).toBe('not_calculated')
    expect(terrainMapKey('slope', adjusted, range)).toBe('gently_undulating')
    for (const layer of ['slope', 'elevation', 'aspect'] as const) {
      expect(terrainMapKey(layer, notCalculatedSummary('a'), range)).toBe('not_calculated')
      expect(terrainMapKey(layer, null, range)).toBe('not_calculated')
    }
  })

  it('builds map features with a dashed outline for fields without relief', () => {
    const fields = [field('a', 'Talhão 4', 'Área Centro', 44), field('b', 'Talhão 5', 'Área Centro', 56)]
    const features = toTerrainMapFeatures(fields, indexTerrainSummaries([summary('a')]), 'slope', range)
    expect(features).toEqual([
      { id: 'a', geometry: polygon, terrainKey: 'gently_undulating', dashed: false, label: 'T4' },
      { id: 'b', geometry: polygon, terrainKey: 'not_calculated', dashed: true, label: 'T5' },
    ])
  })
})

describe('legends', () => {
  const range: ElevationRange = { min: 872, max: 948 }

  it('always lists the five slope classes and "Não calculado" only when needed', () => {
    const complete = terrainLegend('slope', [summary('a')], range)
    expect(complete.title).toBe('Declividade')
    expect(complete.items.map(item => item.label)).toEqual(['Plano', 'Suave ondulado', 'Ondulado', 'Forte ondulado', 'Montanhoso'])
    const partial = terrainLegend('slope', [summary('a'), null], range)
    expect(partial.items.at(-1)).toEqual({ key: 'not_calculated', label: 'Não calculado' })
  })

  it('lists the nine exposures and the extra entries that apply', () => {
    const manual = summary('b', { source: 'manual', measurement: measurement({ aspect: null }) })
    const legend = terrainLegend('aspect', [summary('a'), manual, notCalculatedSummary('c')], range)
    expect(legend.items).toHaveLength(9 + 2)
    expect(legend.items.slice(-2).map(item => item.label)).toEqual(['Sem exposição', 'Não calculado'])
    expect(terrainLegend('aspect', [summary('a')], range).items).toHaveLength(9)
  })

  it('lists five elevation bands rounded to the meter', () => {
    const legend = terrainLegend('elevation', [summary('a')], range)
    expect(legend.title).toBe('Altitude')
    expect(legend.items.map(item => item.label)).toEqual(['872–887 m', '887–902 m', '902–918 m', '918–933 m', '933–948 m'])
    expect(legend.items[0]!.key).toBe('elevation-0')
  })

  it('collapses a zero-width range into one band and shows no bands without a range', () => {
    expect(terrainLegend('elevation', [summary('a')], { min: 900, max: 900 }).items).toEqual([{ key: 'elevation-0', label: '900 m' }])
    expect(terrainLegend('elevation', [notCalculatedSummary('a')], null).items).toEqual([{ key: 'not_calculated', label: 'Não calculado' }])
  })
})

describe('list of blocks (04b)', () => {
  const fields = [
    field('f1', 'Talhão 1', 'Área Norte', 100),
    field('f2', 'Talhão 2', 'Área Norte', 50),
    field('f3', 'Talhão 3', 'Área Centro', 44),
    field('f4', 'Talhão 4', null, 20),
  ]
  const blocks = groupFieldsByBlock(fields)

  it('reuses the block keys and order of "Áreas e talhões"', () => {
    const views = toTerrainBlockViews(blocks, new Map(), new Set(), null)
    expect(views.map(block => [block.key, block.name])).toEqual([
      ['Área Centro', 'Área Centro'],
      ['Área Norte', 'Área Norte'],
      ['__no-block__', 'Sem área'],
    ])
  })

  it('averages the slope by area and tells about fields without relief', () => {
    const summaries = indexTerrainSummaries([
      summary('f1', { measurement: measurement({ slopeMeanPct: 6 }) }),
      summary('f2', { measurement: measurement({ slopeMeanPct: 12 }) }),
      notCalculatedSummary('f3'),
    ])
    const [centro, norte, semArea] = toTerrainBlockViews(blocks, summaries, new Set(['Área Norte']), 'f2')
    // (6 * 100 + 12 * 50) / 150 = 8
    expect(norte).toMatchObject({ areaLabel: '150 ha · média 8,0 %', collapsedLabel: '150 ha · média 8,0 %', expanded: false })
    expect(centro).toMatchObject({ areaLabel: '44 ha · sem relevo', collapsedLabel: '44 ha · sem relevo', expanded: true })
    expect(semArea!.fields[0]!.detail).toBe('Relevo não calculado')
    expect(norte!.fields.find(row => row.selected)?.id).toBe('f2')
  })

  it('says how many fields of a closed block have no relief when others do', () => {
    const summaries = indexTerrainSummaries([summary('f1', { measurement: measurement({ slopeMeanPct: 6 }) })])
    const norte = toTerrainBlockViews(blocks, summaries, new Set(), null)[1]!
    expect(norte.collapsedLabel).toBe('150 ha · 1 talhão sem relevo')
    expect(norte.areaLabel).toBe('150 ha · média 6,0 %')
  })

  it('describes each row by status', () => {
    const summaries = indexTerrainSummaries([
      summary('f1'),
      summary('f2', { status: 'calculating' }),
      summary('f3', { status: 'failed', source: null, measurement: null, failureReason: 'sem bloco' }),
      summary('f4', { source: 'manual' }),
    ])
    const rows = toTerrainBlockViews(blocks, summaries, new Set(), null).flatMap(block => block.fields)
    const byId = Object.fromEntries(rows.map(row => [row.id, row]))
    expect(byId.f1).toMatchObject({ detail: '6,2 % · máx. 9,8 % · encosta', badge: { key: 'gently_undulating', label: 'Suave ondulado' }, colorKey: 'gently_undulating', isAdjusted: false, isFailed: false })
    expect(byId.f2).toMatchObject({ detail: 'Calculando…', isFailed: false })
    expect(byId.f3).toMatchObject({ detail: 'Falha no cálculo', isFailed: true, badge: { key: 'not_calculated', label: 'Não calculado' } })
    expect(byId.f4).toMatchObject({ isAdjusted: true })
  })

  it('computes the weighted mean only over fields with relief', () => {
    expect(weightedSlopeMean([])).toBeNull()
    expect(weightedSlopeMean([{ areaHa: 10, measurement: null }])).toBeNull()
    expect(weightedSlopeMean([
      { areaHa: 30, measurement: measurement({ slopeMeanPct: 10 }) },
      { areaHa: 10, measurement: measurement({ slopeMeanPct: 2 }) },
      { areaHa: 99, measurement: null },
    ])).toBe(8)
  })
})

describe('farm header and indicators', () => {
  it('shows the model and the date of the latest calculation', () => {
    expect(toTerrainPanelHeader(farmTerrain())).toEqual({
      title: 'Relevo por talhão',
      subtitle: 'Modelo de elevação Copernicus 30 m · calculado em 12/11/2026',
    })
    expect(toTerrainPanelHeader(null).subtitle).toBe('Modelo de elevação Copernicus 30 m')
  })

  it('formats the three indicators and shows dashes without relief', () => {
    expect(toFarmTerrainIndicators(farmTerrain())).toEqual({
      slopeMean: { label: 'Declividade média', value: '7,9 %', hint: null },
      restricted: { label: 'Com restrição', value: '1 talhão', hint: null },
      elevation: { label: 'Altitude', value: '872–948 m', hint: null },
    })
    expect(toFarmTerrainIndicators(farmTerrain({ summary: { ...farmTerrain().summary, restrictedFieldCount: 3 } })).restricted.value).toBe('3 talhões')
    const empty = toFarmTerrainIndicators(farmTerrain({ summary: { ...farmTerrain().summary, calculatedFieldCount: 0, slopeMeanPct: null, elevationMinM: null, elevationMaxM: null } }))
    expect([empty.slopeMean.value, empty.restricted.value, empty.elevation.value]).toEqual(['—', '—', '—'])
    expect(toFarmTerrainIndicators(null).slopeMean.value).toBe('—')
  })
})

describe('method fit', () => {
  it('writes the warning from the API fits, restricted first', () => {
    const warning = methodFitWarning([
      fit({ method: 'knapsack', slopeLimitPct: null }),
      fit({ method: 'self_propelled', status: 'unsuitable', slopeLimitPct: 12, areaAboveLimitPct: 82 }),
      fit({ method: 'tractor_boom', status: 'restricted', areaAboveLimitPct: 62 }),
    ])
    expect(warning).toBe('Restrito para tratorizado de barra (62 % da área acima de 20 %). Inadequado para autopropelido.')
  })

  it('joins several methods and omits the share without a distribution', () => {
    expect(methodFitWarning([
      fit({ method: 'tractor_boom', status: 'restricted', areaAboveLimitPct: 10 }),
      fit({ method: 'airblast', status: 'restricted', slopeLimitPct: 25, areaAboveLimitPct: null }),
    ])).toBe('Restrito para tratorizado de barra (10 % da área acima de 20 %) e turbopulverizador.')
  })

  it('has no warning when every method is compatible', () => {
    expect(methodFitWarning([fit({})])).toBeNull()
    expect(methodFitWarning([])).toBeNull()
  })
})

describe('popover', () => {
  const talhao = field('f4', 'Talhão 4', 'Área Centro', 44)
  const calculated = summary('f4', { elevationMinM: 872, elevationMaxM: 941, hasRestrictedMethod: true })

  it('builds the summary of the selected field', () => {
    const view = toTerrainPopoverView(talhao, calculated, null)
    expect(view).toMatchObject({
      id: 'f4',
      name: 'Talhão 4',
      subtitle: 'Área Centro · Arara · 44,0 ha',
      badge: { key: 'gently_undulating' },
      slopeMean: { label: 'Média', value: '6,2 %' },
      slopeMax: { label: 'Máxima', value: '9,8 %' },
      relief: { label: 'Desnível', value: '69 m' },
      canOpenDetail: true,
    })
  })

  it('uses the generic warning while the detail loads, and the detail text once it is there', () => {
    const generic = toTerrainPopoverView(talhao, calculated, null).methodWarning
    expect(generic).toMatch(/restrição/)
    expect(toTerrainPopoverView(talhao, summary('f4'), null).methodWarning).toBeNull()
    const detail = { methodFits: [fit({ status: 'restricted', areaAboveLimitPct: 62 })] } as unknown as FieldTerrain
    expect(toTerrainPopoverView(talhao, calculated, detail).methodWarning).toBe('Restrito para tratorizado de barra (62 % da área acima de 20 %).')
    const clean = { methodFits: [fit({})] } as unknown as FieldTerrain
    expect(toTerrainPopoverView(talhao, calculated, clean).methodWarning).toBeNull()
  })

  it('shows dashes without relief and a block-less subtitle', () => {
    const view = toTerrainPopoverView(field('x', 'Talhão 9', null, 12, null), notCalculatedSummary('x'), null)
    expect(view.subtitle).toBe('Café arábica · 12,0 ha')
    expect([view.slopeMean.value, view.relief.value]).toEqual(['—', '—'])
    expect(view.badge).toEqual({ key: 'not_calculated', label: 'Não calculado' })
  })
})

describe('profile', () => {
  it('normalizes x to 0..1 from the top and y to 0..1 from the lowest', () => {
    expect(normalizeProfile([
      { distanceM: 0, elevationM: 941 },
      { distanceM: 100, elevationM: 900 },
      { distanceM: 200, elevationM: 872 },
    ])).toEqual([{ x: 0, y: 1 }, { x: 0.5, y: 28 / 69 }, { x: 1, y: 0 }])
  })

  it('sorts by distance and survives a flat or zero-length profile', () => {
    expect(normalizeProfile([{ distanceM: 50, elevationM: 10 }, { distanceM: 0, elevationM: 10 }])).toEqual([{ x: 0, y: 0.5 }, { x: 1, y: 0.5 }])
    expect(normalizeProfile([{ distanceM: 0, elevationM: 5 }, { distanceM: 0, elevationM: 9 }])).toEqual([{ x: 0, y: 0 }, { x: 1, y: 1 }])
  })

  it('needs at least two points', () => {
    expect(normalizeProfile([])).toBeNull()
    expect(normalizeProfile([{ distanceM: 0, elevationM: 1 }])).toBeNull()
  })
})

function fieldTerrain(overrides: Partial<FieldTerrain> = {}): FieldTerrain {
  const effective = measurement({ slopeMeanPct: 23.5, slopeP90Pct: 31, slopeMaxPct: 41.8, slopeClass: 'strongly_undulating', position: 'slope', aspect: 'northwest' })
  return {
    fieldId: 'f4',
    farmId: 'farm-1',
    status: 'ready',
    failureReason: null,
    source: 'calculated',
    effective,
    calculated: {
      measurement: effective,
      elevationMinM: 872,
      elevationMaxM: 941,
      classAreas: [
        { slopeClass: 'flat', areaHa: 0, sharePct: 0 },
        { slopeClass: 'gently_undulating', areaHa: 3.5, sharePct: 8 },
        { slopeClass: 'undulating', areaHa: 13.2, sharePct: 30 },
        { slopeClass: 'strongly_undulating', areaHa: 22.9, sharePct: 52 },
        { slopeClass: 'mountainous', areaHa: 4.4, sharePct: 10 },
      ],
      profile: [{ distanceM: 0, elevationM: 941 }, { distanceM: 290, elevationM: 872 }],
      elevationModel: { source: 'Copernicus DEM GLO-30', resolutionM: 30, version: '2023_1' },
      calculatedAt: new Date('2026-11-12T15:00:00Z'),
    },
    adjustment: null,
    adjustmentOutdated: false,
    methodFits: [
      fit({ method: 'knapsack', slopeLimitPct: null }),
      fit({ method: 'tractor_boom', status: 'restricted', areaAboveLimitPct: 62 }),
      fit({ method: 'self_propelled', slopeLimitPct: 12, status: 'unsuitable', areaAboveLimitPct: 82 }),
    ],
    ...overrides,
  }
}

describe('field detail (04c)', () => {
  const talhao = field('f4', 'Talhão 4', 'Área Centro', 44)

  it('presents a calculated field', () => {
    const view = toFieldTerrainDetailView(talhao, fieldTerrain())
    expect(view).toMatchObject({
      title: 'Talhão 4',
      badge: { key: 'strongly_undulating', label: 'Forte ondulado' },
      subtitle: 'Área Centro · Arara · 44,0 ha · calculado do Copernicus DEM GLO-30 (30 m) em 12/11/2026',
      failureMessage: null,
      hasAdjustment: false,
      outdatedWarning: null,
      comparison: [],
      adjustmentNote: null,
      indicators: {
        slopeMean: { value: '23,5 %' },
        slopeP90: { label: '90 % da área até', value: '31,0 %' },
        slopeMax: { value: '41,8 %' },
        elevation: { value: '872–941 m', hint: 'desnível de 69 m' },
      },
      profile: { topLabel: '941 m · topo', baseLabel: '872 m · base (290 m)' },
      facts: [
        { label: 'Posição', value: 'Encosta' },
        { label: 'Exposição', value: 'Noroeste' },
        { label: 'Baixada fria', value: 'Não' },
      ],
    })
    expect(view.classAreas[3]).toEqual({ key: 'strongly_undulating', label: 'Forte ondulado', areaLabel: '22,9 ha', shareLabel: '52 %', sharePct: 52 })
    expect(view.methods).toEqual([
      { method: 'knapsack', label: 'Costal', limitLabel: 'sem limite', status: 'compatible', statusLabel: 'Compatível' },
      { method: 'tractor_boom', label: 'Tratorizado de barra', limitLabel: 'até 20 %', status: 'restricted', statusLabel: 'Restrito · 62 % da área' },
      { method: 'self_propelled', label: 'Autopropelido', limitLabel: 'até 12 %', status: 'unsuitable', statusLabel: 'Inadequado' },
    ])
  })

  it('compares calculated and adjusted values and warns when the adjustment is outdated', () => {
    const adjusted = measurement({ slopeMeanPct: 14.2, slopeP90Pct: null, slopeMaxPct: 21.5, slopeClass: 'undulating', position: 'lowland', aspect: null })
    const view = toFieldTerrainDetailView(talhao, fieldTerrain({
      source: 'manual',
      effective: adjusted,
      adjustmentOutdated: true,
      adjustment: {
        origin: 'topographic_survey',
        measurement: adjusted,
        justification: 'Levantamento de 2026',
        adjustedBy: { id: 'u1', name: 'Marina Souza' },
        adjustedAt: new Date('2026-11-20T12:00:00Z'),
      },
    }))
    expect(view.subtitle).toBe('Área Centro · Arara · 44,0 ha · ajustado manualmente por Marina Souza em 20/11/2026')
    expect(view.hasAdjustment).toBe(true)
    expect(view.outdatedWarning).toBe('O cálculo mais recente diverge do ajuste. Revise o ajuste.')
    expect(view.adjustmentNote).toBe('Justificativa: Levantamento de 2026')
    expect(view.comparison).toEqual([
      { label: 'Declividade média', calculated: '23,5 %', adjusted: '14,2 %' },
      { label: 'Declividade máxima', calculated: '41,8 %', adjusted: '21,5 %' },
      { label: 'Posição', calculated: 'Encosta', adjusted: 'Baixada' },
      { label: 'Classe', calculated: 'Forte ondulado', adjusted: 'Ondulado' },
    ])
    expect(view.indicators.slopeP90.value).toBe('—')
    expect(view.facts.map(row => row.value)).toEqual(['Baixada', '—', 'Sim'])
  })

  it('shows the failure and dashes for a field that failed', () => {
    const view = toFieldTerrainDetailView(talhao, fieldTerrain({
      status: 'failed',
      failureReason: 'Bloco indisponível',
      source: null,
      effective: null,
      calculated: null,
      methodFits: [],
    }))
    expect(view.failureMessage).toBe('Falha no cálculo: Bloco indisponível')
    expect(view.badge).toEqual({ key: 'not_calculated', label: 'Não calculado' })
    expect(view.subtitle).toBe('Área Centro · Arara · 44,0 ha')
    expect(view.profile).toBeNull()
    expect(view.classAreas).toEqual([])
    expect(view.methods).toEqual([])
    expect(view.indicators.elevation).toMatchObject({ value: '—', hint: null })
    expect(toFieldTerrainDetailView(talhao, fieldTerrain({ status: 'failed', failureReason: null })).failureMessage).toBe('Falha no cálculo: Motivo não informado')
  })
})

describe('states (04e)', () => {
  it('offers the calculation only with registry.write', () => {
    expect(toTerrainNotCalculatedView(14, true)).toMatchObject({
      title: 'Relevo ainda não calculado',
      calculateLabel: 'Calcular relevo dos 14 talhões',
      noPermissionMessage: null,
    })
    expect(toTerrainNotCalculatedView(1, true).calculateLabel).toBe('Calcular relevo do talhão')
    const readOnly = toTerrainNotCalculatedView(14, false)
    expect(readOnly.calculateLabel).toBeNull()
    expect(readOnly.noPermissionMessage).toBe('Peça a um responsável pelo cadastro para calcular o relevo.')
    expect(readOnly.items.length).toBeGreaterThan(0)
  })

  it('shows progress as text and percent', () => {
    expect(toTerrainProgressView({ total: 14, done: 9 })).toEqual({ label: '9 de 14 talhões', percent: 64 })
    expect(toTerrainProgressView({ total: 1, done: 0 })).toEqual({ label: '0 de 1 talhão', percent: 0 })
    expect(toTerrainProgressView({ total: 0, done: 0 }).percent).toBe(0)
  })

  it('lists failures with the API reason or a fallback', () => {
    expect(terrainFailures([
      summary('a'),
      summary('b', { status: 'failed', failureReason: ' sem bloco ', measurement: null }),
      summary('c', { status: 'failed', failureReason: null, measurement: null }),
    ])).toEqual([
      { fieldId: 'b', name: 'Talhão b', reason: 'sem bloco' },
      { fieldId: 'c', name: 'Talhão c', reason: 'Motivo não informado' },
    ])
  })

  it('banners fields without relief only when there are some', () => {
    expect(toTerrainMissingBanner(2, true)).toEqual({ label: '2 talhões sem relevo', calculateLabel: 'Calcular' })
    expect(toTerrainMissingBanner(1, false)).toEqual({ label: '1 talhão sem relevo', calculateLabel: null })
    expect(toTerrainMissingBanner(0, true)).toBeNull()
  })

  it('decides the state of the tab', () => {
    const ready = farmTerrain()
    const none = farmTerrain({ summary: { ...ready.summary, calculatedFieldCount: 0 } })
    expect(terrainTabState({ terrain: null, pending: true, hasError: false })).toBe('loading')
    expect(terrainTabState({ terrain: null, pending: false, hasError: false })).toBe('loading')
    expect(terrainTabState({ terrain: null, pending: false, hasError: true })).toBe('error')
    expect(terrainTabState({ terrain: ready, pending: false, hasError: true })).toBe('ready')
    expect(terrainTabState({ terrain: none, pending: false, hasError: false })).toBe('not_calculated')
    expect(terrainTabState({ terrain: { ...ready, progress: { total: 3, done: 1 } }, pending: false, hasError: false })).toBe('calculating')
    expect(terrainTabState({ terrain: { ...none, progress: { total: 3, done: 0 } }, pending: false, hasError: false })).toBe('calculating')
  })
})

describe('adjustment form (04d)', () => {
  const valid: TerrainAdjustmentFormValues = {
    origin: 'topographic_survey',
    slopeMeanPct: '23,5',
    slopeMaxPct: '41.8',
    position: 'slope',
    justification: '  Levantamento com RTK  ',
  }

  it('parses decimals with comma or dot and rejects anything else', () => {
    expect(parseDecimalInput('23,5')).toBe(23.5)
    expect(parseDecimalInput(' 7.25 ')).toBe(7.25)
    expect(parseDecimalInput('0')).toBe(0)
    for (const text of ['', 'abc', '-3', '1.234,5', '1,2,3', '12%']) expect(parseDecimalInput(text)).toBeNull()
  })

  it('accepts valid values and builds the request body without the class', () => {
    expect(validateTerrainAdjustment(valid)).toEqual({})
    expect(toTerrainAdjustmentInput(valid)).toEqual({
      origin: 'topographic_survey',
      slopeMeanPct: 23.5,
      slopeMaxPct: 41.8,
      position: 'slope',
      justification: 'Levantamento com RTK',
    })
  })

  it('accepts the limits 0 and 100 and a maximum equal to the mean', () => {
    expect(validateTerrainAdjustment({ ...valid, slopeMeanPct: '0', slopeMaxPct: '100' })).toEqual({})
    expect(validateTerrainAdjustment({ ...valid, slopeMeanPct: '30', slopeMaxPct: '30' })).toEqual({})
  })

  it('requires every field', () => {
    const errors = validateTerrainAdjustment({ origin: 'field_assessment', slopeMeanPct: '', slopeMaxPct: ' ', position: null, justification: '   ' })
    expect(Object.keys(errors).sort()).toEqual(['justification', 'position', 'slopeMaxPct', 'slopeMeanPct'])
    expect(toTerrainAdjustmentInput({ origin: 'field_assessment', slopeMeanPct: '', slopeMaxPct: '', position: null, justification: '' })).toBeNull()
  })

  it('mirrors the contract ranges', () => {
    expect(validateTerrainAdjustment({ ...valid, slopeMeanPct: '101' }).slopeMeanPct).toBe('Use um valor entre 0 e 100.')
    expect(validateTerrainAdjustment({ ...valid, slopeMeanPct: '-1' }).slopeMeanPct).toBe('Use um número, como 23,5.')
    expect(validateTerrainAdjustment({ ...valid, slopeMaxPct: '10', slopeMeanPct: '12' }).slopeMaxPct).toBe('A máxima não pode ser menor que a média.')
    expect(validateTerrainAdjustment({ ...valid, justification: 'a'.repeat(1000) })).toEqual({})
    expect(validateTerrainAdjustment({ ...valid, justification: 'a'.repeat(1001) }).justification).toBe('Use no máximo 1000 caracteres.')
    expect(validateTerrainAdjustment({ ...valid, origin: 'other' as never }).origin).toBeDefined()
  })

  it('does not compare the maximum with a mean that is invalid', () => {
    expect(validateTerrainAdjustment({ ...valid, slopeMeanPct: 'x', slopeMaxPct: '5' })).toEqual({ slopeMeanPct: 'Use um número, como 23,5.' })
  })

  it('starts from the adjustment, else from the calculated value, else empty', () => {
    expect(emptyTerrainAdjustmentValues(null)).toEqual({ origin: 'field_assessment', slopeMeanPct: '', slopeMaxPct: '', position: null, justification: '' })
    expect(emptyTerrainAdjustmentValues(fieldTerrain())).toEqual({
      origin: 'field_assessment',
      slopeMeanPct: '23,5',
      slopeMaxPct: '41,8',
      position: 'slope',
      justification: '',
    })
    const adjusted = measurement({ slopeMeanPct: 14.2, slopeMaxPct: 21.5, position: 'lowland' })
    expect(emptyTerrainAdjustmentValues(fieldTerrain({
      adjustment: { origin: 'topographic_survey', measurement: adjusted, justification: 'RTK', adjustedBy: { id: 'u', name: 'M' }, adjustedAt: new Date() },
    }))).toEqual({ origin: 'topographic_survey', slopeMeanPct: '14,2', slopeMaxPct: '21,5', position: 'lowland', justification: 'RTK' })
  })

  it('shows the calculated values and the current class, never computing a class', () => {
    expect(toTerrainAdjustmentHints(fieldTerrain())).toEqual({
      slopeMean: 'Calculado: 23,5 %',
      slopeMax: 'Calculado: 41,8 %',
      position: 'Calculado: Encosta',
      currentClass: 'Classe atual: Forte ondulado',
      classNotice: 'A classe é recalculada ao salvar.',
    })
    expect(toTerrainAdjustmentHints(null)).toMatchObject({ slopeMean: null, position: null, currentClass: 'Classe atual: Não calculado' })
  })

  it('maps the 422 details of the API to form fields', () => {
    expect(terrainAdjustmentErrorsFromApi([
      { field: 'slope_max_pct', message: 'máxima menor que a média' },
      { field: 'body.justification', message: 'obrigatória' },
      { field: 'slope_max_pct', message: 'segunda mensagem ignorada' },
      { field: 'unknown_field', message: 'x' },
    ])).toEqual({ slopeMaxPct: 'máxima menor que a média', justification: 'obrigatória' })
    expect(terrainAdjustmentErrorsFromApi([])).toEqual({})
  })
})
