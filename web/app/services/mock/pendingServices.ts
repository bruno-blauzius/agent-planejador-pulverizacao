/**
 * Dev bridge between the real API and the mock (ADR 0010, 0015).
 *
 * With the real API the session, users, invitations, companies, profile, farms,
 * fields, crops, owners, the relief (ADR 0021) and the trafficability of the
 * fields (ADR 0022) come from services/api, but the dashboard, the planner,
 * applications, "Hoje" and weather are still `planned` in the contract. This
 * module keeps those on the in-memory mock so the rest of the app stays usable
 * (the relief and trafficability mocks, `terrainMock.ts` and
 * `trafficabilityMock.ts`, now serve only `NUXT_PUBLIC_API_MOCK=true`). The mock only knows its own seed users, so each request
 * acts as the seed user that has the signed-in user's role (the real company
 * and farm ids do not exist in the mock). Data from the two sides is therefore
 * not consistent with each other; it is scaffolding, loaded in dev builds only,
 * and shrinks as endpoints are implemented.
 */
import type { ApiServices } from '../ports'
import type { UserRole } from '~/types/auth'
import { createMockServices } from './mockServices'

/** Mock services that replace the endpoints still `planned` while the real API serves the rest. */
export const PENDING_SERVICE_KEYS = [
  'dashboard',
  'planner',
  'applications',
  'today',
  'weather',
] as const satisfies readonly (keyof ApiServices)[]

/** Seed user (mock) that stands in for each real role. */
export const MOCK_USER_BY_ROLE: Readonly<Record<UserRole, string>> = {
  super_admin: 'user-bruno',
  admin: 'user-marina',
  planning_operator: 'user-carlos',
}

const MOCK_TOKEN_PREFIX = 'mock-token:'

export interface PendingMockOptions {
  readonly latencyMs: number
  /** Role of the signed-in (real) user; null/undefined while signed out. */
  readonly getRole: () => UserRole | null | undefined
}

/** `real` with the still-`planned` services swapped for the mock. */
export function createPendingMockServices(real: ApiServices, options: PendingMockOptions): ApiServices {
  const mock = createMockServices({
    latencyMs: options.latencyMs,
    getAccessToken: () => {
      const role = options.getRole()
      return role ? `${MOCK_TOKEN_PREFIX}${MOCK_USER_BY_ROLE[role]}` : null
    },
  })
  const replacements = Object.fromEntries(PENDING_SERVICE_KEYS.map(key => [key, mock[key]]))
  return { ...real, ...replacements }
}
