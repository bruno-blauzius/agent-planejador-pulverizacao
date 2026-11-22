import { describe, expect, it } from 'vitest'
import type { Field } from '~/types/field'
import { groupFieldsByBlock } from '~/utils/field'

function field(name: string, block: string | null, areaHa = 10): Field {
  return {
    id: `${block}-${name}`,
    farmId: 'farm-1',
    name,
    block,
    areaHa,
    crop: { code: 'coffee_arabica', name: 'Café arábica' },
    variety: null,
    season: null,
    plantingDate: null,
    rowSpacingM: null,
    plantSpacingM: null,
    plantsPerHa: null,
    polygon: { type: 'Polygon', coordinates: [] },
    sprayCondition: null,
    lastApplicationAt: null,
  }
}

describe('groupFieldsByBlock', () => {
  it('orders blocks alphabetically, fields naturally and unblocked fields last, whatever the API order', () => {
    const blocks = groupFieldsByBlock([
      field('Talhão 10', 'Área Sul', 5.25),
      field('Talhão 1', null),
      field('Talhão 2', 'Área Sul', 4.1),
      field('Talhão 3', 'área centro'),
      field('Talhão 4', 'Área Norte'),
    ])
    expect(blocks.map(block => block.name)).toEqual(['área centro', 'Área Norte', 'Área Sul', null])
    expect(blocks[2]).toMatchObject({ areaHa: 9.4 })
    expect(blocks[2]!.fields.map(item => item.name)).toEqual(['Talhão 2', 'Talhão 10'])
  })
})
