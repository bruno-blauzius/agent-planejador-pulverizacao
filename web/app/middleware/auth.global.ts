/**
 * Global auth guard. Pages opt out with `definePageMeta({ public: true })`
 * (e.g. /login) and require a capability with `definePageMeta({ permission })`
 * (ADR 0013). Decisions live in `resolveAuthRedirect`/`resolvePermissionRedirect`.
 *
 * With the SSO (ADR 0015) the session is an HttpOnly cookie the front cannot
 * read, so a missing user is restored by asking `GET /auth/me` (on the server
 * the cookie of the request is forwarded by the HTTP client).
 */
import { useFlash } from '~/composables/useFlash'
import { usePermissions } from '~/composables/usePermissions'
import { useSession } from '~/composables/useSession'
import { isLoginErrorCode, loginErrorFor } from '~/utils/login'
import { PERMISSION_DENIED_FLASH } from '~/utils/permissions'
import { resolveAuthRedirect, resolvePermissionRedirect } from '~/utils/redirect'

export default defineNuxtRouteMiddleware(async (to) => {
  const session = useSession()
  const isPublic = to.meta.public === true
  // /login?error=... is where a failed check lands: asking again there would only repeat it.
  const alreadyReported = isLoginErrorCode(Array.isArray(to.query.error) ? to.query.error[0] : to.query.error)

  // No user yet (reload, SSR, first visit): restore it. 401 falls through to
  // /login; 403 (product denied) and outages are reported to the login screen.
  if (!session.user.value && session.canRestore.value && !(isPublic && alreadyReported)) {
    await session.fetchCurrentUser()
  }

  const permissions = usePermissions()
  const target = resolveAuthRedirect({
    path: to.path,
    fullPath: to.fullPath,
    isPublic,
    redirect: to.query.redirect,
    isAuthenticated: session.isAuthenticated.value,
    homePath: permissions.homePath.value,
    loginError: loginErrorFor(session.error.value),
  })
  if (target) return navigateTo(target)
  if (!session.isAuthenticated.value) return undefined

  const denied = resolvePermissionRedirect({
    path: to.path,
    required: to.meta.permission,
    granted: permissions.permissions.value,
    homePath: permissions.homePath.value,
  })
  if (!denied) return undefined
  if (denied.notify) useFlash().show(PERMISSION_DENIED_FLASH)
  return navigateTo(denied.path, { replace: true })
})
