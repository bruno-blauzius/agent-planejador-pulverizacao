import { computed, readonly } from 'vue'
import type { Credentials, User } from '~/types/auth'
import { ApiError, toApiError } from '~/services/errors'
import { isProductAccessDenied } from '~/utils/login'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useAuthToken } from './useAuthToken'
import { useLogoutCleanups } from './useLogoutCleanup'

/**
 * Authenticated session, shared across screens through `useState` (SSR-safe).
 * `pending` and `error` are per instance: they reflect the calls made through
 * THIS `useSession()` call, so a login form and a middleware each see only
 * their own request state.
 *
 * Two flavours, chosen by the composition root (ADR 0010/0015):
 * - `sso` (real API): the session is the HttpOnly `agro_session` cookie, which
 *   JavaScript cannot see. The only way to know whether it is valid is to ask
 *   `GET /auth/me`, so `canRestore` is always true. Signing in is a redirect
 *   (`signInUrl`); there is no token in the front.
 * - `password` (dev mock): token in a cookie (`useAuthToken`) and a form.
 */
export function useSession() {
  const { auth } = useApiServices()
  const passwordAuth = auth.kind === 'password' ? auth : null
  // Only the mock keeps a token; the real path never creates that cookie.
  const tokenStore = passwordAuth ? useAuthToken() : null
  const user = useState<User | null>('session:user', () => null)
  const runLogoutCleanups = useLogoutCleanups()

  const loginAction = useAction(async (credentials: Credentials) => {
    if (!passwordAuth) {
      throw new ApiError({ code: 'unknown_error', message: 'O login por senha não está disponível; entre pelo SSO.', status: 0 })
    }
    const session = await passwordAuth.login(credentials)
    // Page data cached by useAsyncData belongs to the previous session (another
    // user may see other companies/farms): drop it before the new user navigates.
    clearNuxtData()
    tokenStore?.setToken(session.accessToken, session.expiresAt)
    user.value = session.user
    return session.user
  })

  const fetchAction = useAction(async () => {
    if (tokenStore && !tokenStore.token.value) {
      user.value = null
      return null
    }
    try {
      user.value = await auth.getCurrentUser()
      return user.value
    }
    catch (error) {
      // An expired/revoked session, or a denied product, must not linger: drop it, then report.
      const apiError = toApiError(error)
      if (apiError.code === 'unauthorized' || isProductAccessDenied(apiError)) clearLocal()
      throw error
    }
  })

  const logoutAction = useAction(async () => {
    // Server first: if it fails the session is still valid, and forgetting it
    // locally would send the user to /login only to be signed straight back in.
    await auth.logout()
    clearLocal()
    clearNuxtData()
    runLogoutCleanups()
    return true as const
  })

  function clearLocal() {
    tokenStore?.clearToken()
    user.value = null
  }

  return {
    user: readonly(user),
    isAuthenticated: computed(() => Boolean(user.value)),
    /** `sso`: redirect login on the real API; `password`: the dev mock form. */
    authKind: auth.kind,
    /** Whether `fetchCurrentUser` can find a session: always for the cookie, only with a token for the mock. */
    canRestore: computed(() => (tokenStore ? Boolean(tokenStore.token.value) : true)),
    pending: computed(() => loginAction.pending.value || fetchAction.pending.value || logoutAction.pending.value),
    error: computed(() => loginAction.error.value ?? fetchAction.error.value ?? logoutAction.error.value),
    /** Dev mock only. Resolves with the user, or `null` when credentials are rejected (see `error`). */
    login: loginAction.execute,
    /** Absolute URL of the SSO login (full-page redirect), or `null` in the dev mock. */
    signInUrl: (returnTo: string): string | null => (auth.kind === 'sso' ? auth.loginUrl(returnTo) : null),
    /** Loads the user of the current session (route middleware, after a profile change). */
    fetchCurrentUser: fetchAction.execute,
    /** Ends the session on the server, then locally. Resolves `true`, or `null` when the server refused (see `error`). */
    logout: logoutAction.execute,
    /**
     * Forgets the session locally without calling the API: the API already said
     * it is gone (401 on a page call). Runs the registered cleanups too.
     */
    expire() {
      clearLocal()
      clearNuxtData()
      runLogoutCleanups()
    },
  }
}
