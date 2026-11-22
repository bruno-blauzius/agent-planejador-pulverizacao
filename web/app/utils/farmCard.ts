/** Farm card of the list (Figma 7:172), ready to render. Presentation only. */
import type { FlashMessage } from '~/types/flash'
import type { Farm, FarmSummary } from '~/types/farm'
import type { GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { SprayCondition } from '~/types/spray'
import { cropNames } from './farm'
import { formatHectares, initials } from './format'
import { SPRAY_CONDITION_LABELS } from './labels'

export interface FarmCardView {
  readonly id: string
  readonly name: string
  /** "Guaxupé · MG" */
  readonly locationLabel: string
  /** Badge computed by the backend; null without forecast. */
  readonly condition: { readonly value: SprayCondition, readonly label: string } | null
  /** "486 ha" */
  readonly areaLabel: string
  /** "14 talhões" */
  readonly fieldCountLabel: string
  /** "Café arábica · Milho"; empty when the farm has no fields. */
  readonly cropLabel: string
  readonly owners: readonly { readonly initials: string, readonly name: string }[]
  /** "2 janelas aptas hoje", from the API. */
  readonly headline: string | null
  /** Detail route (screen 9). */
  readonly to: string
  /** Map marker and simplified perimeter, drawn by the map library. */
  readonly forecastPoint: GeoPoint | null
  readonly perimeter: PerimeterGeometry | null
}

function fieldCountLabel(count: number): string {
  return count === 1 ? '1 talhão' : `${count} talhões`
}

export function toFarmCardView(farm: FarmSummary): FarmCardView {
  return {
    id: farm.id,
    name: farm.name,
    locationLabel: `${farm.city} · ${farm.state}`,
    condition: farm.sprayOutlook
      ? { value: farm.sprayOutlook.condition, label: SPRAY_CONDITION_LABELS[farm.sprayOutlook.condition] }
      : null,
    areaLabel: formatHectares(farm.totalAreaHa, 0),
    fieldCountLabel: fieldCountLabel(farm.fieldCount),
    cropLabel: cropNames(farm),
    owners: farm.owners.map(owner => ({ initials: initials(owner.name), name: owner.name })),
    headline: farm.sprayOutlook?.headline ?? null,
    to: `/farms/${encodeURIComponent(farm.id)}`,
    forecastPoint: farm.forecastPoint,
    perimeter: farm.perimeter,
  }
}

/** Toast shown on the list after the new-farm wizard saves (neutral text): "<nome> criada". */
export function farmCreatedFlash(farm: Pick<Farm, 'name' | 'totalAreaHa'>): FlashMessage {
  return {
    kind: 'success',
    title: `${farm.name.trim()} criada`,
    description: `Área de ${formatHectares(farm.totalAreaHa, 0)} cadastrada.`,
  }
}
