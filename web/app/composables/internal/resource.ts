/**
 * Building blocks shared by the public composables. Not auto-imported (nested
 * folder) on purpose: pages and components use the `useX` composables only.
 */
import { computed, readonly, ref, shallowRef, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import { toApiError, type ApiError } from '~/services/errors'

export interface Resource<T> {
  readonly data: Readonly<Ref<T>>
  readonly pending: ComputedRef<boolean>
  /** Normalized error of the last load; null while loading or on success. */
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
}

/** Page data through `useAsyncData` (SSR + hydration) with a stable key. */
export function useResource<T>(
  key: MaybeRefOrGetter<string>,
  load: () => Promise<T>,
  fallback: () => T,
): Resource<T> {
  const asyncData = useAsyncData<T, unknown, T, [], T>(key, () => load(), { default: fallback })
  return {
    data: asyncData.data,
    pending: computed(() => asyncData.status.value === 'pending'),
    error: computed(() => (asyncData.error.value ? toApiError(asyncData.error.value) : null)),
    refresh: () => asyncData.refresh(),
  }
}

export interface Action<Args extends unknown[], R> {
  /** Resolves with the result, or with `null` when it failed (see `error`). */
  readonly execute: (...args: Args) => Promise<R | null>
  readonly pending: Readonly<Ref<boolean>>
  readonly error: Readonly<Ref<ApiError | null>>
}

/**
 * User-triggered call (submit, save, upload). Failures are not swallowed: they
 * are exposed in `error` (with per-field `details`) and the promise resolves to
 * `null`, so the UI never needs try/catch to stay consistent.
 */
export function useAction<Args extends unknown[], R>(run: (...args: Args) => Promise<R>): Action<Args, R> {
  const pending = ref(false)
  const error = shallowRef<ApiError | null>(null)

  async function execute(...args: Args): Promise<R | null> {
    pending.value = true
    error.value = null
    try {
      return await run(...args)
    }
    catch (caught) {
      error.value = toApiError(caught)
      return null
    }
    finally {
      pending.value = false
    }
  }

  return { execute, pending: readonly(pending), error: readonly(error) }
}

/** Deterministic cache key for a filter object (key order and empty values ignored). */
export function stableKey(prefix: string, params: MaybeRefOrGetter<object>): string {
  const entries = Object.entries(toValue(params))
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .sort(([a], [b]) => a.localeCompare(b))
  return entries.length ? `${prefix}?${new URLSearchParams(entries.map(([k, v]) => [k, String(v)]))}` : prefix
}
