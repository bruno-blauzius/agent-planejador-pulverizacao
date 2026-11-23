/**
 * Post-login redirect guard against open redirects: only same-origin paths of
 * this app are accepted; anything else falls back to the default page.
 */
import type { LoginErrorCode } from './login'

export const DEFAULT_AUTHENTICATED_PATH = '/farms'
export const LOGIN_PATH = '/login'

const PROBE_ORIGIN = 'http://app.invalid'

function hasControlChars(value: string): boolean {
  return [...value].some((char) => {
    const code = char.charCodeAt(0)
    return code <= 0x1F || code === 0x7F
  })
}

/** Accepts the raw `route.query.redirect` (string, array or null). */
export function safeRedirect(target: unknown, fallback: string = DEFAULT_AUTHENTICATED_PATH): string {
  const candidate = Array.isArray(target) ? target[0] : target
  if (typeof candidate !== 'string') return fallback
  // Protocol-relative ("//host"), backslash tricks ("/\host") and control chars
  // are rejected before parsing, since browsers normalize them into other hosts.
  if (!candidate.startsWith('/') || candidate.startsWith('//') || candidate.includes('\\') || hasControlChars(candidate)) {
    return fallback
  }
  let url: URL
  try {
    url = new URL(candidate, PROBE_ORIGIN)
  }
  catch {
    return fallback
  }
  if (url.origin !== PROBE_ORIGIN) return fallback
  if (url.pathname === LOGIN_PATH) return fallback
  // Dot segments ("/.//host", "/a/..//host", "/%2e//host") normalize into "//host",
  // which a router or browser reads as protocol-relative: validate the result too.
  if (!url.pathname.startsWith('/') || url.pathname.startsWith('//')) return fallback
  return `${url.pathname}${url.search}${url.hash}`
}

export interface AuthGuardInput {
  readonly path: string
  readonly fullPath: string
  readonly isPublic: boolean
  readonly redirect: unknown
  readonly isAuthenticated: boolean
  /** Landing page of the signed-in user (`homePathFor(permissions)`); defaults to /farms. */
  readonly homePath?: string
  /** Why the session check failed (denied product, API down); null for a plain "not signed in". */
  readonly loginError?: LoginErrorCode | null
}

export type AuthGuardTarget = { path: string, query?: Record<string, string> } | null

/**
 * Decision of the global auth middleware: `null` lets the navigation through.
 * Public pages are open; a signed-in user on /login goes to the safe redirect;
 * a guest on a private page goes to /login keeping where they wanted to go, with
 * `error` when the check failed for a reason the user must read (the public
 * /login never checks again, so this cannot loop).
 */
export function resolveAuthRedirect(input: AuthGuardInput): AuthGuardTarget {
  if (input.isPublic) {
    return input.isAuthenticated && input.path === LOGIN_PATH
      ? { path: safeRedirect(input.redirect, input.homePath ?? DEFAULT_AUTHENTICATED_PATH) }
      : null
  }
  if (input.isAuthenticated) return null
  const query: Record<string, string> = input.loginError
    ? { error: input.loginError, redirect: input.fullPath }
    : { redirect: input.fullPath }
  return { path: LOGIN_PATH, query }
}

export interface PermissionGuardInput {
  readonly path: string
  /** `definePageMeta({ permission })` of the target page. */
  readonly required: string | undefined
  readonly granted: readonly string[]
  readonly homePath: string
}

export interface PermissionGuardTarget {
  readonly path: string
  /** Show "Acesso não permitido": false when the user merely landed on "/" without the dashboard. */
  readonly notify: boolean
}

/**
 * Page-level capability check (ADR 0013, presentation only: the API still
 * answers 403). Denied pages send the user to their home page.
 */
export function resolvePermissionRedirect(input: PermissionGuardInput): PermissionGuardTarget | null {
  if (!input.required || input.granted.includes(input.required)) return null
  const path = input.homePath !== input.path ? input.homePath : DEFAULT_AUTHENTICATED_PATH
  // Both candidates are the denied page itself: nothing safer to offer, let the page show its 403 state.
  if (path === input.path) return null
  return { path, notify: input.path !== '/' }
}
