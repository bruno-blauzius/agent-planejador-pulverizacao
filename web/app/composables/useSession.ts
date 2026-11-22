import { computed, readonly } from 'vue'
import type { Credentials, User } from '~/types/auth'
import { toApiError } from '~/services/errors'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useAuthToken } from './useAuthToken'
import { useLogoutCleanups } from './useLogoutCleanup'

/**
 * Authenticated session. Shared across screens: only `user` (`useState`,
 * SSR-safe) and the token cookie (`useAuthToken`). `pending` and `error` are
 * per instance: they reflect the calls made through THIS `useSession()` call,
 * so a login form and a middleware each see only their own request state.
 */
export function useSession() {
  const { auth } = useApiServices()
  const { token, setToken, clearToken } = useAuthToken()
  const user = useState<User | null>('session:user', () => null)
  const runLogoutCleanups = useLogoutCleanups()

  const loginAction = useAction(async (credentials: Credentials) => {
    const session = await auth.login(credentials)
    // Page data cached by useAsyncData belongs to the previous session (another
    // user may see other companies/farms): drop it before the new user navigates.
    clearNuxtData()
    setToken(session.accessToken, session.expiresAt)
    user.value = session.user
    return session.user
  })

  const fetchAction = useAction(async () => {
    if (!token.value) {
      user.value = null
      return null
    }
    try {
      user.value = await auth.getCurrentUser()
      return user.value
    }
    catch (error) {
      // An expired or revoked token must not linger: drop it, then report.
      if (toApiError(error).code === 'unauthorized') clear()
      throw error
    }
  })

  function clear() {
    clearToken()
    user.value = null
  }

  return {
    user: readonly(user),
    isAuthenticated: computed(() => Boolean(token.value && user.value)),
    hasToken: computed(() => Boolean(token.value)),
    pending: computed(() => loginAction.pending.value || fetchAction.pending.value),
    error: computed(() => loginAction.error.value ?? fetchAction.error.value),
    /** Resolves with the user, or `null` when credentials are rejected (see `error`). */
    login: loginAction.execute,
    /** Loads the user for an existing token (e.g. in a route middleware). */
    fetchCurrentUser: fetchAction.execute,
    /** Local logout (the contract has no server-side logout yet); runs the registered cleanups. */
    logout() {
      clear()
      clearNuxtData()
      runLogoutCleanups()
    },
  }
}
