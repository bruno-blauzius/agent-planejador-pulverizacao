import { effectScope, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFarmTerrain } from '~/composables/useFarmTerrain'
import { useFieldTerrain } from '~/composables/useFieldTerrain'
import { ApiError } from '~/services/errors'
import type { TerrainService } from '~/services/ports'
import type { FarmTerrain, FieldTerrain, TerrainProgress } from '~/types/terrain'
import { createTestApi } from '../../support/mockApi'
import { installNuxtFakes } from '../../support/nuxtFakes'

const settle = () => vi.advanceTimersByTimeAsync(0)

function farm(progress: TerrainProgress | null, calculated = 1): FarmTerrain {
  return {
    farmId: 'farm-1',
    summary: {
      fieldCount: 3,
      calculatedFieldCount: calculated,
      slopeMeanPct: null,
      restrictedFieldCount: 0,
      elevationMinM: null,
      elevationMaxM: null,
      calculatedAt: null,
    },
    progress,
    items: [],
  }
}

function field(overrides: Partial<FieldTerrain> = {}): FieldTerrain {
  return {
    fieldId: 'field-1',
    farmId: 'farm-1',
    status: 'ready',
    failureReason: null,
    source: null,
    effective: null,
    calculated: null,
    adjustment: null,
    adjustmentOutdated: false,
    methodFits: [],
    ...overrides,
  }
}

/** A service that answers from a script and counts the calls. */
function scripted(answers: FarmTerrain[]) {
  const calls = { getFarm: 0, calculate: [] as string[], getField: [] as string[], adjust: 0, remove: 0 }
  let index = 0
  const service: TerrainService = {
    async getFarm() {
      calls.getFarm += 1
      return answers[Math.min(index++, answers.length - 1)]!
    },
    async calculate(_farmId, scope) {
      calls.calculate.push(scope)
      return farm({ total: 3, done: 0 }, 0)
    },
    async getField(_farmId, fieldId) {
      calls.getField.push(fieldId)
      return field({ fieldId })
    },
    async adjust(_farmId, fieldId) {
      calls.adjust += 1
      return field({ fieldId, source: 'manual' })
    },
    async removeAdjustment(_farmId, fieldId) {
      calls.remove += 1
      return field({ fieldId })
    },
  }
  return { service, calls }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('useFarmTerrain', () => {
  it('loads the relief and reports the first load as pending', async () => {
    installNuxtFakes(createTestApi().api)
    const { service } = scripted([farm(null)])
    const resource = useFarmTerrain('farm-1', { service })
    expect(resource.pending.value).toBe(true)
    expect(resource.terrain.value).toBeNull()
    await settle()
    expect(resource.pending.value).toBe(false)
    expect(resource.terrain.value?.farmId).toBe('farm-1')
    expect(resource.isRunning.value).toBe(false)
    expect(resource.error.value).toBeNull()
  })

  it('does not call the API while the tab is inactive and loads when it becomes active', async () => {
    installNuxtFakes(createTestApi().api)
    const { service, calls } = scripted([farm(null)])
    const active = ref(false)
    const resource = useFarmTerrain('farm-1', { active, service })
    await settle()
    expect(calls.getFarm).toBe(0)
    expect(resource.terrain.value).toBeNull()

    active.value = true
    await settle()
    expect(calls.getFarm).toBe(1)
    expect(resource.terrain.value?.farmId).toBe('farm-1')
  })

  it('exposes the API error and keeps no data', async () => {
    installNuxtFakes(createTestApi().api)
    const service: TerrainService = {
      ...scripted([]).service,
      getFarm: async () => { throw new ApiError({ code: 'not_found', message: 'fazenda não encontrada', status: 404 }) },
    }
    const resource = useFarmTerrain('farm-1', { service })
    await settle()
    expect(resource.error.value?.code).toBe('not_found')
    expect(resource.terrain.value).toBeNull()
    expect(resource.pending.value).toBe(false)
  })

  describe('polling while a calculation runs', () => {
    it('asks again every interval until progress is gone, then stops', async () => {
      installNuxtFakes(createTestApi().api)
      const { service, calls } = scripted([
        farm({ total: 3, done: 0 }),
        farm({ total: 3, done: 1 }),
        farm({ total: 3, done: 2 }),
        farm(null, 3),
      ])
      const resource = useFarmTerrain('farm-1', { service, pollIntervalMs: 3000 })
      await settle()
      expect(calls.getFarm).toBe(1)
      expect(resource.isRunning.value).toBe(true)

      await vi.advanceTimersByTimeAsync(2999)
      expect(calls.getFarm).toBe(1)
      await vi.advanceTimersByTimeAsync(1)
      expect(calls.getFarm).toBe(2)
      expect(resource.terrain.value?.progress).toEqual({ total: 3, done: 1 })

      await vi.advanceTimersByTimeAsync(3000)
      expect(calls.getFarm).toBe(3)
      await vi.advanceTimersByTimeAsync(3000)
      expect(calls.getFarm).toBe(4)
      expect(resource.isRunning.value).toBe(false)
      expect(resource.terrain.value?.summary.calculatedFieldCount).toBe(3)

      await vi.advanceTimersByTimeAsync(30_000)
      expect(calls.getFarm).toBe(4)
      expect(vi.getTimerCount()).toBe(0)
    })

    it('uses 3 seconds by default', async () => {
      installNuxtFakes(createTestApi().api)
      const { service, calls } = scripted([farm({ total: 2, done: 0 }), farm(null)])
      useFarmTerrain('farm-1', { service })
      await settle()
      await vi.advanceTimersByTimeAsync(2999)
      expect(calls.getFarm).toBe(1)
      await vi.advanceTimersByTimeAsync(1)
      expect(calls.getFarm).toBe(2)
    })

    it('never polls when nothing is running', async () => {
      installNuxtFakes(createTestApi().api)
      const { service, calls } = scripted([farm(null)])
      useFarmTerrain('farm-1', { service })
      await settle()
      await vi.advanceTimersByTimeAsync(60_000)
      expect(calls.getFarm).toBe(1)
    })

    it('does not poll before the component is mounted (server render, no browser)', async () => {
      const fakes = installNuxtFakes(createTestApi().api)
      fakes.autoMount = false
      const { service, calls } = scripted([farm({ total: 2, done: 0 }), farm(null)])
      useFarmTerrain('farm-1', { service })
      await settle()
      await vi.advanceTimersByTimeAsync(10_000)
      expect(calls.getFarm).toBe(1)

      fakes.mount()
      await vi.advanceTimersByTimeAsync(3000)
      expect(calls.getFarm).toBe(2)
    })

    it('stops when the tab is left and resumes when it comes back', async () => {
      installNuxtFakes(createTestApi().api)
      const { service, calls } = scripted([farm({ total: 3, done: 0 }), farm({ total: 3, done: 1 }), farm({ total: 3, done: 2 })])
      const active = ref(true)
      useFarmTerrain('farm-1', { active, service })
      await settle()
      expect(calls.getFarm).toBe(1)

      active.value = false
      await settle()
      await vi.advanceTimersByTimeAsync(20_000)
      expect(calls.getFarm).toBe(1)
      expect(vi.getTimerCount()).toBe(0)

      active.value = true
      await settle()
      // Becoming active reloads once, then the polling continues.
      expect(calls.getFarm).toBe(2)
      await vi.advanceTimersByTimeAsync(3000)
      expect(calls.getFarm).toBe(3)
    })

    it('stops when the scope is disposed (component unmounted)', async () => {
      installNuxtFakes(createTestApi().api)
      const { service, calls } = scripted([farm({ total: 3, done: 0 })])
      const scope = effectScope()
      scope.run(() => useFarmTerrain('farm-1', { service }))
      await settle()
      await vi.advanceTimersByTimeAsync(3000)
      expect(calls.getFarm).toBe(2)

      scope.stop()
      await vi.advanceTimersByTimeAsync(30_000)
      expect(calls.getFarm).toBe(2)
      expect(vi.getTimerCount()).toBe(0)
    })

    it('keeps polling after a failed request, as the calculation may still be running', async () => {
      installNuxtFakes(createTestApi().api)
      let attempt = 0
      const service: TerrainService = {
        ...scripted([]).service,
        getFarm: async () => {
          attempt += 1
          if (attempt === 2) throw new ApiError({ code: 'network_error', message: 'offline', status: 0 })
          return attempt === 1 ? farm({ total: 3, done: 0 }) : farm(null, 3)
        },
      }
      const resource = useFarmTerrain('farm-1', { service })
      await settle()
      await vi.advanceTimersByTimeAsync(3000)
      expect(attempt).toBe(2)
      expect(resource.error.value?.code).toBe('network_error')
      expect(resource.terrain.value?.progress).toEqual({ total: 3, done: 0 })
      await vi.advanceTimersByTimeAsync(3000)
      expect(attempt).toBe(3)
      expect(resource.isRunning.value).toBe(false)
    })
  })

  describe('calculate', () => {
    it('shows the 202 answer right away and starts polling', async () => {
      installNuxtFakes(createTestApi().api)
      const { service, calls } = scripted([farm(null, 0), farm({ total: 3, done: 2 }, 0), farm(null, 3)])
      const resource = useFarmTerrain('farm-1', { service })
      await settle()
      expect(resource.isRunning.value).toBe(false)

      const answer = await resource.calculate('missing')
      expect(calls.calculate).toEqual(['missing'])
      expect(answer?.progress).toEqual({ total: 3, done: 0 })
      expect(resource.terrain.value?.progress).toEqual({ total: 3, done: 0 })
      expect(resource.isRunning.value).toBe(true)
      expect(resource.calculating.value).toBe(false)

      await settle()
      await vi.advanceTimersByTimeAsync(3000)
      expect(resource.terrain.value?.progress).toEqual({ total: 3, done: 2 })
      await vi.advanceTimersByTimeAsync(3000)
      expect(resource.isRunning.value).toBe(false)
      expect(resource.terrain.value?.summary.calculatedFieldCount).toBe(3)
    })

    it('resolves null and exposes the error when the request fails (403)', async () => {
      installNuxtFakes(createTestApi().api)
      const service: TerrainService = {
        ...scripted([farm(null)]).service,
        calculate: async () => { throw new ApiError({ code: 'forbidden', message: 'sem permissão', status: 403 }) },
      }
      const resource = useFarmTerrain('farm-1', { service })
      await settle()
      expect(await resource.calculate('all')).toBeNull()
      expect(resource.calculateError.value?.code).toBe('forbidden')
      expect(resource.isRunning.value).toBe(false)
    })
  })
})

describe('useFieldTerrain', () => {
  it('calls nothing without a field id and loads when one is picked', async () => {
    installNuxtFakes(createTestApi().api)
    const { service, calls } = scripted([farm(null)])
    const fieldId = ref<string | null>(null)
    const resource = useFieldTerrain('farm-1', fieldId, { service })
    await settle()
    expect(calls.getField).toEqual([])
    expect(resource.terrain.value).toBeNull()
    expect(resource.pending.value).toBe(false)

    fieldId.value = 'field-7'
    await settle()
    expect(calls.getField).toEqual(['field-7'])
    expect(resource.terrain.value?.fieldId).toBe('field-7')
  })

  it('adjusts, shows the answer and tells the farm relief to reload', async () => {
    installNuxtFakes(createTestApi().api)
    const { service, calls } = scripted([farm(null)])
    let changed = 0
    const resource = useFieldTerrain('farm-1', 'field-1', { service, onChanged: async () => { changed += 1 } })
    await settle()
    expect(resource.terrain.value?.source).toBeNull()

    const result = await resource.adjust({ origin: 'field_assessment', slopeMeanPct: 10, slopeMaxPct: 12, position: 'slope', justification: 'x' })
    expect(result?.source).toBe('manual')
    expect(resource.terrain.value?.source).toBe('manual')
    expect(calls.adjust).toBe(1)
    expect(changed).toBe(1)
    expect(resource.adjusting.value).toBe(false)
  })

  it('keeps the 422 details and does not reload the farm when the adjustment fails', async () => {
    installNuxtFakes(createTestApi().api)
    let changed = 0
    const service: TerrainService = {
      ...scripted([farm(null)]).service,
      adjust: async () => {
        throw new ApiError({ code: 'validation_failed', message: 'dados inválidos', status: 422, details: [{ field: 'slope_max_pct', message: 'menor' }] })
      },
    }
    const resource = useFieldTerrain('farm-1', 'field-1', { service, onChanged: async () => { changed += 1 } })
    await settle()
    expect(await resource.adjust({ origin: 'field_assessment', slopeMeanPct: 10, slopeMaxPct: 5, position: 'slope', justification: 'x' })).toBeNull()
    expect(resource.adjustError.value?.details).toEqual([{ field: 'slope_max_pct', message: 'menor' }])
    expect(changed).toBe(0)
  })

  it('removes the adjustment and reloads the farm', async () => {
    installNuxtFakes(createTestApi().api)
    const { service, calls } = scripted([farm(null)])
    let changed = 0
    const resource = useFieldTerrain('farm-1', 'field-1', { service, onChanged: async () => { changed += 1 } })
    await settle()
    expect((await resource.removeAdjustment())?.source).toBeNull()
    expect(calls.remove).toBe(1)
    expect(changed).toBe(1)
    expect(resource.removeError.value).toBeNull()
  })

  it('reports a missing adjustment (404) through removeError', async () => {
    installNuxtFakes(createTestApi().api)
    const service: TerrainService = {
      ...scripted([farm(null)]).service,
      removeAdjustment: async () => { throw new ApiError({ code: 'not_found', message: 'sem ajuste', status: 404 }) },
    }
    const resource = useFieldTerrain('farm-1', 'field-1', { service })
    await settle()
    expect(await resource.removeAdjustment()).toBeNull()
    expect(resource.removeError.value?.code).toBe('not_found')
  })
})

describe('races of actions and loads', () => {
  it('keeps the progress of calculate when a polling GET that started before it ends later', async () => {
    installNuxtFakes(createTestApi().api)
    let releaseLate: (value: FarmTerrain) => void = () => {}
    let call = 0
    const service: TerrainService = {
      ...scripted([]).service,
      getFarm: () => {
        call += 1
        if (call === 1) return Promise.resolve(farm(null, 0))
        return new Promise<FarmTerrain>((resolve) => { releaseLate = resolve })
      },
      calculate: async () => farm({ total: 3, done: 0 }, 0),
    }
    const resource = useFarmTerrain('farm-1', { service })
    await settle()
    const lateGet = resource.refresh()
    await resource.calculate('all')
    expect(resource.isRunning.value).toBe(true)

    releaseLate(farm(null, 0))
    await lateGet
    await settle()
    expect(resource.terrain.value?.progress).toEqual({ total: 3, done: 0 })
    expect(resource.isRunning.value).toBe(true)
  })

  it('ignores the answer of calculate when the farm changed meanwhile', async () => {
    installNuxtFakes(createTestApi().api)
    let release: (value: FarmTerrain) => void = () => {}
    const service: TerrainService = {
      ...scripted([farm(null, 0)]).service,
      calculate: () => new Promise<FarmTerrain>((resolve) => { release = resolve }),
    }
    const farmId = ref('farm-1')
    const resource = useFarmTerrain(farmId, { service })
    await settle()
    const pending = resource.calculate('all')
    farmId.value = 'farm-2'
    await settle()
    release(farm({ total: 3, done: 0 }, 0))
    await pending
    expect(resource.isRunning.value).toBe(false)
    expect(resource.terrain.value?.progress).toBeNull()
  })

  it('clears the calculation error on request and when the farm changes', async () => {
    installNuxtFakes(createTestApi().api)
    const service: TerrainService = {
      ...scripted([farm(null)]).service,
      calculate: async () => { throw new ApiError({ code: 'forbidden', message: 'sem permissão', status: 403 }) },
    }
    const farmId = ref('farm-1')
    const resource = useFarmTerrain(farmId, { service })
    await settle()
    await resource.calculate('all')
    expect(resource.calculateError.value).not.toBeNull()
    resource.clearCalculateError()
    expect(resource.calculateError.value).toBeNull()

    await resource.calculate('all')
    farmId.value = 'farm-2'
    await settle()
    expect(resource.calculateError.value).toBeNull()
  })

  it('does not show or propagate an adjustment answer after the field changed', async () => {
    installNuxtFakes(createTestApi().api)
    let release: (value: FieldTerrain) => void = () => {}
    let changed = 0
    const service: TerrainService = {
      ...scripted([farm(null)]).service,
      adjust: () => new Promise<FieldTerrain>((resolve) => { release = resolve }),
    }
    const fieldId = ref<string | null>('field-1')
    const resource = useFieldTerrain('farm-1', fieldId, { service, onChanged: async () => { changed += 1 } })
    await settle()
    const pending = resource.adjust({ origin: 'field_assessment', slopeMeanPct: 1, slopeMaxPct: 2, position: 'slope', justification: 'x' })
    fieldId.value = 'field-2'
    await settle()
    release(field({ fieldId: 'field-1', source: 'manual' }))
    await pending
    expect(resource.terrain.value?.fieldId).toBe('field-2')
    expect(resource.terrain.value?.source).toBeNull()
    expect(changed).toBe(0)
  })
})
