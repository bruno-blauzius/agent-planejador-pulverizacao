import { computed, reactive, ref, watch } from 'vue'
import type { ApiError } from '~/services/errors'
import {
  LOGIN_ERROR_TITLE,
  LOGIN_MESSAGES,
  loginErrorMessage,
  validateLoginForm,
  type LoginField,
  type LoginFieldErrors,
} from '~/utils/login'
import { homePathFor } from '~/utils/permissions'
import { safeRedirect } from '~/utils/redirect'
import { useSession } from './useSession'

function messageFor(error: ApiError, hasFieldDetails: boolean): string | null {
  if (error.code === 'unauthorized') return LOGIN_MESSAGES.invalidCredentials
  if (error.code === 'network_error') return LOGIN_MESSAGES.network
  if (error.code === 'validation_failed' && hasFieldDetails) return null
  return LOGIN_MESSAGES.generic
}

function fieldErrorsFrom(error: ApiError | null): LoginFieldErrors {
  const errors: LoginFieldErrors = {}
  for (const detail of error?.details ?? []) {
    if (detail.field === 'identifier' || detail.field === 'password') errors[detail.field] = detail.message
  }
  return errors
}

/** Where the SSO sends the user back to when there is no `?redirect=`; the route guard picks the real home. */
const SSO_DEFAULT_RETURN_TO = '/'

/**
 * State and submit of the login screen.
 *
 * Real API (`kind: 'sso'`, ADR 0015): there is no form. `ssoUrl` is the
 * `GET /auth/login?return_to=` link (full-page redirect to the Keycloak login)
 * and `accessError` explains why a previous attempt or session check failed
 * (`?error=<code>`: not invited, no access, ambiguous role, inactive, SSO down).
 *
 * Dev mock (`kind: 'password'`): local errors show after blur (`touch`) or
 * submit; API errors show after submit and hide as soon as the user edits a
 * field. On success it navigates to the safe `?redirect=` or the user's home
 * page (`homePathFor`: "/" with `dashboard.view`, otherwise /farms).
 */
export function useLoginForm() {
  const session = useSession()
  const route = useRoute()

  const identifier = ref('')
  const password = ref('')
  const rememberMe = ref(true)
  const touched = reactive<Record<LoginField, boolean>>({ identifier: false, password: false })
  const showApiError = ref(false)

  const localErrors = computed(() => validateLoginForm({ identifier: identifier.value, password: password.value }))
  const apiError = computed(() => (showApiError.value ? session.error.value : null))

  const fieldErrors = computed<LoginFieldErrors>(() => {
    const visibleLocal: LoginFieldErrors = {}
    for (const field of ['identifier', 'password'] as const) {
      if (touched[field] && localErrors.value[field]) visibleLocal[field] = localErrors.value[field]
    }
    return { ...fieldErrorsFrom(apiError.value), ...visibleLocal }
  })

  const formError = computed(() => {
    const error = apiError.value
    return error ? messageFor(error, Object.keys(fieldErrorsFrom(error)).length > 0) : null
  })

  watch([identifier, password], () => {
    showApiError.value = false
  })

  function touch(field: LoginField) {
    touched[field] = true
  }

  /** Resolves `true` after a successful login and navigation. */
  async function submit(): Promise<boolean> {
    touch('identifier')
    touch('password')
    if (Object.keys(localErrors.value).length > 0 || session.pending.value) return false

    const user = await session.login({
      identifier: identifier.value,
      password: password.value,
      rememberMe: rememberMe.value,
    })
    showApiError.value = user === null
    if (!user) return false

    await navigateTo(safeRedirect(route.query.redirect, homePathFor(user.permissions)))
    return true
  }

  const ssoUrl = computed(() => session.signInUrl(safeRedirect(route.query.redirect, SSO_DEFAULT_RETURN_TO)))
  const accessError = computed(() => {
    const message = loginErrorMessage(route.query.error)
    return message ? { title: LOGIN_ERROR_TITLE, message } : null
  })

  return {
    kind: session.authKind,
    ssoUrl,
    accessError,
    identifier,
    password,
    rememberMe,
    fieldErrors,
    formError,
    pending: session.pending,
    touch,
    submit,
  }
}
