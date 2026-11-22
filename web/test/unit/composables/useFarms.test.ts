import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFarm } from '~/composables/useFarm'
import { useFarmActions } from '~/composables/useFarmActions'
import { useFarms } from '~/composables/useFarms'
import type { FarmFilters } from '~/types/farm'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

describe('useFarms', () => {
  it('starts pending and then exposes farms and display totals', async () => {
    installNuxtFakes(createTestApi().api)
    const { farms, summary, pending, isEmpty } = useFarms()

    expect(pending.value).toBe(true)
    expect(isEmpty.value).toBe(false)
    await flushPromises()

    expect(pending.value).toBe(false)
    expect(farms.value).toHaveLength(4)
    expect(summary.value).toMatchObject({
      farmCount: 4,
      totalAreaHa: 1284.2,
      pendingCount: 1,
      countByProductionType: { coffee: 3, grains: 1, livestock: 0, mixed: 0 },
    })
  })

  it('refetches when the filters change', async () => {
    installNuxtFakes(createTestApi().api)
    const filters = ref<FarmFilters>({})
    const { farms } = useFarms(filters)
    await flushPromises()

    filters.value = { productionType: 'grains' }
    await flushPromises()

    expect(farms.value.map(farm => farm.name)).toEqual(['Fazenda Recanto'])
  })

  it('reports empty results distinctly from errors', async () => {
    installNuxtFakes(createTestApi().api)
    const { isEmpty, error } = useFarms({ search: 'inexistente' })
    await flushPromises()

    expect(isEmpty.value).toBe(true)
    expect(error.value).toBeNull()
  })

  it('surfaces API failures as ApiError, not as an empty list', async () => {
    installNuxtFakes(createTestApi(null).api)
    const { error, isEmpty } = useFarms()
    await flushPromises()

    expect(error.value).toMatchObject({ code: 'unauthorized', status: 401 })
    expect(isEmpty.value).toBe(false)
  })
})

describe('useFarm', () => {
  it('loads the detail and reports 404', async () => {
    installNuxtFakes(createTestApi().api)
    const found = useFarm('farm-santa-rita')
    const missing = useFarm('farm-unknown')
    await flushPromises()

    expect(found.farm.value?.cooperative).toBe('Cooxupé')
    expect(missing.farm.value).toBeNull()
    expect(missing.error.value?.code).toBe('not_found')
  })
})

describe('useFarmActions', () => {
  it('keeps field errors of a rejected save', async () => {
    installNuxtFakes(createTestApi().api)
    const { createFarm, saveError, saving } = useFarmActions()

    const result = await createFarm({
      name: '',
      code: null,
      cooperative: null,
      productionType: 'coffee',
      city: 'Guaxupé',
      state: 'MG',
      status: 'draft',
      location: { postalCode: null, carNumber: null, forecastPoint: null, perimeter: null },
      owners: [],
    })

    expect(result).toBeNull()
    expect(saving.value).toBe(false)
    expect(saveError.value?.details).toEqual([{ field: 'name', message: 'informe o nome da fazenda' }])
  })

  it('imports a perimeter and clears the previous error', async () => {
    installNuxtFakes(createTestApi().api)
    const { importPerimeter, importError } = useFarmActions()

    expect(await importPerimeter(new File(['x'], 'mapa.pdf'))).toBeNull()
    expect(importError.value?.code).toBe('unsupported_media_type')

    const imported = await importPerimeter(new File(['x'], 'fazenda.kmz'))
    expect(imported?.sourceFormat).toBe('kmz')
    expect(importError.value).toBeNull()
  })
})
