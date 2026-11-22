/**
 * Composition root of the front (ADR 0010): the only place that chooses between
 * the HTTP adapters (services/api) and the in-memory mock. Composables reach the
 * result through `useApiServices()`, never by importing an implementation.
 */
import { createHttpClient } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { ApiServices } from '~/services/ports'

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

export default defineNuxtPlugin(async () => {
  const config = useRuntimeConfig()
  const { token } = useAuthToken()
  const getAccessToken = () => token.value

  if (config.public.apiMock) {
    const mock = await loadMockServices(getAccessToken, Number(config.public.apiMockLatencyMs) || 0)
    if (mock) return { provide: { api: mock } }
    console.warn('[api] NUXT_PUBLIC_API_MOCK is ignored outside dev builds; using services/api.')
  }

  const baseURL = import.meta.server && config.apiBaseUrl ? config.apiBaseUrl : config.public.apiBase
  const http = createHttpClient({ baseURL, getAccessToken }, (path, options) => $fetch(path, options))
  const api: ApiServices = createHttpServices(http)
  return { provide: { api } }
})
