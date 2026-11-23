/**
 * Resolves a record opened through a query deep link (`?company=<id>`,
 * `?user=<id>`) that may not be on the page/filter currently listed: after the
 * list settles without it, the record is fetched by id.
 *
 * `not_found` means "does not exist (for this tenant)"; any other failure is a
 * load error, which the UI must not present as "not found".
 */
import { shallowRef, watch } from 'vue'
import { toApiError, type ApiError } from '~/services/errors'

export interface DeepLinkOptions<T> {
  /** Id taken from the query, or null when no deep link is open. */
  readonly id: () => string | null
  /** Looks the record up in the data already loaded for the list. */
  readonly find: (id: string) => T | null
  /** The list finished loading (successfully or not); only then a miss means "not in this page". */
  readonly listSettled: () => boolean
  /** False while the viewer may not open the record at all (no fetch is made). */
  readonly enabled?: () => boolean
  readonly load: (id: string) => Promise<T>
}

export interface DeepLinkResolution<T> {
  readonly record: T | null
  /** Looking for the record (list or fetch by id still running): a missing `record` is not final. */
  readonly loading: boolean
  /** Fetch failed for a reason other than `not_found`; `record` is null. */
  readonly loadError: ApiError | null
}

type Fetched<T>
  = | { id: string, status: 'pending' }
    | { id: string, status: 'found', record: T }
    | { id: string, status: 'missing' }
    | { id: string, status: 'error', error: ApiError }

export function useDeepLinkedRecord<T>(options: DeepLinkOptions<T>) {
  const fetched = shallowRef<Fetched<T> | null>(null)
  const isEnabled = () => options.enabled?.() ?? true

  async function fetchById(id: string): Promise<void> {
    fetched.value = { id, status: 'pending' }
    let next: Fetched<T>
    try {
      next = { id, status: 'found', record: await options.load(id) }
    }
    catch (caught) {
      const error = toApiError(caught)
      next = error.code === 'not_found' ? { id, status: 'missing' } : { id, status: 'error', error }
    }
    // The link changed while the request was in flight: drop the stale answer.
    if (fetched.value?.id === id) fetched.value = next
  }

  // The server render leaves the modal "loading"; the client fetches after hydration.
  watch(
    () => {
      const id = options.id()
      return { id, listed: id !== null && options.find(id) !== null, settled: options.listSettled(), enabled: isEnabled() }
    },
    ({ id, listed, settled, enabled }) => {
      if (import.meta.server) return
      if (!id) {
        fetched.value = null
        return
      }
      if (listed || !settled || !enabled || fetched.value?.id === id) return
      void fetchById(id)
    },
    { immediate: true },
  )

  /** Reactive resolution of `id` (call inside a computed). */
  function resolve(id: string): DeepLinkResolution<T> {
    const listed = options.find(id)
    if (listed) return { record: listed, loading: false, loadError: null }
    const current = fetched.value?.id === id ? fetched.value : null
    if (current?.status === 'found') return { record: current.record, loading: false, loadError: null }
    if (current?.status === 'missing') return { record: null, loading: false, loadError: null }
    if (current?.status === 'error') return { record: null, loading: false, loadError: current.error }
    return { record: null, loading: true, loadError: null }
  }

  return { resolve }
}
