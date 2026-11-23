/**
 * Composition root of the front (ADR 0010): the only place that chooses between
 * the HTTP adapters (services/api) and the in-memory mock. Composables reach the
 * result through `useApiServices()`, never by importing an implementation.
 *
 * Real API (ADR 0015): the session is the HttpOnly `agro_session` cookie set by
 * the API. The browser sends it with `credentials: 'include'` (web and API on
 * different ports of localhost are same-site, so SameSite=Lax cookies travel);
 * during SSR the request's `cookie` header is forwarded, since a server-side
 * fetch has no cookie jar. No Authorization header, no token in the front.
 */
import { createHttpClient, type RawFetch } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { ApiServices } from '~/services/ports'
import type { User } from '~/types/auth'
import { useAuthToken } from '~/composables/useAuthToken'
import { useSessionExpiry } from '~/composables/useSessionExpiry'

/**
 * The mock (and its seed with test credentials) is loaded only in dev builds:
 * `import.meta.dev` is replaced by `false` in production, so the bundler drops
 * this branch and the dynamic import with it, and no public env var can turn
 * the fake login on in a deployed app.
 */
async function loadMockServices(getAccessToken: () => string | null, latencyMs: number): Promise<ApiServices | null> {
  if (!import.meta.dev) return null
  const { createMockServices } = await import('~/services/mock/mockServices')
  return createMockServices({ latencyMs, getAccessToken })
}

/** Dev builds only (same reasoning as above): endpoints still `planned` on the in-memory mock. */
async function loadPendingMocks(real: ApiServices, getRole: () => User['role'] | undefined, latencyMs: number): Promise<ApiServices | null> {
  if (!import.meta.dev) return null
  const { createPendingMockServices } = await import('~/services/mock/pendingServices')
  return createPendingMockServices(real, { latencyMs, getRole })
}

/** The plugin runs on every SSR request: warn about an ignored flag once per process, not per request. */
let warnedPendingIgnored = false

/** `$fetch.raw` so the headers (e.g. `X-Invitation-Email`) reach the services; the body is what callers get. */
const rawFetch: RawFetch = async (path, { onHeaders, ...options }) => {
  const response = await $fetch.raw(path, options)
  onHeaders?.(response.headers)
  return response._data
}

export default defineNuxtPlugin(async (nuxtApp) => {
  const config = useRuntimeConfig()

  if (config.public.apiMock) {
    // The token cookie belongs to the mock only; the real path never touches it.
    const { token } = useAuthToken()
    const mock = await loadMockServices(() => token.value, Number(config.public.apiMockLatencyMs) || 0)
    if (mock) return { provide: { api: mock } }
    console.warn('[api] NUXT_PUBLIC_API_MOCK is ignored outside dev builds; using services/api.')
  }

  const baseURL = import.meta.server && config.apiBaseUrl ? config.apiBaseUrl : config.public.apiBase
  const requestHeaders = useRequestHeaders(['cookie'])
  const http = createHttpClient(
    {
      baseURL,
      credentials: 'include',
      getRequestHeaders: () => requestHeaders,
      onUnauthorized: path => nuxtApp.runWithContext(() => useSessionExpiry().handle(path)),
    },
    rawFetch,
  )
  // The login redirect leaves the app, so its URL is the public one, not the SSR-internal address.
  const real: ApiServices = createHttpServices(http, config.public.apiBase)
  if (!config.public.apiMockPending) return { provide: { api: real } }

  const sessionUser = useState<User | null>('session:user', () => null)
  const withMocks = await loadPendingMocks(real, () => sessionUser.value?.role, Number(config.public.apiMockLatencyMs) || 0)
  if (!withMocks && !warnedPendingIgnored) {
    warnedPendingIgnored = true
    console.warn('[api] NUXT_PUBLIC_API_MOCK_PENDING is ignored outside dev builds; using services/api only.')
  }
  return { provide: { api: withMocks ?? real } }
})
