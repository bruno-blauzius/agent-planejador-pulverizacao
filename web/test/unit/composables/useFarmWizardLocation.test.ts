import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useFarmWizard } from '~/composables/useFarmWizard'
import type { WizardGeolocation } from '~/composables/useFarmWizardLocation'
import { ApiError } from '~/services/errors'
import type { ApiServices } from '~/services/ports'
import type { GeoJsonPolygon } from '~/types/geo'
import { LOCATION_MESSAGES } from '~/utils/wizardLocation'
import { createTestApi } from '../../support/mockApi'
import { installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const NOW = () => new Date('2026-11-21T12:00:00Z')

const SQUARE: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.29], [-46.72, -21.31]]],
}

function wizardAt(
  query: Record<string, unknown> = { step: 'location', source: 'address' },
  overrides: Partial<ApiServices> = {},
  geolocation?: WizardGeolocation | null,
) {
  const fakes = installNuxtFakes({ ...createTestApi().api, ...overrides })
  fakes.route.query = query
  return useFarmWizard({ storage: null, now: NOW, geolocation })
}

function kml(name = 'fazenda-santa-rita.kml', size = 48 * 1024): File {
  const file = new File(['<kml/>'], name)
  Object.defineProperty(file, 'size', { value: size })
  return file
}

describe('useFarmWizard — step 2 (location)', () => {
  it('opens step 2 in the mode given by ?source=', () => {
    const wizard = wizardAt({ step: 'location', source: 'pin' })
    expect(wizard.currentStep.value).toBe('location')
    expect(wizard.location.value.mode).toBe('pin')
  })

  it('imports a file: official area, centroid as headquarters, summary and preview', async () => {
    const wizard = wizardAt({ step: 'location', source: 'file' })

    expect(await wizard.importPerimeterFile(kml())).toBe(true)

    const location = wizard.location.value
    expect(location.perimeterMethod).toBe('file')
    expect(location.perimeterAreaHa).toBeGreaterThan(0)
    expect(location.forecastPoint).not.toBeNull()
    expect(location.latitude).not.toBe('')
    expect(wizard.importSummary.value).toMatchObject({
      detailLabel: '48 KB · KML',
      cityLabel: 'Guaxupé · MG',
      datumLabel: 'SIRGAS 2000',
      fieldCount: 14,
    })
    // Município/UF identified by the API fill the empty inputs.
    expect([location.city, location.state]).toEqual(['Guaxupé', 'MG'])
    expect(wizard.headquartersWarning.value).toBeNull()
    expect(wizard.perimeterAreaLabel.value).not.toContain('aproximada')
    expect(wizard.preview.value.rows[0]?.value).toBe(wizard.importSummary.value?.areaLabel)
    expect(wizard.importErrorMessage.value).toBeNull()
  })

  it('keeps a headquarters and a city already set when importing, warning when outside', async () => {
    const wizard = wizardAt()
    wizard.location.value.city = 'Muzambinho'
    wizard.setForecastPoint({ lat: -21.5, lng: -46.5 })

    await wizard.importPerimeterFile(kml())

    expect(wizard.location.value.forecastPoint).toEqual({ lat: -21.5, lng: -46.5 })
    expect(wizard.location.value.city).toBe('Muzambinho')
    expect(wizard.headquartersWarning.value).toBe(LOCATION_MESSAGES.headquartersOutside)
  })

  it('reports import failures honestly, before and after the upload', async () => {
    const base = createTestApi().api
    const importPerimeter = vi.fn(async () => {
      throw new ApiError({ code: 'validation_failed', message: 'inválido', status: 422 })
    })
    const wizard = wizardAt({ step: 'location', source: 'address' }, { farms: { ...base.farms, importPerimeter } })

    expect(await wizard.importPerimeterFile(kml('mapa.pdf'))).toBe(false)
    expect(wizard.importErrorMessage.value).toBe(LOCATION_MESSAGES.importFormat)
    expect(importPerimeter).not.toHaveBeenCalled()

    expect(await wizard.importPerimeterFile(kml('grande.kml', 21 * 1024 * 1024))).toBe(false)
    expect(wizard.importErrorMessage.value).toBe(LOCATION_MESSAGES.importTooLarge)

    expect(await wizard.importPerimeterFile(kml())).toBe(false)
    expect(wizard.importErrorMessage.value).toBe(LOCATION_MESSAGES.importNoPolygon)
    expect(wizard.location.value.perimeter).toBeNull()
  })

  it('previews a drawn perimeter with an approximate area and drops the import', async () => {
    const wizard = wizardAt()
    await wizard.importPerimeterFile(kml())

    wizard.setDrawnPerimeter(SQUARE)

    expect(wizard.location.value.importedFileName).toBeNull()
    expect(wizard.importSummary.value).toBeNull()
    expect(wizard.perimeterAreaLabel.value).toMatch(/^≈ .+ ha · aproximada$/)
    expect(wizard.preview.value.rows[0]?.value).toBe(wizard.perimeterAreaLabel.value)
  })

  it('typed coordinates drive the forecast point; invalid text keeps the last valid one', async () => {
    const wizard = wizardAt()

    wizard.location.value.latitude = '-21,3045'
    wizard.location.value.longitude = '-46,7128'
    await nextTick()
    expect(wizard.location.value.forecastPoint).toEqual({ lat: -21.3045, lng: -46.7128 })
    expect(wizard.preview.value.rows[3]?.value).toBe('-21,30 · -46,71')

    wizard.location.value.longitude = '-46,'
    await nextTick()
    expect(wizard.location.value.forecastPoint).toEqual({ lat: -21.3045, lng: -46.7128 })
    wizard.touch('longitude')
    expect(wizard.fieldErrors.value.longitude).toBe(LOCATION_MESSAGES.longitudeInvalid)

    wizard.location.value.latitude = ''
    wizard.location.value.longitude = ''
    await nextTick()
    expect(wizard.location.value.forecastPoint).toBeNull()

    expect(wizard.pasteCoordinates('-21,30, -46,71')).toBe(true)
    expect(wizard.location.value.latitude).toBe('-21,3')
    expect(wizard.pasteCoordinates('Guaxupé')).toBe(false)
  })

  it('uses the browser location', async () => {
    const position = { coords: { latitude: -21.3, longitude: -46.71 } } as GeolocationPosition
    const granted: WizardGeolocation = { getCurrentPosition: success => success(position) }
    const wizard = wizardAt({ step: 'location', source: 'address' }, {}, granted)

    expect(await wizard.locateMe()).toBe(true)
    expect(wizard.location.value.forecastPoint).toEqual({ lat: -21.3, lng: -46.71 })
    expect(wizard.locating.value).toBe(false)
  })

  it('explains a denied permission and a browser without location', async () => {
    const denied: WizardGeolocation = {
      getCurrentPosition: (_success, failure) => failure?.({ code: 1 } as GeolocationPositionError),
    }
    const blocked = wizardAt({ step: 'location', source: 'address' }, {}, denied)
    expect(await blocked.locateMe()).toBe(false)
    expect(blocked.locateError.value).toBe(LOCATION_MESSAGES.geolocationDenied)

    vi.unstubAllGlobals()
    const unavailable = wizardAt({ step: 'location', source: 'address' }, {}, null)
    expect(await unavailable.locateMe()).toBe(false)
    expect(unavailable.locateError.value).toBe(LOCATION_MESSAGES.geolocationUnavailable)
  })

  it('undoes draws and imports, restoring the previous perimeter', async () => {
    const wizard = wizardAt()
    expect(wizard.canUndoPerimeter.value).toBe(false)

    await wizard.importPerimeterFile(kml())
    const imported = wizard.location.value.perimeter
    wizard.setDrawnPerimeter(SQUARE)
    wizard.setDrawnPerimeter(SQUARE)
    expect(wizard.canUndoPerimeter.value).toBe(true)

    wizard.undoPerimeter()
    expect(wizard.location.value.perimeter).toEqual(imported)
    expect(wizard.importSummary.value?.fileName).toBe('fazenda-santa-rita.kml')

    wizard.undoPerimeter()
    expect(wizard.location.value.perimeter).toBeNull()
    expect(wizard.canUndoPerimeter.value).toBe(false)
  })

  it('undoing an import also reverts the headquarters and the município it filled', async () => {
    const wizard = wizardAt()
    await wizard.importPerimeterFile(kml())
    expect(wizard.location.value.city).toBe('Guaxupé')

    wizard.undoPerimeter()

    expect(wizard.location.value).toMatchObject({
      perimeter: null, forecastPoint: null, latitude: '', longitude: '', city: '', state: '',
    })
  })

  it('undoing a drawing keeps what the user typed afterwards', () => {
    const wizard = wizardAt()
    wizard.setDrawnPerimeter(SQUARE)
    wizard.location.value.city = 'Guaxupé'

    wizard.undoPerimeter()

    expect(wizard.location.value.perimeter).toBeNull()
    expect(wizard.location.value.city).toBe('Guaxupé')
  })

  it('opens step 2 with ?source= alone', () => {
    const wizard = wizardAt({ source: 'file' })
    expect(wizard.currentStep.value).toBe('location')
    expect(wizard.location.value.mode).toBe('file')
  })
})
