import { readonly } from 'vue'

export const AUTH_TOKEN_COOKIE = 'agro_access_token'

/**
 * Bearer token of services/api, kept in a cookie (not localStorage) so SSR
 * requests carry it too. The cookie expires together with the token
 * (`expires_at` from the login); `expires` is a function because Nuxt resolves
 * it on every write, after `setToken` has stored the new expiry.
 *
 * Temporary: the cookie is NOT httpOnly because the client writes it. When the
 * API exists, the recommended path is an httpOnly cookie set by services/api
 * (ADR 0010, adendo).
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
