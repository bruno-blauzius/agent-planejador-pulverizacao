/**
 * Global auth guard. Pages opt out with `definePageMeta({ public: true })`
 * (e.g. /login) and require a capability with `definePageMeta({ permission })`
 * (ADR 0013). Decisions live in `resolveAuthRedirect`/`resolvePermissionRedirect`.
 */
import { useFlash } from '~/composables/useFlash'
import { usePermissions } from '~/composables/usePermissions'
import { useSession } from '~/composables/useSession'
import { PERMISSION_DENIED_FLASH } from '~/utils/permissions'
import { resolveAuthRedirect, resolvePermissionRedirect } from '~/utils/redirect'

export default defineNuxtRouteMiddleware(async (to) => {
  const session = useSession()

  // Token from a previous visit but no user yet (reload, SSR): restore it.
  // An invalid token is dropped by useSession, which falls through to /login.
  if (session.hasToken.value && !session.user.value) await session.fetchCurrentUser()

  const permissions = usePermissions()
  const target = resolveAuthRedirect({
    path: to.path,
    fullPath: to.fullPath,
    isPublic: to.meta.public === true,
    redirect: to.query.redirect,
    isAuthenticated: session.isAuthenticated.value,
    homePath: permissions.homePath.value,
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
