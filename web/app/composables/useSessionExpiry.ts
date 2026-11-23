import { useSession } from './useSession'
import { LOGIN_PATH } from '~/utils/redirect'

/**
 * Reaction to a 401 on a page call (the session cookie expired, was revoked,
 * or the user was deactivated/removed): forget the user and send them to /login
 * keeping where they were. Wired by `plugins/api.ts` (`onUnauthorized`).
 *
 * Calls of the auth flow itself (`/auth/...`) are the session's own business
 * (middleware, logout) and never trigger it. Once the user is forgotten, the
 * remaining 401s of the same page (parallel requests) do nothing, so the
 * navigation happens once. The server render is left alone: its middleware
 * already decided, and a redirect there would race the page render.
 */
export function useSessionExpiry() {
  const session = useSession()
  const route = useRoute()

  return {
    handle(path: string): void {
      if (import.meta.server || path.startsWith('/auth/')) return
      if (!session.isAuthenticated.value) return
      session.expire()
      if (route.meta.public === true) return
      void navigateTo({ path: LOGIN_PATH, query: { redirect: route.fullPath } })
    },
  }
}
