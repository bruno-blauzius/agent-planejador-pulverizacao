/**
 * Sessions for planner tests that need the mock's fault switches. Plain
 * `signInAs` (support/session.ts) covers the happy path; this one also lets a
 * test flip `plannerFaults` (agent/weather down, stale review, blocked check).
 */
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useSession } from '~/composables/useSession'
import { createMockServices } from '~/services/mock/mockServices'
import type { PlannerFaults } from '~/services/mock/plannerServices'
import { TEST_PASSWORD } from '~/services/mock/seed'
import { installNuxtFakes, type NuxtFakes } from './nuxtFakes'
import { SEED_LOGINS, SEED_NOW } from './session'

export async function signInWithPlannerFaults(
  faults: PlannerFaults,
  identifier: string = SEED_LOGINS.admin,
): Promise<NuxtFakes> {
  const fakes = installNuxtFakes(nuxt => createMockServices({
    latencyMs: 0,
    getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value,
    now: () => SEED_NOW,
    plannerFaults: faults,
  }))
  const user = await useSession().login({ identifier, password: TEST_PASSWORD, rememberMe: false })
  if (!user) throw new Error(`seed login failed: ${identifier}`)
  return fakes
}

/** The seeded scenario of the Figma: Santa Rita, fields T3 and T7. */
export const SCENARIO = {
  farmId: 'farm-santa-rita',
  fieldT3: 'field-str-3',
  fieldT7: 'field-str-7',
} as const
