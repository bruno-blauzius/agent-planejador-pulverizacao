import { describe, expect, it } from 'vitest'
import type { Farm } from '~/types/farm'
import type { Field } from '~/types/field'
import {
  cultivarColorSlots,
  farmDetailTabFrom,
  toFarmOverviewView,
  toFarmOwnerRows,
  cultivarLabel,
  fieldShortLabel,
  formatFieldArea,
  newFieldRoute,
  statusLegend,
  toFieldBlockView,
  toFieldPopoverView,
  toFieldRowView,
  toFieldsSummaryView,
} from '~/utils/farmDetail'

function field(overrides: Partial<Field> = {}): Field {
  return {
    id: 'field-1',
    farmId: 'farm-1',
    name: 'Talhão 3',
    block: 'Área Norte',
    areaHa: 38.6,
    crop: { code: 'coffee_arabica', name: 'Café arábica' },
    variety: 'Catuaí 144',
    season: null,
    plantingDate: '2019-03-20',
    rowSpacingM: null,
    plantSpacingM: null,
    plantsPerHa: null,
    polygon: { type: 'Polygon', coordinates: [[[0, 0], [1, 0], [1, 1], [0, 0]]] },
    sprayCondition: 'suitable',
    lastApplicationAt: new Date('2026-09-19T12:00:00Z'),
    ...overrides,
  }
}

describe('farmDetail utils', () => {
  it('labels the cultivar by variety, falling back to the crop name', () => {
    expect(cultivarLabel(field())).toBe('Catuaí 144')
    expect(cultivarLabel(field({ variety: null, crop: { code: 'corn_second_season', name: 'Milho safrinha' } }))).toBe('Milho safrinha')
    expect(cultivarLabel(field({ variety: '  ' }))).toBe('Café arábica')
  })

  it('shortens field names for the map tags', () => {
    expect(fieldShortLabel('Talhão 3')).toBe('T3')
    expect(fieldShortLabel('talhao 12B')).toBe('T12B')
    expect(fieldShortLabel('Lote do Rio')).toBe('LDR')
  })

  it('formats field areas with one fixed decimal', () => {
    expect(formatFieldArea(44)).toBe('44,0 ha')
    expect(formatFieldArea(1250.25)).toBe('1.250,3 ha')
  })

  it('assigns one color slot per cultivar in order of appearance', () => {
    const fields = [
      field({ id: 'a', variety: 'Arara' }),
      field({ id: 'b' }),
      field({ id: 'c', variety: 'Arara' }),
      field({ id: 'd', variety: null, crop: { code: 'corn', name: 'Milho' } }),
    ]
    const slots = cultivarColorSlots(fields)

    expect(slots.legend).toEqual([
      { colorIndex: 0, label: 'Arara' },
      { colorIndex: 1, label: 'Catuaí 144' },
      { colorIndex: 2, label: 'Milho' },
    ])
    expect(fields.map(slots.indexOf)).toEqual([0, 1, 0, 2])
  })

  it('lists only the conditions present, in badge order', () => {
    const fields = [field({ sprayCondition: 'rain' }), field({ sprayCondition: null }), field({ sprayCondition: 'suitable' })]
    expect(statusLegend(fields).map(item => item.label)).toEqual(['Apta', 'Chuva'])
  })

  it('builds the row and the block of the list', () => {
    const row = toFieldRowView(field(), 1, 'field-1')
    expect(row).toEqual({
      id: 'field-1',
      name: 'Talhão 3',
      shortLabel: 'T3',
      subtitle: 'Catuaí 144 · 38,6 ha',
      condition: { value: 'suitable', label: 'Apta' },
      colorIndex: 1,
      selected: true,
    })

    const block = toFieldBlockView({ name: null, areaHa: 102, fields: [field(), field()] }, [row], false)
    expect(block).toMatchObject({ name: 'Sem área', areaLabel: '102 ha', collapsedLabel: '102 ha · 2 talhões', expanded: false })
  })

  it('shows dashes in the summary until there is data', () => {
    const blocks = [
      { name: 'Área Norte', areaHa: 162, fields: [field(), field()] },
      { name: null, areaHa: 10.4, fields: [field()] },
    ]
    expect(toFieldsSummaryView(blocks, false)).toEqual({ blockCount: '—', fieldCount: '—', productiveAreaLabel: '—' })
    expect(toFieldsSummaryView(blocks, true)).toEqual({ blockCount: '1', fieldCount: '3', productiveAreaLabel: '172 ha' })
  })

  it('builds the popover without inventing the day window', () => {
    const popover = toFieldPopoverView(field(), new Date('2026-10-01T12:00:00Z'))
    expect(popover).toMatchObject({
      subtitle: 'Área Norte · Catuaí 144',
      areaLabel: '38,6 ha',
      plantingLabel: 'mar/2019',
      lastApplicationLabel: 'há 12 dias',
      todayWindowLabel: null,
      editFieldTo: '/farms/farm-1/fields/field-1/edit',
      planApplication: { disabled: true, hint: 'Disponível com o planejador' },
    })
    expect(toFieldPopoverView(field({ block: null, plantingDate: null, lastApplicationAt: null }), new Date()))
      .toMatchObject({ subtitle: 'Catuaí 144', plantingLabel: '—', lastApplicationLabel: '—' })
  })

  it('routes to the new field screen, encoding the block', () => {
    expect(newFieldRoute('farm 1')).toBe('/farms/farm%201/fields/new')
    expect(newFieldRoute('farm-1', 'Área Norte')).toBe('/farms/farm-1/fields/new?block=%C3%81rea%20Norte')
  })
})

function farm(overrides: Partial<Farm> = {}): Farm {
  return {
    companyId: 'company-1',
    id: 'farm-1',
    name: 'Fazenda Santa Rita',
    code: null,
    city: 'Guaxupé',
    state: 'MG',
    status: 'pending',
    productionType: 'coffee',
    totalAreaHa: 486.2,
    productiveAreaHa: 412,
    fieldCount: 9,
    crops: [{ code: 'coffee_arabica', name: 'Café arábica' }],
    owners: [],
    sprayOutlook: null,
    forecastPoint: { lat: -21.3, lng: -46.71 },
    perimeter: null,
    createdAt: new Date('2026-09-01T12:00:00Z'),
    cooperative: 'Cooxupé',
    location: { postalCode: '37800000', carNumber: null, forecastPoint: null, perimeter: null },
    updatedAt: new Date('2026-09-01T12:00:00Z'),
    ...overrides,
  }
}

describe('farm detail tabs', () => {
  it.each([
    ['overview', 'overview'],
    ['owners', 'owners'],
    ['applications', 'fields'],
    ['windows', 'fields'],
    ['unknown', 'fields'],
    [undefined, 'fields'],
  ])('reads ?tab=%s as %s', (value, expected) => {
    expect(farmDetailTabFrom(value)).toBe(expected)
  })

  it('groups the registry data of the overview, muting what is not filled', () => {
    const [identification, location, areas] = toFarmOverviewView(farm())
    expect(identification?.facts).toContainEqual({ label: 'Apelido / código', value: '—', isEmpty: true })
    expect(identification?.facts).toContainEqual({ label: 'Situação do cadastro', value: 'Com pendências', isEmpty: false })
    expect(location?.facts).toEqual([
      { label: 'Município / UF', value: 'Guaxupé / MG', isEmpty: false },
      { label: 'CEP', value: '37800-000', isEmpty: false },
      { label: 'Nº do CAR', value: '—', isEmpty: true },
      { label: 'Sede (lat · lng)', value: '-21,30 · -46,71', isEmpty: false },
    ])
    expect(areas?.facts).toContainEqual({ label: 'Área total', value: '486,2 ha', isEmpty: false })
    expect(areas?.facts).toContainEqual({ label: 'Culturas', value: 'Café arábica', isEmpty: false })
  })

  it('lists owners first, then tenants and representatives, with the share only for owners', () => {
    const rows = toFarmOwnerRows([
      { ownerId: 'o-3', name: 'Zé Arrendatário', role: 'tenant', sharePercent: null },
      { ownerId: 'o-2', name: 'Marina Rezende', role: 'owner', sharePercent: 40 },
      { ownerId: 'o-1', name: 'Helena Prado', role: 'owner', sharePercent: 60 },
    ])
    expect(rows.map(row => [row.name, row.roleLabel, row.shareLabel, row.to])).toEqual([
      ['Helena Prado', 'Proprietário', '60%', '/owners?owner=o-1'],
      ['Marina Rezende', 'Proprietário', '40%', '/owners?owner=o-2'],
      ['Zé Arrendatário', 'Arrendatário', null, '/owners?owner=o-3'],
    ])
  })
})
