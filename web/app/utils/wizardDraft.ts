/**
 * Local persistence of the new-farm wizard draft. The stored text is untrusted
 * (old version, edited by hand, another app on the origin): `parseDraft`
 * validates it at the edge and falls back to "no draft".
 */
import type { PerimeterSourceFormat, ProductionType } from '~/types/farm'
import type { GeoJsonPosition, GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { FarmWizardDraft, IdentificationDraft, LocationDraft, WizardOwnerShare } from '~/types/wizard'
import { createEmptyDraft, parseWizardStep } from './wizard'

/** Port of the draft storage; tests use an in-memory fake. */
export interface WizardDraftStorage {
  load(): string | null
  save(serialized: string): void
  clear(): void
}

export const WIZARD_DRAFT_STORAGE_KEY = 'agro:farm-wizard-draft:v1'

type UnknownRecord = Record<string, unknown>

const PRODUCTION_TYPES: readonly ProductionType[] = ['coffee', 'grains', 'livestock', 'mixed']
const PERIMETER_FORMATS: readonly PerimeterSourceFormat[] = ['kml', 'kmz', 'geojson', 'shapefile']

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringOr(value: unknown, fallback: string): string {
  return typeof value === 'string' ? value : fallback
}

function nullableNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function nullableString(value: unknown): string | null {
  return typeof value === 'string' ? value : null
}

function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  return allowed.find(item => item === value) ?? fallback
}

function parsePoint(value: unknown): GeoPoint | null {
  if (!isRecord(value)) return null
  const lat = nullableNumber(value.lat)
  const lng = nullableNumber(value.lng)
  return lat !== null && lng !== null ? { lat, lng } : null
}

function parsePosition(value: unknown): GeoJsonPosition | null {
  if (!Array.isArray(value) || value.length < 2) return null
  const [lng, lat] = value
  return typeof lng === 'number' && typeof lat === 'number' ? [lng, lat] : null
}

function parseRing(value: unknown): GeoJsonPosition[] | null {
  if (!Array.isArray(value)) return null
  const positions = value.map(parsePosition)
  return positions.every(position => position !== null) ? positions.filter(position => position !== null) : null
}

function parseRings(value: unknown): GeoJsonPosition[][] | null {
  if (!Array.isArray(value) || value.length === 0) return null
  const rings = value.map(parseRing)
  return rings.every(ring => ring !== null) ? rings.filter(ring => ring !== null) : null
}

/** Structural check only: the geometry itself is re-validated by the API on submit. */
function parsePerimeter(value: unknown): PerimeterGeometry | null {
  if (!isRecord(value) || !Array.isArray(value.coordinates)) return null
  if (value.type === 'Polygon') {
    const rings = parseRings(value.coordinates)
    return rings ? { type: 'Polygon', coordinates: rings } : null
  }
  if (value.type !== 'MultiPolygon' || value.coordinates.length === 0) return null
  const polygons = value.coordinates.map(parseRings)
  return polygons.every(polygon => polygon !== null)
    ? { type: 'MultiPolygon', coordinates: polygons.filter(polygon => polygon !== null) }
    : null
}

function parseIdentification(value: unknown): IdentificationDraft {
  const empty = createEmptyDraft().identification
  if (!isRecord(value)) return empty
  return {
    name: stringOr(value.name, ''),
    code: stringOr(value.code, ''),
    cooperative: stringOr(value.cooperative, ''),
    productionType: PRODUCTION_TYPES.find(type => type === value.productionType) ?? null,
  }
}

function parseLocation(value: unknown): LocationDraft {
  const empty = createEmptyDraft().location
  if (!isRecord(value)) return empty
  return {
    mode: oneOf(value.mode, ['address', 'pin', 'file'] as const, empty.mode),
    perimeterMethod: oneOf(value.perimeterMethod, ['draw', 'file', 'car'] as const, empty.perimeterMethod),
    postalCode: stringOr(value.postalCode, ''),
    city: stringOr(value.city, ''),
    state: stringOr(value.state, ''),
    carNumber: stringOr(value.carNumber, ''),
    perimeter: parsePerimeter(value.perimeter),
    perimeterAreaHa: nullableNumber(value.perimeterAreaHa),
    forecastPoint: parsePoint(value.forecastPoint),
    latitude: stringOr(value.latitude, ''),
    longitude: stringOr(value.longitude, ''),
    importedFileName: nullableString(value.importedFileName),
    importedFileSize: nullableNumber(value.importedFileSize),
    importedSourceFormat: PERIMETER_FORMATS.find(format => format === value.importedSourceFormat) ?? null,
    importedCity: nullableString(value.importedCity),
    importedState: nullableString(value.importedState),
    importedDatum: nullableString(value.importedDatum),
    importedFieldCount: nullableNumber(value.importedFieldCount),
  }
}

function parseOwner(value: unknown): WizardOwnerShare | null {
  if (!isRecord(value) || typeof value.ownerId !== 'string' || typeof value.name !== 'string') return null
  return {
    ownerId: value.ownerId,
    name: value.name,
    personType: oneOf(value.personType, ['individual', 'company'] as const, 'individual'),
    documentLabel: stringOr(value.documentLabel, ''),
    role: oneOf(value.role, ['owner', 'tenant', 'legal_representative'] as const, 'owner'),
    sharePercent: nullableNumber(value.sharePercent),
    isNewlyRegistered: value.isNewlyRegistered === true,
  }
}

export function serializeDraft(draft: FarmWizardDraft): string {
  return JSON.stringify(draft)
}

/** Null for missing, corrupt or other-version drafts. */
export function parseDraft(raw: string | null): FarmWizardDraft | null {
  if (!raw) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  }
  catch {
    return null
  }
  if (!isRecord(parsed) || parsed.version !== 1) return null
  const owners = Array.isArray(parsed.owners) ? parsed.owners.map(parseOwner) : []
  return {
    version: 1,
    identification: parseIdentification(parsed.identification),
    location: parseLocation(parsed.location),
    owners: owners.filter((owner): owner is WizardOwnerShare => owner !== null),
    confirmed: parsed.confirmed === true,
    furthestStep: parseWizardStep(parsed.furthestStep) ?? 'identification',
    savedAt: nullableString(parsed.savedAt),
    userId: nullableString(parsed.userId),
  }
}

/**
 * `localStorage` adapter, or null on the server. Storage can throw (private
 * mode, quota, blocked site data): the wizard keeps working in memory and the
 * failure is reported in the console, never hidden.
 */
export function browserDraftStorage(key: string = WIZARD_DRAFT_STORAGE_KEY): WizardDraftStorage | null {
  if (!import.meta.client) return null
  const attempt = <T>(action: () => T, fallback: T): T => {
    try {
      return action()
    }
    catch (error) {
      console.warn('[farm-wizard] draft storage unavailable', error)
      return fallback
    }
  }
  return {
    load: () => attempt(() => window.localStorage.getItem(key), null),
    save: serialized => attempt(() => window.localStorage.setItem(key, serialized), undefined),
    clear: () => attempt(() => window.localStorage.removeItem(key), undefined),
  }
}
