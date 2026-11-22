import { afterEach, describe, expect, it, vi } from 'vitest'
import { useCrops } from '~/composables/useCrops'
import { useFarmFields } from '~/composables/useFarmFields'
import type { FieldInput } from '~/types/field'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const newField: FieldInput = {
  name: 'Talhão 10',
  block: 'Área Sul',
  cropCode: 'coffee_arabica',
  variety: 'Catuaí 144',
  season: '2026/27',
  plantingDate: '2026-10-15',
  rowSpacingM: 3.5,
  plantSpacingM: 0.7,
  polygon: { type: 'Polygon', coordinates: [[[-46.71, -21.3], [-46.705, -21.3], [-46.705, -21.297], [-46.71, -21.297], [-46.71, -21.3]]] },
}

describe('useFarmFields', () => {
  it('groups the fields of the farm by block', async () => {
    installNuxtFakes(createTestApi().api)
    const { blocks } = useFarmFields('farm-santa-rita')
    await flushPromises()

    expect(blocks.value.map(block => [block.name, block.areaHa, block.fields.length])).toEqual([
      ['Área Centro', 148, 3],
      ['Área Norte', 162, 4],
      ['Área Sul', 102, 2],
    ])
  })

  it('creates a field and refreshes the list', async () => {
    installNuxtFakes(createTestApi().api)
    const { fields, createField, createError } = useFarmFields('farm-santa-rita')
    await flushPromises()

    const created = await createField(newField)

    expect(created?.plantsPerHa).toBe(4082)
    expect(createError.value).toBeNull()
    expect(fields.value.map(field => field.id)).toContain(created?.id)
  })

  it('keeps the list intact when the creation is rejected', async () => {
    installNuxtFakes(createTestApi().api)
    const { fields, createField, createError } = useFarmFields('farm-recanto')
    await flushPromises()

    expect(await createField({ ...newField, cropCode: 'banana' })).toBeNull()

    expect(createError.value?.details).toEqual([{ field: 'crop_code', message: 'cultura desconhecida' }])
    expect(fields.value).toHaveLength(2)
  })
})

describe('useFarmFields.updateField', () => {
  it('updates a field keeping its id and spray condition, and refreshes the list', async () => {
    installNuxtFakes(createTestApi().api)
    const { fields, updateField, updateError } = useFarmFields('farm-santa-rita')
    await flushPromises()

    const updated = await updateField('field-str-3', { ...newField, name: 'Talhão 3A', block: 'Área Norte' })

    expect(updateError.value).toBeNull()
    expect(updated).toMatchObject({ id: 'field-str-3', name: 'Talhão 3A', variety: 'Catuaí 144', sprayCondition: 'suitable' })
    expect(fields.value.find(field => field.id === 'field-str-3')?.name).toBe('Talhão 3A')
    expect(fields.value).toHaveLength(9)
  })

  it('exposes not found for a field of another farm', async () => {
    installNuxtFakes(createTestApi().api)
    const { updateField, updateError } = useFarmFields('farm-recanto')
    await flushPromises()

    expect(await updateField('field-str-3', newField)).toBeNull()
    expect(updateError.value?.code).toBe('not_found')
  })

  it('rejects an invalid input with field details', async () => {
    installNuxtFakes(createTestApi().api)
    const { updateField, updateError } = useFarmFields('farm-santa-rita')
    await flushPromises()

    expect(await updateField('field-str-3', { ...newField, name: ' ' })).toBeNull()
    expect(updateError.value?.details).toEqual([{ field: 'name', message: 'informe o nome do talhão' }])
  })
})

describe('useCrops', () => {
  it('loads the crop catalog', async () => {
    installNuxtFakes(createTestApi().api)
    const { crops } = useCrops()
    await flushPromises()
    expect(crops.value.map(crop => crop.name)).toContain('Café arábica')
  })
})
