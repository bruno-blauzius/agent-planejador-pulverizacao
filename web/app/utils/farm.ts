import type { FarmOwnerShare, FarmSummary, ProductionType } from '~/types/farm'

export interface FarmListSummary {
  readonly farmCount: number
  readonly totalAreaHa: number
  readonly productiveAreaHa: number
  readonly fieldCount: number
  readonly cropCount: number
  readonly pendingCount: number
  readonly countByProductionType: Readonly<Record<ProductionType, number>>
}

function roundOneDecimal(value: number): number {
  return Math.round(value * 10) / 10
}

/** Display totals for the cards and filter chips of the farm list. */
export function summarizeFarms(farms: readonly FarmSummary[]): FarmListSummary {
  const countByProductionType: Record<ProductionType, number> = { coffee: 0, grains: 0, livestock: 0, mixed: 0 }
  const cropCodes = new Set<string>()
  let totalAreaHa = 0
  let productiveAreaHa = 0
  let fieldCount = 0
  let pendingCount = 0

  for (const farm of farms) {
    countByProductionType[farm.productionType] += 1
    farm.crops.forEach(crop => cropCodes.add(crop.code))
    totalAreaHa += farm.totalAreaHa
    productiveAreaHa += farm.productiveAreaHa
    fieldCount += farm.fieldCount
    if (farm.status === 'pending') pendingCount += 1
  }

  return {
    farmCount: farms.length,
    totalAreaHa: roundOneDecimal(totalAreaHa),
    productiveAreaHa: roundOneDecimal(productiveAreaHa),
    fieldCount,
    cropCount: cropCodes.size,
    pendingCount,
    countByProductionType,
  }
}

/**
 * Sum of the owner shares shown in the "Participação total" bar. Display only:
 * the API is the one that rejects a total different from 100%.
 */
export function totalSharePercent(shares: readonly Pick<FarmOwnerShare, 'role' | 'sharePercent'>[]): number {
  return roundOneDecimal(
    shares.reduce((sum, share) => sum + (share.role === 'owner' ? share.sharePercent ?? 0 : 0), 0),
  )
}

/** "Café arábica · Milho" for the farm card. */
export function cropNames(farm: Pick<FarmSummary, 'crops'>): string {
  return farm.crops.map(crop => crop.name).join(' · ')
}

/** Tabs of the farm list (Figma 7:134). */
export type FarmListTab = 'all' | 'coffee' | 'grains' | 'pending'
export const FARM_LIST_TABS: readonly FarmListTab[] = ['all', 'coffee', 'grains', 'pending']
export const FARM_LIST_TAB_LABELS: Readonly<Record<FarmListTab, string>> = {
  all: 'Todas',
  coffee: 'Café',
  grains: 'Grãos',
  pending: 'Com pendências',
}

export function matchesFarmTab(farm: Pick<FarmSummary, 'productionType' | 'status'>, tab: FarmListTab): boolean {
  if (tab === 'all') return true
  if (tab === 'pending') return farm.status === 'pending'
  return farm.productionType === tab
}

export function countFarmTabs(farms: readonly FarmSummary[]): Record<FarmListTab, number> {
  return {
    all: farms.length,
    coffee: farms.filter(farm => matchesFarmTab(farm, 'coffee')).length,
    grains: farms.filter(farm => matchesFarmTab(farm, 'grains')).length,
    pending: farms.filter(farm => matchesFarmTab(farm, 'pending')).length,
  }
}

/** Farms registered in the same calendar month as `now` ("+1 este mês"). */
export function countCreatedInMonth(farms: readonly Pick<FarmSummary, 'createdAt'>[], now: Date): number {
  return farms.filter(farm =>
    farm.createdAt.getFullYear() === now.getFullYear() && farm.createdAt.getMonth() === now.getMonth(),
  ).length
}
