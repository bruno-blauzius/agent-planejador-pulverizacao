/**
 * Presentation rules of wizard step 2 (location): coordinates typed by hand,
 * pre-checks of the perimeter file, honest import messages and the area label.
 * The official area, the file parsing and the geometry validation belong to
 * the API (`POST /perimeter-imports`, `POST /farms`).
 */
import type { ApiError } from '~/services/errors'
import type { PerimeterSourceFormat } from '~/types/farm'
import type { GeoJsonPolygon, GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { LocationDraft, WizardFieldErrors } from '~/types/wizard'
import { formatCoordinates, formatHectares } from './format'
import { approximateAreaHa, isDrawablePolygon, verticesInside } from './geoArea'

export const LOCATION_MESSAGES = {
  latitudeInvalid: 'Latitude entre -90 e 90 (ex.: -21,3045).',
  longitudeInvalid: 'Longitude entre -180 e 180 (ex.: -46,7128).',
  coordinatesIncomplete: 'Informe latitude e longitude.',
  perimeterIncomplete: 'Feche o perímetro com pelo menos 3 vértices.',
  importFormat: 'Formato não suportado. Use KML, KMZ, GeoJSON ou Shapefile (.zip).',
  importTooLarge: 'Arquivo acima de 20 MB. Simplifique o arquivo ou envie só o perímetro da fazenda.',
  headquartersOutside: 'A sede está fora do perímetro. Confira o ponto ou o desenho.',
  importNoPolygon: 'Não encontramos um polígono válido no arquivo.',
  importFailed: 'Não foi possível importar o arquivo. Tente novamente.',
  geolocationDenied: 'Permita o acesso à localização no navegador ou digite as coordenadas.',
  geolocationFailed: 'Não foi possível obter sua localização. Digite as coordenadas ou marque no mapa.',
  geolocationUnavailable: 'Este navegador não informa a localização. Digite as coordenadas ou marque no mapa.',
} as const

/** Limit stated in the contract (`/perimeter-imports`: "Até 20 MB"); the API enforces it. */
export const PERIMETER_IMPORT_MAX_BYTES = 20 * 1024 * 1024

const EXTENSION_FORMATS: Readonly<Record<string, PerimeterSourceFormat>> = {
  kml: 'kml',
  kmz: 'kmz',
  geojson: 'geojson',
  json: 'geojson',
  zip: 'shapefile',
}

/** `accept` of the file input. */
export const PERIMETER_IMPORT_ACCEPT = '.kml,.kmz,.geojson,.json,.zip'

export const PERIMETER_FORMAT_LABELS: Readonly<Record<PerimeterSourceFormat, string>> = {
  kml: 'KML',
  kmz: 'KMZ',
  geojson: 'GeoJSON',
  shapefile: 'Shapefile',
}

const NUMBER = String.raw`-?\d+(?:[.,]\d+)?`
const COORDINATE_PAIR = new RegExp(String.raw`^\s*(${NUMBER})\s*[,;\s]\s*(${NUMBER})\s*$`)
const SINGLE_NUMBER = new RegExp(String.raw`^\s*(${NUMBER})\s*$`)

/** "-21,3045" or "-21.3045" → -21.3045; null when not a number. */
export function parseCoordinate(text: string): number | null {
  const match = SINGLE_NUMBER.exec(text)
  return match?.[1] ? Number(match[1].replace(',', '.')) : null
}

/** Value for the coordinate inputs (pt-BR decimal comma, up to 6 decimals). */
export function formatCoordinateInput(value: number): string {
  return new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 6, useGrouping: false }).format(value)
}

function inRange(value: number | null, limit: number): value is number {
  return value !== null && Math.abs(value) <= limit
}

/** Both inputs as a point; null while empty, incomplete or out of range. */
export function pointFromInputs(latitude: string, longitude: string): GeoPoint | null {
  const lat = parseCoordinate(latitude)
  const lng = parseCoordinate(longitude)
  return inRange(lat, 90) && inRange(lng, 180) ? { lat, lng } : null
}

/** "-21,30, -46,71", "-21.30 -46.71", "-21,30; -46,71" (latitude first) → point. */
export function parseCoordinatesQuery(text: string): GeoPoint | null {
  const match = COORDINATE_PAIR.exec(text)
  if (!match?.[1] || !match[2]) return null
  return pointFromInputs(match[1], match[2])
}

/** Optional headquarters: when one input is filled, both must be valid. */
export function validateCoordinateInputs(latitude: string, longitude: string): WizardFieldErrors {
  const errors: WizardFieldErrors = {}
  if (!latitude.trim() && !longitude.trim()) return errors
  if (!inRange(parseCoordinate(latitude), 90)) {
    errors.latitude = latitude.trim() ? LOCATION_MESSAGES.latitudeInvalid : LOCATION_MESSAGES.coordinatesIncomplete
  }
  if (!inRange(parseCoordinate(longitude), 180)) {
    errors.longitude = longitude.trim() ? LOCATION_MESSAGES.longitudeInvalid : LOCATION_MESSAGES.coordinatesIncomplete
  }
  return errors
}

/** A drawn polygon with fewer than 3 vertices cannot be saved yet. */
export function isIncompletePerimeter(perimeter: PerimeterGeometry | null): boolean {
  return perimeter?.type === 'Polygon' && !isDrawablePolygon(perimeter)
}

export function perimeterFormatOf(fileName: string): PerimeterSourceFormat | null {
  const extension = fileName.split('.').pop()?.toLowerCase() ?? ''
  return EXTENSION_FORMATS[extension] ?? null
}

/** Checks done before uploading, so an obviously wrong file never leaves the browser. */
export function precheckPerimeterFile(file: Pick<File, 'name' | 'size'>): string | null {
  if (!perimeterFormatOf(file.name)) return LOCATION_MESSAGES.importFormat
  if (file.size > PERIMETER_IMPORT_MAX_BYTES) return LOCATION_MESSAGES.importTooLarge
  return null
}

/** User-facing message of a failed import: the API reason when it gives one. */
export function importErrorMessage(error: ApiError): string {
  if (error.code === 'unsupported_media_type') return LOCATION_MESSAGES.importFormat
  if (error.code === 'payload_too_large') return LOCATION_MESSAGES.importTooLarge
  if (error.code === 'validation_failed') return error.details[0]?.message ?? LOCATION_MESSAGES.importNoPolygon
  return error.message || LOCATION_MESSAGES.importFailed
}

/**
 * "486,2 ha" when the API computed it (import); "≈ 41,2 ha · aproximada" for a
 * drawn perimeter (ADR 0012: preview only, the official area comes on save).
 */
export function perimeterAreaLabel(location: Pick<LocationDraft, 'perimeter' | 'perimeterAreaHa'>): string | null {
  if (location.perimeterAreaHa !== null) return formatHectares(location.perimeterAreaHa)
  const { perimeter } = location
  if (!perimeter || isIncompletePerimeter(perimeter)) return null
  return `≈ ${formatHectares(approximateAreaHa(perimeter))} · aproximada`
}

function fileSizeLabel(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 }).format(bytes / (1024 * 1024))} MB`
}

/** Result card of the import (Figma 37:503). */
export interface PerimeterImportSummary {
  readonly fileName: string
  /** "48 KB · KML". */
  readonly detailLabel: string
  readonly areaLabel: string
  /** "Guaxupé · MG"; "—" when the API did not identify it. */
  readonly cityLabel: string
  /** Centroid returned by the API, or the headquarters typed/pinned later. */
  readonly forecastPointLabel: string
  /** "SIRGAS 2000"; "—" when unknown. */
  readonly datumLabel: string
  /** Inner polygons (possible fields): banner "O arquivo já tem os talhões" when > 0. */
  readonly fieldCount: number | null
}

export function buildImportSummary(location: LocationDraft): PerimeterImportSummary | null {
  if (!location.importedFileName || location.perimeterAreaHa === null) return null
  const parts = [
    location.importedFileSize !== null ? fileSizeLabel(location.importedFileSize) : null,
    location.importedSourceFormat ? PERIMETER_FORMAT_LABELS[location.importedSourceFormat] : null,
  ].filter(part => part !== null)
  return {
    fileName: location.importedFileName,
    detailLabel: parts.join(' · '),
    areaLabel: formatHectares(location.perimeterAreaHa),
    cityLabel: location.importedCity
      ? [location.importedCity, location.importedState].filter(Boolean).join(' · ')
      : '—',
    forecastPointLabel: formatCoordinates(location.forecastPoint),
    datumLabel: location.importedDatum ?? '—',
    fieldCount: location.importedFieldCount,
  }
}

/** Non-blocking warning: the headquarters lies outside the perimeter. */
export function isHeadquartersOutside(location: Pick<LocationDraft, 'forecastPoint' | 'perimeter'>): boolean {
  const { forecastPoint, perimeter } = location
  if (!forecastPoint || !perimeter || isIncompletePerimeter(perimeter)) return false
  // A one-vertex "polygon" reuses the vertex-inside check of geoArea.
  const point: GeoJsonPolygon = { type: 'Polygon', coordinates: [[[forecastPoint.lng, forecastPoint.lat]]] }
  return !verticesInside(point, perimeter)
}
