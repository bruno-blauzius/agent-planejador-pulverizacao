/**
 * Signed-in test sessions shared by every track. Single owner: trilha G
 * (logic). Do not rewrite it from another track: add exports instead.
 *
 * Exports in use: `signInAs`, `SEED_LOGINS`, `SEED_NOW` and the aliases
 * `installSignedIn`/`TEST_USERS` (permission tests).
 */
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useSession } from '~/composables/useSession'
import { createMockServices } from '~/services/mock/mockServices'
import { TEST_PASSWORD } from '~/services/mock/seed'
import { installNuxtFakes, type NuxtFakes } from './nuxtFakes'

/** Reference date of the seed (last access, invitation expiry). */
export const SEED_NOW = new Date('2026-11-21T12:00:00Z')

/** Seeded test users by role (ADR 0013); all use `TEST_PASSWORD` from the seed. */
export const SEED_LOGINS = {
  superAdmin: 'bruno@agentesagro.test',
  admin: 'marina@fazendasantarita.test',
  operator: 'carlos.mendes@email.test',
} as const

/**
 * Nuxt fakes + mock services whose token follows the session cookie (as
 * `plugins/api.ts` wires them), already signed in as `identifier`. Role,
 * capabilities and farm scope come from the mock session of that user.
 */
export async function signInAs(identifier: string = SEED_LOGINS.superAdmin): Promise<NuxtFakes> {
  const fakes = installNuxtFakes(nuxt => createMockServices({
    latencyMs: 0,
    getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value,
    now: () => SEED_NOW,
  }))
  const user = await useSession().login({ identifier, password: TEST_PASSWORD, rememberMe: false })
  if (!user) throw new Error(`seed login failed: ${identifier}`)
  return fakes
}

/** Alias of `SEED_LOGINS` (permission tests). */
export const TEST_USERS = SEED_LOGINS

/** Alias of `signInAs` (permission tests). */
export const installSignedIn = signInAs
