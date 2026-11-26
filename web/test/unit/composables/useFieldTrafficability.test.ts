import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFieldTrafficability } from '~/composables/useFieldTrafficability'
import { ApiError } from '~/services/errors'
import type { TrafficabilityService } from '~/services/ports'
import type { FieldTrafficability } from '~/types/trafficability'
import { createTestApi } from '../../support/mockApi'
import { installNuxtFakes } from '../../support/nuxtFakes'

const settle = () => vi.advanceTimersByTimeAsync(0)

function answer(farmId: string, fieldId: string, state: FieldTrafficability['soil']['state'] = 'waterlogged'): FieldTrafficability {
  return {
    fieldId,
    farmId,
    asOf: new Date('2026-11-26T10:00:00Z'),
    soil: { state, basis: 'measured_rain', texture: 'clay', thresholdsSource: 'heuristic', rain: null, modelMoisture: null, inspection: null },
    rainDaily: [],
    days: [],
    releases: [],
    dataIssues: [],
  }
}

/** Fake service that records the calls and can be held to observe the loading states. */
function fakeService() {
  const calls: { farmId: string, fieldId: string, days: number | undefined }[] = []
  let state: FieldTrafficability['soil']['state'] = 'waterlogged'
  let gate: Promise<void> | null = null
  const service: TrafficabilityService = {
    async getFieldTrafficability(farmId, fieldId, days) {
      calls.push({ farmId, fieldId, days })
      if (gate) await gate
      return answer(farmId, fieldId, state)
    },
  }
  return {
    service,
    calls,
    setState: (next: typeof state) => { state = next },
    hold: () => {
      let release = () => {}
      gate = new Promise(resolve => { release = resolve })
      return () => {
        gate = null
        release()
      }
    },
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  installNuxtFakes(createTestApi().api)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('useFieldTrafficability', () => {
  it('loads the field and reports the first load as pending', async () => {
    const fake = fakeService()
    const resource = useFieldTrafficability('farm-1', 'field-4', { service: fake.service })
    expect(resource.pending.value).toBe(true)
    expect(resource.trafficability.value).toBeNull()
    await settle()
    expect(resource.pending.value).toBe(false)
    expect(resource.trafficability.value?.soil.state).toBe('waterlogged')
    expect(resource.error.value).toBeNull()
    expect(fake.calls).toEqual([{ farmId: 'farm-1', fieldId: 'field-4', days: undefined }])
  })

  it('calls nothing without a field id', async () => {
    const fake = fakeService()
    const resource = useFieldTrafficability('farm-1', null, { service: fake.service })
    await settle()
    expect(fake.calls).toHaveLength(0)
    expect(resource.trafficability.value).toBeNull()
    expect(resource.pending.value).toBe(false)
  })

  it('passes the horizon and reloads when it changes', async () => {
    const fake = fakeService()
    const days = ref<number | undefined>(3)
    useFieldTrafficability('farm-1', 'field-4', { service: fake.service, days })
    await settle()
    days.value = 7
    await settle()
    expect(fake.calls.map(call => call.days)).toEqual([3, 7])
  })

  it('never shows the previous field while the next one loads', async () => {
    const fake = fakeService()
    const fieldId = ref<string | null>('field-4')
    const resource = useFieldTrafficability('farm-1', fieldId, { service: fake.service })
    await settle()
    const release = fake.hold()
    fieldId.value = 'field-9'
    await settle()
    expect(resource.trafficability.value).toBeNull()
    expect(resource.pending.value).toBe(true)
    release()
    await settle()
    expect(resource.trafficability.value?.fieldId).toBe('field-9')
  })

  it('refreshes keeping the current answer on screen, then shows the new one', async () => {
    const fake = fakeService()
    const resource = useFieldTrafficability('farm-1', 'field-4', { service: fake.service })
    await settle()
    fake.setState('moist')
    const release = fake.hold()
    const refreshing = resource.refresh()
    await settle()
    expect(resource.refreshing.value).toBe(true)
    expect(resource.pending.value).toBe(false)
    expect(resource.trafficability.value?.soil.state).toBe('waterlogged')
    release()
    await refreshing
    expect(resource.refreshing.value).toBe(false)
    expect(resource.trafficability.value?.soil.state).toBe('moist')
  })

  it('exposes the API error and no data', async () => {
    const service: TrafficabilityService = {
      getFieldTrafficability: async () => { throw new ApiError({ code: 'not_found', message: 'talhão não encontrado', status: 404 }) },
    }
    const resource = useFieldTrafficability('farm-1', 'field-x', { service })
    await settle()
    expect(resource.error.value?.code).toBe('not_found')
    expect(resource.trafficability.value).toBeNull()
    expect(resource.pending.value).toBe(false)
  })

  it('uses the app service by default (mock world: Talhão 4 of Santa Rita)', async () => {
    const resource = useFieldTrafficability('farm-santa-rita', 'field-str-4')
    await settle()
    expect(resource.trafficability.value?.soil).toMatchObject({ state: 'waterlogged', texture: 'clay' })
  })
})

describe('useFieldTrafficability: horizon', () => {
  it('exposes the requested days, defaulting to the contract', () => {
    const fake = fakeService()
    expect(useFieldTrafficability('farm-1', 'field-4', { service: fake.service }).days.value).toBe(5)
    const days = ref<number | undefined>(2)
    const resource = useFieldTrafficability('farm-1', 'field-4', { service: fake.service, days })
    expect(resource.days.value).toBe(2)
    days.value = undefined
    expect(resource.days.value).toBe(5)
  })
})
