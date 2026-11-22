import type { Crop } from './crop'
import type { GeoJsonPolygon } from './geo'
import type { SprayCondition } from './spray'

/** Talhão (glossário: Field). */
export interface Field {
  readonly id: string
  readonly farmId: string
  readonly name: string
  /** Área (bloco) que agrupa talhões, ex. "Área Norte". */
  readonly block: string | null
  /** Computed by the API from the polygon. */
  readonly areaHa: number
  readonly crop: Crop
  readonly variety: string | null
  /** Safra, ex. "2026/27". */
  readonly season: string | null
  /** ISO date `YYYY-MM-DD`. */
  readonly plantingDate: string | null
  readonly rowSpacingM: number | null
  readonly plantSpacingM: number | null
  /** Computed by the API from the spacing. */
  readonly plantsPerHa: number | null
  readonly polygon: GeoJsonPolygon
  readonly sprayCondition: SprayCondition | null
  readonly lastApplicationAt: Date | null
}

export interface FieldInput {
  readonly name: string
  readonly block: string | null
  readonly cropCode: string
  readonly variety: string | null
  readonly season: string | null
  readonly plantingDate: string | null
  readonly rowSpacingM: number | null
  readonly plantSpacingM: number | null
  readonly polygon: GeoJsonPolygon
}

/** Fields grouped by block for the "Áreas e talhões" panel. */
export interface FieldBlock {
  readonly name: string | null
  readonly areaHa: number
  readonly fields: readonly Field[]
}
