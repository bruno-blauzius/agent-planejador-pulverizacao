/**
 * A resource value that a user action can replace with the response it got
 * (for example the 202 of "calculate relief"), until a newer load arrives. Not
 * auto-imported (nested folder).
 *
 * The action result wins over a GET that started before the action ended: such
 * a late answer describes the world before the action (no `progress` yet).
 * Loads are marked with `trackLoad`; a load that starts after the action
 * replaces the value as usual.
 */
import { computed, shallowRef, watch, type ComputedRef, type Ref } from 'vue'

export interface LatestValue<T> {
  readonly value: ComputedRef<T>
  /** Shows `next` instead of the loaded data until a load that started afterwards (or `resetOn`) changes it. */
  readonly set: (next: T) => void
  /** Runs a load, remembering whether it started before the latest `set`. */
  readonly trackLoad: (load: () => Promise<void>) => Promise<void>
}

export function useLatestValue<T>(source: Readonly<Ref<T>>, resetOn: () => unknown = () => null): LatestValue<T> {
  const latest = shallowRef<{ readonly value: T } | null>(null)
  let version = 0
  let loadStartedAt: number | null = null

  // Another farm/field: whatever an action returned belongs to the previous one.
  watch(resetOn, () => {
    latest.value = null
    version += 1
  }, { flush: 'sync' })

  watch(source, () => {
    if (loadStartedAt !== null && loadStartedAt < version) return
    latest.value = null
  }, { flush: 'sync' })

  async function trackLoad(load: () => Promise<void>): Promise<void> {
    loadStartedAt = version
    try {
      await load()
    }
    finally {
      loadStartedAt = null
    }
  }

  return {
    value: computed(() => (latest.value ? latest.value.value : source.value)),
    set: (next) => {
      version += 1
      latest.value = { value: next }
    },
    trackLoad,
  }
}
