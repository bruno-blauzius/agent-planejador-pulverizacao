import type { Crop } from './crop'
import type { GeoPoint, PerimeterGeometry } from './geo'
import type { LinkRole } from './owner'
import type { PageRequest } from './pagination'
import type { SprayOutlook } from './spray'

/** draft = rascunho do assistente; pending = cadastro com pendências. */
export type FarmStatus = 'draft' | 'active' | 'pending'
export type ProductionType = 'coffee' | 'grains' | 'livestock' | 'mixed'

export interface FarmRef {
  readonly id: string
  readonly name: string
}

export interface FarmOwnerShare {
  readonly ownerId: string
  readonly name: string
  readonly role: LinkRole
  /** 0–100; null for tenants and legal representatives. */
  readonly sharePercent: number | null
}

export interface FarmSummary {
  /** Owning company (tenant, ADR 0013). */
  readonly companyId: string
  readonly id: string
  readonly name: string
  /** Apelido/código curto, ex. "STR-01". */
  readonly code: string | null
  readonly city: string
  /** UF. */
  readonly state: string
  readonly status: FarmStatus
  readonly productionType: ProductionType
  readonly totalAreaHa: number
  readonly productiveAreaHa: number
  readonly fieldCount: number
  readonly crops: readonly Crop[]
  readonly owners: readonly FarmOwnerShare[]
  readonly sprayOutlook: SprayOutlook | null
  /** Map marker of the list. */
  readonly forecastPoint: GeoPoint | null
  /** Simplified perimeter for the card thumbnail and the map view. */
  readonly perimeter: PerimeterGeometry | null
  readonly createdAt: Date
}

export interface FarmLocation {
  /** CEP, digits only. */
  readonly postalCode: string | null
  /** Número do CAR. */
  readonly carNumber: string | null
  /** Ponto de previsão (sede ou centróide do perímetro). */
  readonly forecastPoint: GeoPoint | null
  readonly perimeter: PerimeterGeometry | null
}

export interface Farm extends FarmSummary {
  readonly cooperative: string | null
  readonly location: FarmLocation
  readonly updatedAt: Date
}

export interface FarmFilters extends PageRequest {
  readonly search?: string
  readonly status?: FarmStatus
  readonly productionType?: ProductionType
}

export interface FarmOwnerShareInput {
  readonly ownerId: string
  readonly role: LinkRole
  readonly sharePercent: number | null
}

/** Payload of the new-farm wizard (create) and of the edit form (update). */
export interface FarmInput {
  readonly name: string
  readonly code: string | null
  readonly cooperative: string | null
  readonly productionType: ProductionType
  readonly city: string
  readonly state: string
  /** `pending` is assigned by the API, never sent. */
  readonly status: Exclude<FarmStatus, 'pending'>
  readonly location: FarmLocation
  readonly owners: readonly FarmOwnerShareInput[]
}

export type PerimeterSourceFormat = 'kml' | 'kmz' | 'geojson' | 'shapefile'

export interface PerimeterImport {
  readonly sourceFormat: PerimeterSourceFormat
  readonly perimeter: PerimeterGeometry
  /** Computed by the API. */
  readonly areaHa: number
  readonly centroid: GeoPoint
  /** Município/UF of the centroid, when the API identifies them. */
  readonly city: string | null
  readonly state: string | null
  /** Reference system of the source file, e.g. "SIRGAS 2000". */
  readonly datum: string | null
  /** Inner polygons found in the file (possible fields). */
  readonly fieldCount: number | null
}
