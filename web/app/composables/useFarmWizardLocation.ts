import { computed, readonly, ref, shallowRef, watch, type Ref } from 'vue'
import type { GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { FarmWizardDraft, LocationDraft } from '~/types/wizard'
import {
  buildImportSummary,
  formatCoordinateInput,
  importErrorMessage,
  isHeadquartersOutside,
  LOCATION_MESSAGES,
  parseCoordinatesQuery,
  perimeterAreaLabel,
  PERIMETER_IMPORT_ACCEPT,
  pointFromInputs,
  precheckPerimeterFile,
} from '~/utils/wizardLocation'
import { useFarmActions } from './useFarmActions'

/** Only what the wizard uses from the browser Geolocation API (tests pass a fake). */
export type WizardGeolocation = Pick<Geolocation, 'getCurrentPosition'>

export interface UseFarmWizardLocationOptions {
  /** Default: `navigator.geolocation` in the browser, none on the server. */
  geolocation?: WizardGeolocation | null
}

/**
 * Fields restored by "Desfazer": the perimeter ones always, plus what the
 * import filled (headquarters, município/UF) and only then, so undoing a
 * drawing never reverts something the user typed afterwards.
 */
type PerimeterSnapshot = Pick<LocationDraft,
  | 'perimeterMethod' | 'perimeter' | 'perimeterAreaHa' | 'importedFileName' | 'importedFileSize'
  | 'importedSourceFormat' | 'importedCity' | 'importedState' | 'importedDatum' | 'importedFieldCount'>
  & Partial<Pick<LocationDraft, 'forecastPoint' | 'latitude' | 'longitude' | 'city' | 'state'>>

const GEOLOCATION_TIMEOUT_MS = 10_000
const PERMISSION_DENIED = 1
/** Below this difference the typed text and the stored point are the same place. */
const SAME_POINT_EPSILON = 1e-6

function browserGeolocation(): WizardGeolocation | null {
  if (!import.meta.client || !('geolocation' in navigator)) return null
  return navigator.geolocation
}

function samePoint(a: GeoPoint | null, b: GeoPoint | null): boolean {
  if (!a || !b) return a === b
  return Math.abs(a.lat - b.lat) < SAME_POINT_EPSILON && Math.abs(a.lng - b.lng) < SAME_POINT_EPSILON
}

/**
 * Step 2 of the new-farm wizard (Figma 14:30, 36:472, 37:503): perimeter drawn
 * or imported, optional headquarters (map pin, typed coordinates or the
 * browser location) and honest messages for every failure. Works on the
 * wizard draft owned by `useFarmWizard`.
 */
export function useFarmWizardLocation(draft: Ref<FarmWizardDraft>, options: UseFarmWizardLocationOptions = {}) {
  const actions = useFarmActions()
  const geolocation = options.geolocation !== undefined ? options.geolocation : browserGeolocation()

  const precheckError = ref<string | null>(null)
  /** Perimeter states before each draw/import, for "Desfazer" (this visit only). */
  const perimeterHistory = shallowRef<readonly PerimeterSnapshot[]>([])

  function snapshot(): PerimeterSnapshot {
    const { perimeterMethod, perimeter, perimeterAreaHa, importedFileName, importedFileSize, importedSourceFormat,
      importedCity, importedState, importedDatum, importedFieldCount } = draft.value.location
    return { perimeterMethod, perimeter, perimeterAreaHa, importedFileName, importedFileSize, importedSourceFormat,
      importedCity, importedState, importedDatum, importedFieldCount }
  }

  function remember(extra: Partial<PerimeterSnapshot> = {}) {
    perimeterHistory.value = [...perimeterHistory.value, { ...snapshot(), ...extra }]
  }

  /** "Desfazer": back to the perimeter before the last draw or import. */
  function undoPerimeter() {
    const previous = perimeterHistory.value.at(-1)
    if (!previous) return
    perimeterHistory.value = perimeterHistory.value.slice(0, -1)
    Object.assign(draft.value.location, previous)
  }
  const locating = ref(false)
  const locateError = ref<string | null>(null)

  /** Pinned on the map, from the browser, or pasted: also fills the inputs. */
  function setForecastPoint(point: GeoPoint | null) {
    Object.assign(draft.value.location, {
      forecastPoint: point,
      latitude: point ? formatCoordinateInput(point.lat) : '',
      longitude: point ? formatCoordinateInput(point.lng) : '',
    })
    locateError.value = null
  }

  /** From the drawing control (trilha C). The preview area is approximate. */
  function setDrawnPerimeter(perimeter: PerimeterGeometry | null) {
    // The drawing tool may report the same geometry again (e.g. when it reselects it).
    if (JSON.stringify(perimeter) === JSON.stringify(draft.value.location.perimeter)) return
    remember()
    Object.assign(draft.value.location, {
      perimeterMethod: 'draw',
      perimeter,
      perimeterAreaHa: null,
      importedFileName: null,
      importedFileSize: null,
      importedSourceFormat: null,
      importedCity: null,
      importedState: null,
      importedDatum: null,
      importedFieldCount: null,
    })
  }

  function clearPerimeter() {
    setDrawnPerimeter(null)
    precheckError.value = null
  }

  /** Current values of the fields the import is about to fill, for "Desfazer". */
  function importSideEffects(importedCity: string | null): Partial<PerimeterSnapshot> {
    const { forecastPoint, latitude, longitude, city, state } = draft.value.location
    return {
      ...(forecastPoint ? {} : { forecastPoint, latitude, longitude }),
      ...(importedCity && !city.trim() ? { city, state } : {}),
    }
  }

  /** Município/UF identified by the API fill the inputs only when still empty. */
  function fillCityFromImport(city: string | null, state: string | null) {
    const location = draft.value.location
    if (!city || location.city.trim()) return
    location.city = city
    if (state && !location.state.trim()) location.state = state
  }

  /**
   * `POST /perimeter-imports`: the API parses the file and returns the official
   * area and the centroid. The centroid becomes the forecast point only when no
   * headquarters was set yet.
   */
  async function importPerimeterFile(file: File): Promise<boolean> {
    precheckError.value = precheckPerimeterFile(file)
    if (precheckError.value) return false

    const result = await actions.importPerimeter(file)
    if (!result) return false
    remember(importSideEffects(result.city))
    Object.assign(draft.value.location, {
      perimeterMethod: 'file',
      perimeter: result.perimeter,
      perimeterAreaHa: result.areaHa,
      importedFileName: file.name,
      importedFileSize: file.size,
      importedSourceFormat: result.sourceFormat,
      importedCity: result.city,
      importedState: result.state,
      importedDatum: result.datum,
      importedFieldCount: result.fieldCount,
    })
    fillCityFromImport(result.city, result.state)
    if (!draft.value.location.forecastPoint) setForecastPoint(result.centroid)
    return true
  }

  /** Pasted "-21,30, -46,71" into a coordinate input; false when it is not a pair. */
  function pasteCoordinates(text: string): boolean {
    const point = parseCoordinatesQuery(text)
    if (point) setForecastPoint(point)
    return point !== null
  }

  /** "Usar minha localização". Resolves false with `locateError` set on failure. */
  function locateMe(): Promise<boolean> {
    if (!geolocation) {
      locateError.value = LOCATION_MESSAGES.geolocationUnavailable
      return Promise.resolve(false)
    }
    locating.value = true
    locateError.value = null
    return new Promise((resolve) => {
      geolocation.getCurrentPosition(
        (position) => {
          locating.value = false
          setForecastPoint({ lat: position.coords.latitude, lng: position.coords.longitude })
          resolve(true)
        },
        (error) => {
          locating.value = false
          locateError.value = error.code === PERMISSION_DENIED
            ? LOCATION_MESSAGES.geolocationDenied
            : LOCATION_MESSAGES.geolocationFailed
          resolve(false)
        },
        { enableHighAccuracy: true, timeout: GEOLOCATION_TIMEOUT_MS },
      )
    })
  }

  // Typed coordinates drive the forecast point; invalid text keeps the last valid point.
  watch(
    () => [draft.value.location.latitude, draft.value.location.longitude] as const,
    ([latitude, longitude]) => {
      const location = draft.value.location
      const point = pointFromInputs(latitude, longitude)
      if (point && !samePoint(point, location.forecastPoint)) location.forecastPoint = point
      else if (!latitude.trim() && !longitude.trim()) location.forecastPoint = null
    },
  )

  return {
    perimeterImportAccept: PERIMETER_IMPORT_ACCEPT,
    importPerimeterFile,
    importing: actions.importing,
    importError: actions.importError,
    /** Pre-check or API failure, ready to show (format, size, no polygon, network). */
    importErrorMessage: computed(() => {
      if (precheckError.value) return precheckError.value
      const error = actions.importError.value
      return error ? importErrorMessage(error) : null
    }),
    importSummary: computed(() => buildImportSummary(draft.value.location)),
    /** Map badge "Área do perímetro": official (import) or "≈ N ha · aproximada" (drawn). */
    perimeterAreaLabel: computed(() => perimeterAreaLabel(draft.value.location)),
    /** Non-blocking warning next to the headquarters ("A sede está fora do perímetro…"). */
    headquartersWarning: computed(() =>
      isHeadquartersOutside(draft.value.location) ? LOCATION_MESSAGES.headquartersOutside : null),
    setDrawnPerimeter,
    clearPerimeter,
    undoPerimeter,
    canUndoPerimeter: computed(() => perimeterHistory.value.length > 0),
    setForecastPoint,
    pasteCoordinates,
    locateMe,
    locating: readonly(locating),
    locateError: readonly(locateError),
  }
}
