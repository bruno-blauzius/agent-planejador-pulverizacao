import { effectScope, ref, type Ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFarmDetailPage } from '~/composables/useFarmDetailPage'
import { useFarmFields } from '~/composables/useFarmFields'
import { useApiServices } from '~/composables/useApiServices'
import { useFarmTerrainTab } from '~/composables/useFarmTerrainTab'
import { useFieldTerrain } from '~/composables/useFieldTerrain'
import { useTerrainAdjustmentForm } from '~/composables/useTerrainAdjustmentForm'
import { MOCK_SEDE_GAUGE_ID } from '~/services/mock/fieldConditionsMock'
import type { TrafficabilityService } from '~/services/ports'
import { flushPromises, type NuxtFakes } from '../../support/nuxtFakes'
import { SEED_LOGINS, SEED_NOW, signInAs } from '../../support/session'

afterEach(() => {
  vi.unstubAllGlobals()
})

const FARM = 'farm-santa-rita'

async function openTab(login: string = SEED_LOGINS.admin, farmId: string = FARM): Promise<{ fakes: NuxtFakes, tab: ReturnType<typeof useFarmTerrainTab> }> {
  const fakes = await signInAs(login)
  fakes.route.path = `/farms/${farmId}`
  fakes.route.query = { tab: 'terrain' }
  const fields = useFarmFields(farmId)
  const tab = useFarmTerrainTab({ farmId, active: true, fields })
  await flushPromises()
  return { fakes, tab }
}

describe('useFarmTerrainTab: the loaded tab', () => {
  it('starts from the fields and crosses each one with its relief row', async () => {
    const { tab } = await openTab()
    expect(tab.state.value).toBe('ready')
    expect(tab.header.value.subtitle).toBe('Modelo de elevação Copernicus 30 m · calculado em 12/11/2026')
    expect(tab.indicators.value.restricted.value).toMatch(/talh/)

    // Same blocks and order as "Áreas e talhões".
    expect(tab.blocks.value.map(block => block.name)).toEqual(['Área Centro', 'Área Norte', 'Área Sul'])
    const rows = tab.blocks.value.flatMap(block => block.fields)
    expect(rows).toHaveLength(9)
    const talhao4 = rows.find(row => row.name === 'Talhão 4')!
    expect(talhao4).toMatchObject({ badge: { label: 'Forte ondulado' }, colorKey: 'strongly_undulating', detail: '24,3 % · máx. 41,8 % · encosta' })
    expect(rows.find(row => row.name === 'Talhão 2')).toMatchObject({ isAdjusted: true })
    expect(rows.find(row => row.name === 'Talhão 9')).toMatchObject({ isFailed: true, detail: 'Falha no cálculo', badge: { label: 'Não calculado' } })
    expect(tab.failures.value).toEqual([{ fieldId: 'field-str-9', name: 'Talhão 9', reason: 'Bloco do modelo de elevação indisponível para este talhão.' }])
  })

  it('draws the map from the field polygons, with the layer of the control', async () => {
    const { tab } = await openTab()
    expect(tab.mapFeatures.value).toHaveLength(9)
    const feature = (id: string) => tab.mapFeatures.value.find(item => item.id === id)!
    expect(feature('field-str-4')).toMatchObject({ terrainKey: 'strongly_undulating', dashed: false, label: 'T4' })
    expect(feature('field-str-9')).toMatchObject({ terrainKey: 'not_calculated', dashed: true })
    expect(tab.legend.value.title).toBe('Declividade')
    expect(tab.legend.value.items.at(-1)).toEqual({ key: 'not_calculated', label: 'Não calculado' })
    expect(tab.mapLayers.value.map(layer => [layer.value, layer.disabled])).toEqual([['slope', false], ['elevation', false], ['aspect', false]])

    tab.mapLayer.value = 'elevation'
    expect(tab.legend.value.title).toBe('Altitude')
    expect(feature('field-str-4').terrainKey).toMatch(/^elevation-\d$/)
    tab.mapLayer.value = 'aspect'
    expect(feature('field-str-4').terrainKey).toBe('northwest')
    expect(feature('field-str-2').terrainKey).toBe('no_aspect')
    expect(tab.legend.value.items.map(item => item.label)).toContain('Sem exposição')
  })

  it('collapses and reopens blocks', async () => {
    const { tab } = await openTab()
    tab.toggleBlock('Área Norte')
    expect(tab.blocks.value.find(block => block.key === 'Área Norte')!.expanded).toBe(false)
    tab.selectField('field-str-1')
    expect(tab.blocks.value.find(block => block.key === 'Área Norte')!.expanded).toBe(true)
  })

  it('has no extra badge data until loaded and counts the restricted fields afterwards', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { tab: 'terrain' }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM) })
    expect(tab.state.value).toBe('loading')
    expect(tab.restrictedFieldCount.value).toBeNull()
    expect(tab.mapFeatures.value).toEqual([])
    await flushPromises()
    expect(tab.restrictedFieldCount.value).toBeGreaterThanOrEqual(1)
  })
})

describe('useFarmTerrainTab: selection through the query', () => {
  it('selects a field with replace, ignores unknown ids and shows the popover only in the map view', async () => {
    const { fakes, tab } = await openTab()
    expect(tab.popover.value).toBeNull()

    tab.selectField('field-str-4')
    expect(fakes.route.query).toEqual({ tab: 'terrain', terrainField: 'field-str-4' })
    expect(fakes.navigations.at(-1)).toMatchObject({ options: { replace: true } })
    expect(tab.selectedFieldId.value).toBe('field-str-4')
    await flushPromises()
    expect(tab.popover.value).toMatchObject({
      name: 'Talhão 4',
      subtitle: 'Área Centro · Arara · 44,0 ha',
      relief: { value: '69 m' },
      canOpenDetail: true,
    })
    // The popover warning comes from the field detail (method fits of the API).
    expect(tab.popover.value?.methodWarning).toBe('Restrito para tratorizado de barra (62 % da área acima de 20 %) e turbopulverizador (52 % da área acima de 25 %). Inadequado para autopropelido.')

    tab.openDetail()
    expect(fakes.route.query).toEqual({ tab: 'terrain', terrainField: 'field-str-4', terrainView: 'detail' })
    expect(tab.view.value).toBe('detail')
    expect(tab.popover.value).toBeNull()

    tab.backToMap()
    expect(fakes.route.query).toEqual({ tab: 'terrain', terrainField: 'field-str-4' })
    expect(tab.view.value).toBe('map')
    expect(tab.selectedFieldId.value).toBe('field-str-4')

    fakes.route.query = { tab: 'terrain', terrainField: 'field-nope' }
    expect(tab.selectedFieldId.value).toBeNull()
  })

  it('reads detail without a selected field as the map, and clearing the selection leaves the card', async () => {
    const { fakes, tab } = await openTab()
    fakes.route.query = { tab: 'terrain', terrainView: 'detail' }
    expect(tab.view.value).toBe('map')
    tab.openDetail()
    expect(fakes.route.query).toEqual({ tab: 'terrain', terrainView: 'detail' })

    tab.selectField('field-str-4')
    tab.openDetail()
    expect(tab.view.value).toBe('detail')
    tab.selectField(null)
    expect(fakes.route.query).toEqual({ tab: 'terrain' })
    expect(tab.view.value).toBe('map')
  })

  it('builds the detail card of the selected field', async () => {
    const { tab } = await openTab()
    expect(tab.detail.value).toBeNull()
    tab.selectField('field-str-4')
    expect(tab.detailPending.value).toBe(true)
    await flushPromises()
    expect(tab.detailPending.value).toBe(false)
    expect(tab.detail.value).toMatchObject({
      title: 'Talhão 4',
      badge: { label: 'Forte ondulado' },
      indicators: { elevation: { value: '872–941 m', hint: 'desnível de 69 m' } },
    })
    expect(tab.detail.value?.methods.find(row => row.method === 'tractor_boom')).toMatchObject({ statusLabel: 'Restrito · 62 % da área', limitLabel: 'até 20 %' })
  })

  it('shows the failed field in the card with the reason', async () => {
    const { tab } = await openTab()
    tab.selectField('field-str-9')
    await flushPromises()
    expect(tab.detail.value?.failureMessage).toBe('Falha no cálculo: Bloco do modelo de elevação indisponível para este talhão.')
  })
})

describe('useFarmTerrainTab: calculation states', () => {
  it('shows the not-calculated panel (04e) with a button for who can write', async () => {
    const { tab } = await openTab(SEED_LOGINS.superAdmin, 'farm-boa-esperanca')
    expect(tab.state.value).toBe('not_calculated')
    expect(tab.notCalculated.value).toMatchObject({ title: 'Relevo ainda não calculado', calculateLabel: 'Calcular relevo dos 2 talhões', noPermissionMessage: null })
    expect(tab.mapLayers.value.every(layer => layer.disabled)).toBe(true)
    expect(tab.mapFeatures.value.every(feature => feature.dashed && feature.terrainKey === 'not_calculated')).toBe(true)
    expect(tab.indicators.value.slopeMean.value).toBe('—')
    expect(tab.missingBanner.value).toBeNull()
  })

  it('shows the list, never the 04e panel, to a reader of a calculated farm', async () => {
    const fakes = await signInAs(SEED_LOGINS.operator)
    fakes.route.query = { tab: 'terrain' }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM) })
    await flushPromises()
    expect(tab.canWrite.value).toBe(false)
    expect(tab.notCalculated.value).toBeNull()
    expect(tab.state.value).toBe('ready')
  })

  it('calculates the farm and follows the progress to the end', async () => {
    vi.useFakeTimers()
    try {
      const fakes = await signInAs(SEED_LOGINS.superAdmin)
      fakes.route.query = { tab: 'terrain' }
      const farmId = 'farm-boa-esperanca'
      const fields = useFarmFields(farmId)
      const tab = useFarmTerrainTab({ farmId, active: true, fields, pollIntervalMs: 1000 })
      await vi.advanceTimersByTimeAsync(0)
      expect(tab.state.value).toBe('not_calculated')

      await tab.calculate('missing')
      expect(tab.state.value).toBe('calculating')
      expect(tab.calculating.value).toBe(true)
      // Nothing is ready yet: still the 04e panel, now with the progress.
      expect(tab.notCalculated.value).not.toBeNull()
      expect(tab.progress.value).toEqual({ label: '0 de 2 talhões', percent: 0 })
      expect(tab.blocks.value.flatMap(block => block.fields).map(row => row.detail)).toEqual(['Calculando…', 'Calculando…'])

      await vi.advanceTimersByTimeAsync(1000)
      expect(tab.progress.value).toEqual({ label: '1 de 2 talhões', percent: 50 })
      // Something is ready: the list is back, the progress stays on top.
      expect(tab.notCalculated.value).toBeNull()

      await vi.advanceTimersByTimeAsync(1000)
      expect(tab.progress.value).toBeNull()
      expect(tab.state.value).toBe('ready')
      expect(tab.calculating.value).toBe(false)
      expect(tab.indicators.value.slopeMean.value).not.toBe('—')
      expect(vi.getTimerCount()).toBe(0)
    }
    finally {
      vi.useRealTimers()
    }
  })

  it('refreshes the open detail when the calculation ends', async () => {
    vi.useFakeTimers()
    try {
      const fakes = await signInAs(SEED_LOGINS.superAdmin)
      fakes.route.query = { tab: 'terrain', terrainField: 'field-sbe-1' }
      const farmId = 'farm-boa-esperanca'
      const tab = useFarmTerrainTab({ farmId, active: true, fields: useFarmFields(farmId), pollIntervalMs: 1000 })
      await vi.advanceTimersByTimeAsync(0)
      expect(tab.detail.value?.badge.label).toBe('Não calculado')

      await tab.calculate('all')
      await vi.advanceTimersByTimeAsync(2000)
      await vi.advanceTimersByTimeAsync(0)
      expect(tab.state.value).toBe('ready')
      expect(tab.detail.value?.badge.key).not.toBe('not_calculated')
    }
    finally {
      vi.useRealTimers()
    }
  })

  it('banners the fields without relief while others have it, with a button only for writers', async () => {
    const { tab } = await openTab()
    expect(tab.missingBanner.value).toEqual({ label: '1 talhão sem relevo', calculateLabel: 'Calcular' })
    const reader = await openTab(SEED_LOGINS.operator)
    expect(reader.tab.missingBanner.value).toEqual({ label: '1 talhão sem relevo', calculateLabel: null })
  })

  it('exposes the 403 of a calculation as an error, never as a silent failure', async () => {
    const { tab } = await openTab(SEED_LOGINS.operator)
    await tab.calculate('all')
    expect(tab.calculateError.value?.code).toBe('forbidden')
    expect(tab.calculating.value).toBe(false)
  })

  it('does not load anything while the tab is not active', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    const tab = useFarmTerrainTab({ farmId: FARM, active: false, fields: useFarmFields(FARM) })
    await flushPromises()
    expect(fakes.route.query).toEqual({})
    expect(tab.state.value).toBe('loading')
    expect(tab.restrictedFieldCount.value).toBeNull()
  })
})

describe('useFarmTerrainTab: adjusting and removing', () => {
  it('adjusts through the form and the list and the card update', async () => {
    const { tab } = await openTab()
    tab.selectField('field-str-1')
    tab.openDetail()
    await flushPromises()
    expect(tab.canWrite.value).toBe(true)
    const form = tab.adjustmentForm
    form.reset()
    expect(form.title.value).toBe('Ajustar relevo · Talhão 1')
    expect(form.values.slopeMeanPct).not.toBe('')

    Object.assign(form.values, { origin: 'topographic_survey', slopeMeanPct: '30,5', slopeMaxPct: '44', position: 'slope', justification: 'RTK 2026' })
    expect(await form.submit()).toBe(true)
    await flushPromises()

    expect(tab.detail.value).toMatchObject({ hasAdjustment: true, badge: { label: 'Forte ondulado' }, adjustmentNote: 'Justificativa: RTK 2026' })
    const row = tab.blocks.value.flatMap(block => block.fields).find(item => item.id === 'field-str-1')!
    expect(row).toMatchObject({ isAdjusted: true, detail: '30,5 % · máx. 44,0 % · encosta' })
  })

  it('removes the adjustment and the list goes back to the calculated value', async () => {
    const { tab } = await openTab()
    tab.selectField('field-str-2')
    await flushPromises()
    expect(await tab.removeAdjustment()).toBe(true)
    await flushPromises()
    expect(tab.detail.value?.hasAdjustment).toBe(false)
    expect(tab.blocks.value.flatMap(block => block.fields).find(row => row.id === 'field-str-2')?.isAdjusted).toBe(false)
    // A second removal has nothing to remove (404).
    expect(await tab.removeAdjustment()).toBe(false)
    expect(tab.removeError.value?.code).toBe('not_found')
  })
})

describe('useTerrainAdjustmentForm', () => {
  async function formFor(login: string = SEED_LOGINS.admin, fieldId = 'field-str-4') {
    const fakes = await signInAs(login)
    const field = useFieldTerrain(FARM, fieldId)
    await flushPromises()
    const form = useTerrainAdjustmentForm({ fieldName: 'Talhão 4', field })
    form.reset()
    return { fakes, field, form }
  }

  it('opens filled with the calculated values and the current class, without computing a class', async () => {
    const { form } = await formFor()
    expect(form.values).toEqual({ origin: 'field_assessment', slopeMeanPct: '24,3', slopeMaxPct: '41,8', position: 'slope', justification: '' })
    expect(form.hints.value).toEqual({
      slopeMean: 'Calculado: 24,3 %',
      slopeMax: 'Calculado: 41,8 %',
      position: 'Calculado: Encosta',
      currentClass: 'Classe atual: Forte ondulado',
      classNotice: 'A classe é recalculada ao salvar.',
    })
    expect(form.title.value).toBe('Ajustar relevo · Talhão 4')
    expect(form.justificationMaxLength).toBe(1000)
    expect(form.originOptions.map(option => option.value)).toEqual(['field_assessment', 'topographic_survey'])
    expect(form.positionOptions.map(option => option.value)).toEqual(['hilltop', 'slope', 'lowland'])
  })

  it('shows local errors only after trying to save and does not call the API', async () => {
    const { form, field } = await formFor()
    form.values.slopeMaxPct = '10'
    form.values.justification = '   '
    expect(form.errors.value).toEqual({})
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({
      slopeMaxPct: 'A máxima não pode ser menor que a média.',
      justification: 'Informe a justificativa.',
    })
    expect(field.adjusting.value).toBe(false)
    expect(field.terrain.value?.adjustment).toBeNull()
  })

  it('saves valid values and trims the justification', async () => {
    const { form, field } = await formFor()
    form.values.justification = '  Terraços  '
    expect(await form.submit()).toBe(true)
    expect(field.terrain.value?.adjustment?.justification).toBe('Terraços')
    expect(form.errors.value).toEqual({})
    expect(form.formError.value).toBeNull()
  })

  it('maps a 422 of the API to the field, and a 403 to the form error', async () => {
    const { form, field } = await formFor()
    // The mock validates the same contract: bypass the local check to reach the API.
    const submitted = await field.adjust({ origin: 'field_assessment', slopeMeanPct: 20, slopeMaxPct: 10, position: 'slope', justification: 'x' })
    expect(submitted).toBeNull()
    expect(form.errors.value).toEqual({ slopeMaxPct: 'a declividade máxima não pode ser menor que a média' })
    expect(form.formError.value).toBeNull()

    // Editing a value drops the API message: it described what was sent.
    form.values.slopeMaxPct = '30'
    expect(form.errors.value).toEqual({})

    const reader = await formFor(SEED_LOGINS.operator)
    expect(await reader.form.submit()).toBe(false)
    expect(reader.form.values.justification).toBe('')
    reader.form.values.justification = 'Tentativa'
    expect(await reader.form.submit()).toBe(false)
    expect(reader.form.formError.value).toMatch(/perfil não permite/)
    expect(reader.form.errors.value).toEqual({})
  })

  it('starts from the current adjustment when there is one', async () => {
    const { form } = await formFor(SEED_LOGINS.admin, 'field-str-2')
    expect(form.values).toMatchObject({ origin: 'topographic_survey', slopeMeanPct: '14,2', slopeMaxPct: '21,5', position: 'slope' })
    expect(form.values.justification).toMatch(/Levantamento/)
    expect(form.hints.value.slopeMean).toMatch(/^Calculado: /)
  })

  it('works without a selected field (fake of the resource)', () => {
    const scope = effectScope()
    scope.run(() => {
      const form = useTerrainAdjustmentForm({
        fieldName: () => '',
        field: {
          terrain: { value: null } as never,
          adjust: async () => null,
          adjusting: { value: false } as never,
          adjustError: { value: null } as never,
        },
      })
      expect(form.hints.value.currentClass).toBe('Classe atual: Não calculado')
      expect(form.values.position).toBeNull()
    })
    scope.stop()
  })
})

describe('farm screen: tab, badge and header buttons', () => {
  async function page(login: string, query: Record<string, string> = { tab: 'terrain' }) {
    const fakes = await signInAs(login)
    fakes.route.path = `/farms/${FARM}`
    fakes.route.query = query
    const detail = useFarmDetailPage(FARM)
    await flushPromises()
    return { fakes, detail }
  }

  it('enables the Relevo tab between the areas and the owners, with a badge of restricted fields', async () => {
    const { detail } = await page(SEED_LOGINS.admin)
    expect(detail.tab.value).toBe('terrain')
    const tabs = detail.tabs.value
    expect(tabs.map(item => item.value)).toEqual(['overview', 'fields', 'terrain', 'owners', 'applications', 'windows'])
    expect(tabs.find(item => item.value === 'terrain')).toMatchObject({ label: 'Terreno', disabled: false })
    expect(tabs.find(item => item.value === 'terrain')!.count).toBeGreaterThanOrEqual(1)
  })

  it('has no badge before the tab is opened (nothing is loaded) and opens through the query', async () => {
    const { fakes, detail } = await page(SEED_LOGINS.admin, {})
    expect(detail.tab.value).toBe('fields')
    expect(detail.tabs.value.find(item => item.value === 'terrain')!.count).toBeNull()

    detail.tab.value = 'terrain'
    await flushPromises()
    expect(fakes.route.query).toEqual({ tab: 'terrain' })
    expect(detail.terrain.state.value).toBe('ready')
    expect(detail.tabs.value.find(item => item.value === 'terrain')!.count).toBeGreaterThanOrEqual(1)
  })

  it('offers "Como calculamos" and "Recalcular relevo" to a writer once something is calculated', async () => {
    const { detail } = await page(SEED_LOGINS.admin)
    expect(detail.headerActions.value.map(action => [action.id, action.label, action.variant, action.loading])).toEqual([
      ['terrainMethodology', 'Como calculamos', 'secondary', false],
      ['recalculateTerrain', 'Recalcular relevo', 'primary', false],
    ])
  })

  it('offers only "Como calculamos" to a reader', async () => {
    const { detail } = await page(SEED_LOGINS.operator)
    expect(detail.headerActions.value.map(action => action.id)).toEqual(['terrainMethodology'])
  })

  it('runs the recalculation with scope all and keeps the button loading while it runs', async () => {
    const { detail } = await page(SEED_LOGINS.admin)
    detail.runHeaderAction('recalculateTerrain')
    await flushPromises()
    expect(detail.terrain.calculating.value).toBe(true)
    expect(detail.headerActions.value.find(action => action.id === 'recalculateTerrain')!.loading).toBe(true)
    expect(detail.terrain.progress.value?.label).toBe('0 de 9 talhões')
  })

  it('ignores header actions that belong to the page', async () => {
    const { detail } = await page(SEED_LOGINS.admin)
    detail.runHeaderAction('terrainMethodology')
    detail.runHeaderAction('newField')
    detail.runHeaderAction('newArea')
    await flushPromises()
    expect(detail.terrain.calculating.value).toBe(false)
  })

  it('keeps "Nova área" and "Novo talhão" on the other tabs, for writers only', async () => {
    const writer = await page(SEED_LOGINS.admin, {})
    expect(writer.detail.headerActions.value.map(action => [action.id, action.to, action.unavailableHint])).toEqual([
      ['newArea', null, 'Em breve'],
      ['newField', `/farms/${FARM}/fields/new`, null],
    ])
    const reader = await page(SEED_LOGINS.operator, {})
    expect(reader.detail.headerActions.value).toEqual([])
  })

  it('exposes the fixed methodology text with the Copernicus attribution', async () => {
    const { detail } = await page(SEED_LOGINS.admin)
    expect(detail.terrain.methodology.title).toBe('Como calculamos o relevo')
    expect(detail.terrain.methodology.paragraphs.length).toBeGreaterThan(1)
    expect(detail.terrain.methodology.attribution).toBe('Contém dados modificados do Copernicus DEM GLO-30 © DLR e Airbus')
  })
})

describe('useFarmTerrainTab: "Chuva e solo" view of the detail (ADR 0022)', () => {
  /** The app's mock service, counting the calls. */
  function countingService() {
    const calls: string[] = []
    const service: TrafficabilityService = {
      getFieldTrafficability: (farmId, fieldId, days) => {
        calls.push(fieldId)
        return useApiServices().trafficability.getFieldTrafficability(farmId, fieldId, days)
      },
    }
    return { service, calls }
  }

  async function openRainTab(active: Ref<boolean> | boolean = true) {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { tab: 'terrain' }
    const counting = countingService()
    const tab = useFarmTerrainTab({ farmId: FARM, active, fields: useFarmFields(FARM), trafficabilityService: counting.service })
    await flushPromises()
    return { fakes, tab, calls: counting.calls }
  }

  const RAIN_QUERY = { tab: 'terrain', terrainField: 'field-str-4', terrainView: 'detail', terrainDetailView: 'rain' }

  it('keeps the detail view in `?terrainDetailView=rain` and reads "relief" outside the detail', async () => {
    const { fakes, tab } = await openRainTab()
    tab.selectField('field-str-4')
    expect(tab.detailView.value).toBe('relief')
    tab.openDetail()
    tab.detailView.value = 'rain'
    expect(fakes.route.query).toEqual(RAIN_QUERY)
    expect(fakes.navigations.at(-1)).toMatchObject({ options: { replace: true } })
    expect(tab.detailView.value).toBe('rain')

    tab.detailView.value = 'relief'
    expect(fakes.route.query).toEqual({ tab: 'terrain', terrainField: 'field-str-4', terrainView: 'detail' })

    // A stray parameter without the detail open reads as "Relevo".
    fakes.route.query = { tab: 'terrain', terrainField: 'field-str-4', terrainDetailView: 'rain' }
    expect(tab.detailView.value).toBe('relief')
  })

  it('clears the detail view on backToMap and on selectField(null)', async () => {
    const { fakes, tab } = await openRainTab()
    fakes.route.query = { ...RAIN_QUERY }
    tab.backToMap()
    expect(fakes.route.query).toEqual({ tab: 'terrain', terrainField: 'field-str-4' })

    fakes.route.query = { ...RAIN_QUERY }
    tab.selectField(null)
    expect(fakes.route.query).toEqual({ tab: 'terrain' })
  })

  it('loads only with the detail open on "rain", for the selected field', async () => {
    const { tab, calls } = await openRainTab()
    tab.selectField('field-str-4')
    await flushPromises()
    tab.openDetail()
    await flushPromises()
    expect(calls).toEqual([])
    expect(tab.trafficability.value).toBeNull()

    tab.detailView.value = 'rain'
    await flushPromises()
    expect(calls).toEqual(['field-str-4'])
    expect(tab.trafficabilityPending.value).toBe(false)
    expect(tab.trafficabilityError.value).toBeNull()
    expect(tab.trafficability.value).toMatchObject({ fieldId: 'field-str-4', soil: { state: 'waterlogged' } })
  })

  it('does not ask again when going back and forth between "Relevo" and "Chuva e solo" on the same field', async () => {
    const { fakes, tab, calls } = await openRainTab()
    fakes.route.query = { ...RAIN_QUERY }
    await flushPromises()
    tab.detailView.value = 'relief'
    await flushPromises()
    expect(tab.trafficability.value).toBeNull()
    tab.detailView.value = 'rain'
    await flushPromises()
    expect(calls).toEqual(['field-str-4'])
    expect(tab.trafficability.value?.fieldId).toBe('field-str-4')

    // Another field is another answer.
    tab.selectField('field-str-1')
    await flushPromises()
    expect(calls).toEqual(['field-str-4', 'field-str-1'])
    expect(tab.trafficability.value).toMatchObject({ fieldId: 'field-str-1', soil: { state: 'unknown' } })
  })

  it('calls nothing while the tab is inactive', async () => {
    const active = ref(false)
    const { fakes, tab, calls } = await openRainTab(active)
    fakes.route.query = { ...RAIN_QUERY }
    await flushPromises()
    expect(calls).toEqual([])
    expect(tab.trafficability.value).toBeNull()
    active.value = true
    await flushPromises()
    expect(calls).toEqual(['field-str-4'])
  })

  it('exposes the requested horizon, not the length of the answer', async () => {
    const { tab } = await openRainTab()
    expect(tab.trafficabilityDays.value).toBe(5)
    const custom = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM), trafficabilityDays: 3 })
    expect(custom.trafficabilityDays.value).toBe(3)
  })
})

describe('useFarmTerrainTab: recording field conditions from "Chuva e solo" (ADR 0022, phase 2)', () => {
  const RAIN_URL = { tab: 'terrain', terrainField: 'field-str-4', terrainView: 'detail', terrainDetailView: 'rain' }

  async function openOnRain(login: string) {
    const fakes = await signInAs(login)
    fakes.route.query = { ...RAIN_URL }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM), now: () => SEED_NOW })
    await flushPromises()
    return { fakes, tab }
  }

  it('reports pending while the field list still loads for a "rain" URL (no blank panel)', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { ...RAIN_URL }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM) })
    expect(tab.selectedFieldId.value).toBeNull()
    expect(tab.trafficabilityPending.value).toBe(true)
    await flushPromises()
    expect(tab.trafficabilityPending.value).toBe(false)
    expect(tab.trafficability.value?.fieldId).toBe('field-str-4')
  })

  it('is not pending for a "relief" URL while the fields load', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { ...RAIN_URL, terrainDetailView: undefined }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM) })
    expect(tab.trafficabilityPending.value).toBe(false)
  })

  it('exposes the capabilities: operators record, only registry.write edits the profile', async () => {
    const operator = (await openOnRain(SEED_LOGINS.operator)).tab
    expect(operator.canRecordConditions.value).toBe(true)
    expect(operator.canEditSoilProfile.value).toBe(false)
    vi.unstubAllGlobals()
    const admin = (await openOnRain(SEED_LOGINS.admin)).tab
    expect(admin.canRecordConditions.value).toBe(true)
    expect(admin.canEditSoilProfile.value).toBe(true)
  })

  it('loads the gauges and the profile of the field, and preselects its gauge in the reading form', async () => {
    const { tab } = await openOnRain(SEED_LOGINS.operator)
    expect(tab.rainGauges.gauges.value).toHaveLength(2)
    expect(tab.soilProfileForm.profile.value).toMatchObject({ soilTexture: 'clay', rainGauge: { id: MOCK_SEDE_GAUGE_ID } })
    tab.rainReadingForm.reset()
    expect(tab.rainReadingForm.values.gaugeId).toBe(MOCK_SEDE_GAUGE_ID)
  })

  it('reloads the trafficability after an inspection: today follows what the person saw', async () => {
    const { tab } = await openOnRain(SEED_LOGINS.operator)
    expect(tab.trafficability.value?.soil.state).toBe('waterlogged')
    tab.soilInspectionForm.reset()
    tab.soilInspectionForm.values.state = 'moist'
    expect(await tab.soilInspectionForm.submit()).toBe(true)
    expect(tab.trafficability.value?.soil).toMatchObject({ state: 'moist', basis: 'inspection' })
  })

  it('reloads the trafficability after a reading', async () => {
    const { tab } = await openOnRain(SEED_LOGINS.operator)
    const before = tab.trafficability.value!.soil.rain!.last24hMm
    tab.rainReadingForm.reset()
    tab.rainReadingForm.values.rainMm = '3'
    expect(await tab.rainReadingForm.submit()).toBe(true)
    expect(tab.trafficability.value?.soil.rain?.last24hMm).toBe(before + 3)
  })

  it('does not load gauges nor profile before "Chuva e solo" is opened', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { tab: 'terrain', terrainField: 'field-str-4', terrainView: 'detail' }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM) })
    await flushPromises()
    expect(tab.rainGauges.gauges.value).toEqual([])
    expect(tab.soilProfileForm.profile.value).toBeNull()
  })
})
