import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type ComputedRef,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import type { ApiError } from '~/services/errors'
import type { TerrainService } from '~/services/ports'
import type { FarmTerrain, TerrainCalculationScope } from '~/types/terrain'
import { TERRAIN_POLL_INTERVAL_MS } from '~/utils/terrain'
import { useAction, useResource } from './internal/resource'
import { useLatestValue } from './internal/latest'
import { useApiServices } from './useApiServices'

export interface FarmTerrainResource {
  /** Null before the first load (or while the tab is not active). */
  readonly terrain: Readonly<Ref<FarmTerrain | null>>
  /** First load only: a refresh with data on screen is not "pending". */
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  /** A requested calculation is running (`progress !== null`). */
  readonly isRunning: ComputedRef<boolean>
  /** Resolves with the farm relief of the 202 answer, or null on failure (see `calculateError`). */
  readonly calculate: (scope: TerrainCalculationScope) => Promise<FarmTerrain | null>
  /** The request is in flight. */
  readonly calculating: Readonly<Ref<boolean>>
  readonly calculateError: Readonly<Ref<ApiError | null>>
  /** Dismisses `calculateError`; it also clears itself when the farm changes. */
  readonly clearCalculateError: () => void
}

export interface FarmTerrainOptions {
  /** False while the tab is hidden: nothing is loaded or polled. */
  readonly active?: MaybeRefOrGetter<boolean>
  readonly pollIntervalMs?: number
  /** Test seam; defaults to the app's `terrain` service. */
  readonly service?: TerrainService
}

/**
 * Relief of a farm (tab "Relevo"). While a calculation runs it asks again every
 * `pollIntervalMs`, only in the browser and only with the tab active, until
 * `progress` is gone; it stops when the tab changes or the scope is disposed.
 */
export function useFarmTerrain(farmId: MaybeRefOrGetter<string>, options: FarmTerrainOptions = {}): FarmTerrainResource {
  const service = options.service ?? useApiServices().terrain
  const interval = options.pollIntervalMs ?? TERRAIN_POLL_INTERVAL_MS
  const isActive = () => toValue(options.active ?? true)

  const resource = useResource<FarmTerrain | null>(
    () => `terrain:farm:${toValue(farmId)}`,
    () => (isActive() ? service.getFarm(toValue(farmId)) : Promise.resolve(null)),
    () => null,
  )
  const latest = useLatestValue(resource.data, () => toValue(farmId))
  const terrain = latest.value

  // The tab may become active after the page rendered.
  watch(isActive, (active) => {
    if (active) void resource.refresh()
  })

  const calculation = useAction(async (scope: TerrainCalculationScope) => {
    const requestedFarm = toValue(farmId)
    const next = await service.calculate(requestedFarm, scope)
    // The answer of another farm must not show up on this one.
    if (toValue(farmId) === requestedFarm) latest.set(next)
    return next
  })

  watch(() => toValue(farmId), calculation.clearError)

  // Polling and manual refreshes are marked, so a GET that began before a `calculate` cannot overwrite its answer.
  const refresh = () => latest.trackLoad(resource.refresh)

  const isRunning = computed(() => Boolean(terrain.value?.progress))

  // `onMounted` never runs on the server, which keeps the polling in the browser.
  const mounted = ref(false)
  onMounted(() => {
    mounted.value = true
  })
  const shouldPoll = computed(() => mounted.value && isActive() && isRunning.value)
  let timer: ReturnType<typeof setTimeout> | undefined

  function stopPolling(): void {
    clearTimeout(timer)
    timer = undefined
  }

  function schedulePoll(): void {
    stopPolling()
    timer = setTimeout(async () => {
      timer = undefined
      if (!shouldPoll.value) return
      await refresh()
      if (shouldPoll.value) schedulePoll()
    }, interval)
  }

  watch(shouldPoll, (polling) => {
    if (polling) schedulePoll()
    else stopPolling()
  }, { immediate: true })
  if (getCurrentScope()) onScopeDispose(stopPolling)

  return {
    terrain,
    pending: computed(() => resource.pending.value && terrain.value === null),
    error: resource.error,
    refresh,
    isRunning,
    calculate: calculation.execute,
    calculating: calculation.pending,
    calculateError: calculation.error,
    clearCalculateError: calculation.clearError,
  }
}
