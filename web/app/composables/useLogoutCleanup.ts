/**
 * Registry of "on logout" cleanups, so the generic session does not know the
 * features that keep per-user data in the browser (e.g. the new-farm draft).
 * Keyed by the Nuxt app instance: one registry per request on the server and
 * one in the browser, released with the app (no state shared across requests).
 */
type LogoutCleanup = () => void

const registries = new WeakMap<object, Map<string, LogoutCleanup>>()

function registryFor(app: object): Map<string, LogoutCleanup> {
  let registry = registries.get(app)
  if (!registry) {
    registry = new Map()
    registries.set(app, registry)
  }
  return registry
}

/** Registers (or replaces, by `key`) a cleanup that runs on logout. */
export function registerLogoutCleanup(key: string, cleanup: LogoutCleanup): void {
  registryFor(useNuxtApp()).set(key, cleanup)
}

/** Returns a function that runs every registered cleanup (used by `useSession().logout`). */
export function useLogoutCleanups(): () => void {
  const registry = registryFor(useNuxtApp())
  return () => {
    for (const cleanup of registry.values()) cleanup()
  }
}
