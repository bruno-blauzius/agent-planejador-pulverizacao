import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFieldForm } from '~/composables/useFieldForm'
import { useFlash } from '~/composables/useFlash'
import type { GeoJsonPolygon } from '~/types/geo'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

function square(lng: number, lat: number, side = 0.004): GeoJsonPolygon {
  return { type: 'Polygon', coordinates: [[[lng, lat], [lng + side, lat], [lng + side, lat + side], [lng, lat + side], [lng, lat]]] }
}
// The Santa Rita perimeter of the seed is centered on (-46.71, -21.3).
const INSIDE = square(-46.712, -21.302)
const OUTSIDE = square(-46.5, -21.3)

function setup(query: Record<string, unknown> = {}): NuxtFakes {
  const fakes = installNuxtFakes(createTestApi().api)
  fakes.route.query = query
  return fakes
}

describe('useFieldForm (create)', () => {
  it('starts on the drawing step with the block from the query', async () => {
    setup({ block: 'Área Sul' })
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()

    expect(form.mode.value).toBe('create')
    expect(form.step.value).toBe('draw')
    expect(form.values.block).toBe('Área Sul')
    expect(form.eyebrow.value).toBe('NOVO TALHÃO · ÁREA SUL')
    expect(form.namePlaceholder.value).toBe('Talhão 10')
    expect(form.header.value.breadcrumbs).toEqual([
      { label: 'Fazendas', to: '/farms' },
      { label: 'Fazenda Santa Rita', to: '/farms/farm-santa-rita' },
      { label: 'Novo talhão' },
    ])
    expect(form.tabs.value.find(tab => tab.value === 'fields')?.count).toBe(9)
    expect(form.areaPreview.value.label).toBe('—')
    expect(form.contextFeatures.value).toHaveLength(9)
    expect(form.contextFeatures.value.find(feature => feature.id === 'field-str-3')).toMatchObject({ label: 'T3', status: 'suitable' })
    expect(form.cancelTo.value).toBe('/farms/farm-santa-rita')
  })

  it('previews the drawing, moves to the details and undoes', async () => {
    setup()
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()

    form.polygon.value = INSIDE
    expect(form.step.value).toBe('details')
    expect(form.vertexCount.value).toBe(4)
    expect(form.drawHint.value).toBe('Arraste os vértices para ajustar · 4 pontos')
    expect(form.areaPreview.value).toMatchObject({ approximate: true, caption: 'aproximados, calculados a partir do desenho' })
    expect(form.areaPreview.value.label).toMatch(/^\d+,\d ha$/)
    expect(form.perimeterWarning.value).toBeNull()
    expect(form.steps.value).toEqual([
      { value: 'draw', label: 'Desenho', done: true, active: false },
      { value: 'details', label: '2 · Detalhes', done: false, active: true },
    ])

    form.polygon.value = OUTSIDE
    expect(form.perimeterWarning.value).toContain('fora do perímetro')
    form.undo()
    expect(form.polygon.value).toEqual(INSIDE)
    expect(form.step.value).toBe('details')
    form.undo()
    expect(form.polygon.value).toBeNull()
    expect(form.step.value).toBe('draw')
    expect(form.canUndo.value).toBe(false)

    form.polygon.value = INSIDE
    expect(form.step.value).toBe('details')
    form.clearDrawing()
    expect(form.step.value).toBe('draw')
    expect(form.canUndo.value).toBe(true)
  })

  it('does not go to the details without a drawing', async () => {
    setup()
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()
    expect(form.goToDetails()).toBe(false)
    expect(form.fieldErrors.value.polygon).toContain('Desenhe o talhão')
  })

  it('blocks the submit with local errors', async () => {
    const fakes = setup()
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()

    expect(await form.submit()).toBe(false)
    expect(Object.keys(form.fieldErrors.value).sort()).toEqual(['cropCode', 'name', 'polygon'])
    expect(fakes.navigations).toEqual([])
  })

  it('creates the field, leaves a flash and goes back with it selected', async () => {
    const fakes = setup({ block: 'Área Sul' })
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()

    form.polygon.value = INSIDE
    Object.assign(form.values, { name: 'Talhão 10', variety: 'Catuaí 144', plantingDate: '2026-10-15', spacing: '3,5 × 0,7' })
    form.selectCrop('coffee_arabica')

    expect(await form.submit()).toBe(true)
    const flash = useFlash().consume()
    expect(flash).toMatchObject({ kind: 'success', title: 'Talhão 10 cadastrado' })
    expect(flash?.description).toMatch(/^Área de \d+,\d ha\.$/)
    expect(fakes.navigations).toEqual([`/farms/farm-santa-rita?field=${encodeURIComponent(flash?.highlightId ?? '')}`])
  })

  it('shows API field errors until the user edits the form', async () => {
    const fakes = setup()
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()

    form.polygon.value = INSIDE
    form.values.name = 'Talhão 10'
    form.selectCrop('banana')

    expect(await form.submit()).toBe(false)
    expect(form.fieldErrors.value.cropCode).toBe('cultura desconhecida')
    expect(form.formError.value).toBeNull()
    expect(fakes.navigations).toEqual([])

    form.values.name = 'Talhão 11'
    await flushPromises()
    expect(form.fieldErrors.value.cropCode).toBeUndefined()
  })
})

describe('useFieldForm (edit)', () => {
  it('fills the form from the field and keeps the API area until the drawing changes', async () => {
    setup()
    const form = useFieldForm({ farmId: 'farm-santa-rita', fieldId: 'field-str-3' })
    await flushPromises()

    expect(form.mode.value).toBe('edit')
    expect(form.step.value).toBe('details')
    expect(form.values).toMatchObject({ name: 'Talhão 3', block: 'Área Norte', cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2019-03-20' })
    expect(form.eyebrow.value).toBe('EDITAR TALHÃO · ÁREA NORTE')
    expect(form.header.value.breadcrumbs.at(-1)).toEqual({ label: 'Talhão 3' })
    expect(form.areaPreview.value).toEqual({ label: '38,6 ha', caption: 'área do cadastro', approximate: false })
    expect(form.canUndo.value).toBe(false)
    expect(form.contextFeatures.value.map(feature => feature.id)).not.toContain('field-str-3')
    expect(form.cancelTo.value).toBe('/farms/farm-santa-rita?field=field-str-3')

    expect(form.standHint.value).toMatch(/^≈ [\d.]+ plantas\/ha · usado no cálculo de calda$/)
    form.values.spacing = '4 × 1'
    expect(form.standHint.value).toBe('Estande calculado ao salvar · usado no cálculo de calda')

    form.polygon.value = INSIDE
    expect(form.areaPreview.value.approximate).toBe(true)
  })

  it('updates the field and goes back to it', async () => {
    const fakes = setup()
    const form = useFieldForm({ farmId: 'farm-santa-rita', fieldId: 'field-str-3' })
    await flushPromises()

    form.values.name = 'Talhão 3A'
    expect(await form.submit()).toBe(true)
    expect(useFlash().consume()).toMatchObject({ title: 'Talhão 3A atualizado', description: 'Área de 38,6 ha.', highlightId: 'field-str-3' })
    expect(fakes.navigations).toEqual(['/farms/farm-santa-rita?field=field-str-3'])
  })

  it('reports a field that is not in the farm as not found', async () => {
    setup()
    const form = useFieldForm({ farmId: 'farm-recanto', fieldId: 'field-str-3' })
    await flushPromises()
    expect(form.isNotFound.value).toBe(true)
  })
})
