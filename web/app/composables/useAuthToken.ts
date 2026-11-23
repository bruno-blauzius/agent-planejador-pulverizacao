import { readonly } from 'vue'

export const AUTH_TOKEN_COOKIE = 'agro_access_token'

/**
 * DEV MOCK ONLY (ADR 0010). Fake Bearer token of the in-memory mock, kept in a
 * cookie so SSR requests carry it too; it expires with the mock login's
 * `expiresAt` (`expires` is a function because Nuxt resolves it on every write).
 *
 * The real session (ADR 0015) is the HttpOnly `agro_session` cookie of
 * services/api: JavaScript never reads it and this composable is never called on
 * that path (`useSession` only creates it for the `password` flavour). Remove it
 * together with `services/mock/` when every endpoint exists.
 */
export function useAuthToken() {
  const expiresAt = useState<string | null>('session:tokenExpiresAt', () => null)
  const token = useCookie<string | null>(AUTH_TOKEN_COOKIE, {
    default: () => null,
    path: '/',
    sameSite: 'lax',
    secure: !import.meta.dev,
    expires: () => (expiresAt.value ? new Date(expiresAt.value) : undefined),
  })

  function setToken(accessToken: string, expiry: Date) {
    expiresAt.value = expiry.toISOString()
    token.value = accessToken
  }

  function clearToken() {
    token.value = null
    expiresAt.value = null
  }

  return { token: readonly(token), expiresAt: readonly(expiresAt), setToken, clearToken }
}
