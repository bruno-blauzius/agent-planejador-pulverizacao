import { describe, expect, it } from 'vitest'
import type { Field } from '~/types/field'
import type { GeoJsonPolygon } from '~/types/geo'
import {
  cropChips,
  drawHintFor,
  emptyFieldFormValues,
  fieldErrorsFromApi,
  fieldToFormValues,
  nextFieldName,
  parseSpacing,
  toFieldInput,
  validateFieldForm,
} from '~/utils/fieldForm'

const polygon: GeoJsonPolygon = { type: 'Polygon', coordinates: [[[0, 0], [1, 0], [1, 1], [0, 0]]] }
const CROPS = [
  { code: 'coffee_arabica', name: 'Café arábica' },
  { code: 'coffee_conilon', name: 'Café conilon' },
  { code: 'soybean', name: 'Soja' },
  { code: 'corn', name: 'Milho' },
  { code: 'corn_second_season', name: 'Milho safrinha' },
  { code: 'wheat', name: 'Trigo' },
]

describe('fieldForm utils', () => {
  it('parses the spacing in the formats people type', () => {
    expect(parseSpacing('3,5 × 0,7')).toEqual({ rowSpacingM: 3.5, plantSpacingM: 0.7 })
    expect(parseSpacing('3.5x0.7')).toEqual({ rowSpacingM: 3.5, plantSpacingM: 0.7 })
    expect(parseSpacing(' 4 * 1 ')).toEqual({ rowSpacingM: 4, plantSpacingM: 1 })
    expect(parseSpacing('')).toEqual({ rowSpacingM: null, plantSpacingM: null })
    expect(parseSpacing('3,5')).toBeUndefined()
    expect(parseSpacing('0 × 0,7')).toBeUndefined()
  })

  it('validates the obvious mistakes only', () => {
    expect(validateFieldForm(emptyFieldFormValues(), null)).toEqual({
      name: 'Informe o nome do talhão.',
      cropCode: 'Escolha a cultura.',
      polygon: 'Desenhe o talhão no mapa, com ao menos 3 pontos.',
    })
    const values = { ...emptyFieldFormValues('Área Sul'), name: 'Talhão 9', cropCode: 'corn', spacing: '3,5', plantingDate: '2026-02-30' }
    expect(validateFieldForm(values, polygon)).toEqual({
      spacing: 'Use o formato 3,5 × 0,7 (entre linhas × entre plantas, em metros).',
      plantingDate: 'Informe uma data válida.',
    })
  })

  it('builds the payload trimming blanks to null', () => {
    const values = { name: ' Talhão 9 ', block: ' ', cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2026-10-15', spacing: '3,5 × 0,7' }
    expect(toFieldInput(values, polygon, '2026/27')).toEqual({
      name: 'Talhão 9',
      block: null,
      cropCode: 'coffee_arabica',
      variety: 'Catuaí 144',
      season: '2026/27',
      plantingDate: '2026-10-15',
      rowSpacingM: 3.5,
      plantSpacingM: 0.7,
      polygon,
    })
  })

  it('fills the form from a field', () => {
    const field: Field = {
      id: 'field-3',
      farmId: 'farm-1',
      name: 'Talhão 3',
      block: null,
      areaHa: 38.6,
      crop: CROPS[0]!,
      variety: null,
      season: null,
      plantingDate: null,
      rowSpacingM: 3.5,
      plantSpacingM: 0.7,
      plantsPerHa: 4082,
      polygon,
      sprayCondition: null,
      lastApplicationAt: null,
    }
    expect(fieldToFormValues(field)).toEqual({ name: 'Talhão 3', block: '', cropCode: 'coffee_arabica', variety: '', plantingDate: '', spacing: '3,5 × 0,7' })
  })

  it('maps API details to form fields', () => {
    expect(fieldErrorsFromApi([
      { field: 'crop_code', message: 'cultura desconhecida' },
      { field: 'row_spacing_m', message: 'deve ser maior que zero' },
      { field: 'plant_spacing_m', message: 'outro' },
      { field: 'unknown', message: 'x' },
    ])).toEqual({ cropCode: 'cultura desconhecida', spacing: 'deve ser maior que zero' })
  })

  it('shows the featured crops as chips and the others under "+ Outra"', () => {
    const chips = cropChips(CROPS, 'coffee_arabica')
    expect(chips.featured.map(chip => `${chip.icon} ${chip.label}${chip.selected ? '*' : ''}`))
      .toEqual(['☕ Café arábica*', '☕ Café conilon', '🌱 Soja', '🌽 Milho', '🌾 Trigo'])
    expect(chips.others.map(chip => chip.label)).toEqual(['Milho safrinha'])

    const otherSelected = cropChips(CROPS, 'corn_second_season')
    expect(otherSelected.featured.at(-1)).toMatchObject({ label: 'Milho safrinha', selected: true })
    expect(otherSelected.others).toEqual([])
  })

  it('suggests the next field name and the draw hint', () => {
    expect(nextFieldName([{ name: 'Talhão 2' }, { name: 'Talhão 9' }, { name: 'Lote A' }])).toBe('Talhão 10')
    expect(nextFieldName([])).toBe('Talhão 1')
    expect(drawHintFor(0)).toContain('Clique no mapa')
    expect(drawHintFor(5)).toBe('Arraste os vértices para ajustar · 5 pontos')
  })
})
