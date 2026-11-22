import { createMockServices } from '~/services/mock/mockServices'
import type { ApiServices } from '~/services/ports'

export const SEED_USER_EMAIL = 'bruno@agentesagro.test'
export const SEED_USER_PASSWORD = 'mock-password'
export const SEED_USER_TOKEN = 'mock-token:user-bruno'

/** Mock services with no latency and a mutable token, as the plugin wires them. */
export function createTestApi(initialToken: string | null = SEED_USER_TOKEN): {
  api: ApiServices
  setToken: (token: string | null) => void
} {
  let token = initialToken
  const api = createMockServices({
    latencyMs: 0,
    getAccessToken: () => token,
    now: () => new Date('2026-11-21T12:00:00Z'),
  })
  return { api, setToken: value => (token = value) }
}
