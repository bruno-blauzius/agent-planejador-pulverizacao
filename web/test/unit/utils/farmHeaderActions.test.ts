import { describe, expect, it } from 'vitest'
import { farmDetailTabFrom, farmDetailTabs, farmHeaderActions, isFarmDetailTabEnabled } from '~/utils/farmDetail'
import { TERRAIN_SCOPE_BY_ACTION } from '~/utils/terrain'

const idle = { calculatedFieldCount: 0, running: false, requesting: false }

describe('farmDetailTabs with the Relevo tab', () => {
  it('places Relevo after "Áreas e talhões", enabled, badge only for restricted fields', () => {
    const tabs = farmDetailTabs({ fields: 9, owners: 2, terrainRestricted: 3 })
    expect(tabs.map(tab => tab.value)).toEqual(['overview', 'fields', 'terrain', 'owners', 'applications', 'windows'])
    expect(tabs[2]).toEqual({ value: 'terrain', label: 'Terreno', count: 3, disabled: false })
  })

  it('shows no badge with zero, with null or when the count is not given', () => {
    expect(farmDetailTabs({ fields: 1, owners: 1, terrainRestricted: 0 })[2]!.count).toBeNull()
    expect(farmDetailTabs({ fields: 1, owners: 1, terrainRestricted: null })[2]!.count).toBeNull()
    expect(farmDetailTabs({ fields: 1, owners: 1 })[2]!.count).toBeNull()
  })

  it('reads ?tab=terrain and keeps the other disabled tabs closed', () => {
    expect(isFarmDetailTabEnabled('terrain')).toBe(true)
    expect(farmDetailTabFrom('terrain')).toBe('terrain')
    expect(farmDetailTabFrom('relevo')).toBe('fields')
    expect(farmDetailTabFrom('windows')).toBe('fields')
  })
})

describe('farmHeaderActions', () => {
  it('on Relevo, offers "Como calculamos" and "Calcular relevo" when nothing is calculated', () => {
    const actions = farmHeaderActions('terrain', { farmId: 'f1', canWrite: true, terrain: idle })
    expect(actions.map(action => [action.id, action.label, action.variant])).toEqual([
      ['terrainMethodology', 'Como calculamos', 'secondary'],
      ['calculateTerrain', 'Calcular relevo', 'primary'],
    ])
  })

  it('switches to "Recalcular relevo" once any field has relief', () => {
    const actions = farmHeaderActions('terrain', { farmId: 'f1', canWrite: true, terrain: { ...idle, calculatedFieldCount: 4 } })
    expect(actions.map(action => action.id)).toEqual(['terrainMethodology', 'recalculateTerrain'])
    expect(actions[1]!.label).toBe('Recalcular relevo')
  })

  it('treats a relief not loaded yet as never calculated', () => {
    expect(farmHeaderActions('terrain', { farmId: 'f1', canWrite: true, terrain: null }).map(action => action.id))
      .toEqual(['terrainMethodology', 'calculateTerrain'])
  })

  it('marks the calculate button as loading while the request is in flight or the calculation runs', () => {
    const loading = (terrain: { calculatedFieldCount: number, running: boolean, requesting: boolean } | null) =>
      farmHeaderActions('terrain', { farmId: 'f1', canWrite: true, terrain }).at(-1)!.loading
    expect(loading(idle)).toBe(false)
    expect(loading({ ...idle, requesting: true })).toBe(true)
    expect(loading({ ...idle, running: true, calculatedFieldCount: 2 })).toBe(true)
    expect(loading(null)).toBe(false)
  })

  it('gives a reader only "Como calculamos"', () => {
    const actions = farmHeaderActions('terrain', { farmId: 'f1', canWrite: false, terrain: { ...idle, calculatedFieldCount: 4 } })
    expect(actions.map(action => action.id)).toEqual(['terrainMethodology'])
    expect(actions[0]!.loading).toBe(false)
  })

  it('on the other tabs keeps "Nova área" (not available) and "Novo talhão" for writers, nothing for readers', () => {
    for (const tab of ['fields', 'overview', 'owners'] as const) {
      const actions = farmHeaderActions(tab, { farmId: 'farm 1', canWrite: true, terrain: null })
      expect(actions).toEqual([
        { id: 'newArea', label: 'Nova área', icon: 'plus', variant: 'secondary', to: null, unavailableHint: 'Em breve', loading: false },
        { id: 'newField', label: 'Novo talhão', icon: 'plus', variant: 'primary', to: '/farms/farm%201/fields/new', unavailableHint: null, loading: false },
      ])
      expect(farmHeaderActions(tab, { farmId: 'f1', canWrite: false, terrain: null })).toEqual([])
    }
  })

  it('maps each calculate button to its scope', () => {
    expect(TERRAIN_SCOPE_BY_ACTION).toEqual({ calculateTerrain: 'missing', recalculateTerrain: 'all' })
  })
})
