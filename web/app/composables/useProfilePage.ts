import { computed, reactive, readonly, ref, shallowRef, watch } from 'vue'
import type { ApiError } from '~/services/errors'
import type { Profile } from '~/types/profile'
import {
  PROFILE_MESSAGES,
  passwordChangedLabel,
  passwordRules,
  profileErrorsFromApi,
  toProfileIdentityView,
  validateProfileForm,
  wantsPasswordChange,
  type ProfileField,
  type ProfileFieldErrors,
} from '~/utils/profile'
import { useAction, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useSession } from './useSession'

export interface UseProfilePageOptions {
  now?: () => Date
}

/** In-page toast after saving. */
export interface ProfileNotice {
  readonly kind: 'success' | 'error'
  readonly title: string
  readonly description: string | null
}

const PROFILE_FIELDS: readonly ProfileField[] = ['name', 'currentPassword', 'newPassword', 'confirmPassword']

/**
 * Profile screen (11): identity card, personal data (name only; e-mail, role
 * and company are managed by an administrator) and password change, saved by
 * one "Salvar alterações". Any role can open it.
 */
export function useProfilePage(options: UseProfilePageOptions = {}) {
  const { profile: profileService } = useApiServices()
  const session = useSession()
  const now = options.now ?? (() => new Date())

  const resource = useResource<Profile | null>('profile:me', () => profileService.get(), () => null)
  const updateAction = useAction((name: string) => profileService.update({ name }))
  const passwordAction = useAction((currentPassword: string, newPassword: string) =>
    profileService.changePassword({ currentPassword, newPassword }))

  /** What the user typed; null = untouched, so the field shows the loaded name. */
  const nameDraft = ref<string | null>(null)
  const currentPassword = ref('')
  const newPassword = ref('')
  const confirmPassword = ref('')
  const touched = reactive(new Set<ProfileField>())
  const apiErrors = shallowRef<ProfileFieldErrors>({})
  const notice = shallowRef<ProfileNotice | null>(null)
  const formError = ref<string | null>(null)
  const saving = ref(false)

  const loaded = computed(() => resource.data.value)

  // Derived, not filled by a watcher: watchers do not run during SSR, and the
  // server and client must both render the loaded name with nothing dirty.
  const name = computed<string>({
    get: () => nameDraft.value ?? loaded.value?.name ?? '',
    set: (value) => {
      nameDraft.value = value
    },
  })

  const passwordValues = computed(() => ({
    currentPassword: currentPassword.value,
    newPassword: newPassword.value,
    confirmPassword: confirmPassword.value,
  }))

  const localErrors = computed(() => validateProfileForm(name.value, passwordValues.value))

  const fieldErrors = computed<ProfileFieldErrors>(() => {
    const visible: ProfileFieldErrors = {}
    for (const field of PROFILE_FIELDS) {
      const message = localErrors.value[field]
      if (message && touched.has(field)) visible[field] = message
    }
    return { ...apiErrors.value, ...visible }
  })

  const nameChanged = computed(() => Boolean(loaded.value) && name.value.trim() !== loaded.value?.name)
  const isDirty = computed(() => nameChanged.value || wantsPasswordChange(passwordValues.value))

  // Editing hides the API errors and the general error of the last attempt.
  watch([name, currentPassword, newPassword, confirmPassword], () => {
    apiErrors.value = {}
    formError.value = null
  })

  function touch(field: ProfileField) {
    touched.add(field)
  }

  function clearPasswords() {
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    for (const field of ['currentPassword', 'newPassword', 'confirmPassword'] as const) touched.delete(field)
  }

  function discard() {
    nameDraft.value = null
    clearPasswords()
    touched.clear()
    apiErrors.value = {}
    formError.value = null
  }

  function reportFailure(error: ApiError | null): boolean {
    if (!error) return false
    const mapped = profileErrorsFromApi(error.details)
    apiErrors.value = { ...apiErrors.value, ...mapped }
    if (Object.keys(mapped).length === 0) formError.value = error.message || PROFILE_MESSAGES.saveFailed
    return true
  }

  async function saveName(): Promise<boolean> {
    const updated = await updateAction.execute(name.value.trim())
    if (!updated) return !reportFailure(updateAction.error.value)
    await resource.refresh()
    nameDraft.value = null
    // The sidebar and topbar read the session user: reload it with the new name.
    await session.fetchCurrentUser()
    return true
  }

  async function savePassword(): Promise<boolean> {
    const result = await passwordAction.execute(currentPassword.value, newPassword.value)
    if (result === null) {
      reportFailure(passwordAction.error.value)
      return false
    }
    clearPasswords()
    await resource.refresh()
    return true
  }

  function successNotice(savedName: boolean, savedPassword: boolean): ProfileNotice {
    const title = savedName && savedPassword
      ? PROFILE_MESSAGES.allSaved
      : savedName ? PROFILE_MESSAGES.profileSaved : PROFILE_MESSAGES.passwordSaved
    return { kind: 'success', title, description: null }
  }

  /** Saves the name (if changed) and then the password (if typed). */
  async function save(): Promise<boolean> {
    if (saving.value || !isDirty.value) return false
    PROFILE_FIELDS.forEach(touch)
    if (Object.keys(localErrors.value).length > 0) return false

    const changeName = nameChanged.value
    const changePassword = wantsPasswordChange(passwordValues.value)
    saving.value = true
    notice.value = null
    try {
      const nameOk = changeName ? await saveName() : true
      if (!nameOk) return false
      const passwordOk = changePassword ? await savePassword() : true
      if (!passwordOk) {
        if (changeName) {
          notice.value = { kind: 'error', title: PROFILE_MESSAGES.nameSavedPasswordFailed, description: null }
        }
        return false
      }
      touched.clear()
      notice.value = successNotice(changeName, changePassword)
      return true
    }
    finally {
      saving.value = false
    }
  }

  return {
    pending: resource.pending,
    error: resource.error,
    refresh: resource.refresh,

    identity: computed(() => (loaded.value ? toProfileIdentityView(loaded.value) : null)),
    photo: { isAvailable: false, hint: PROFILE_MESSAGES.photoUnavailable } as const,

    name,
    email: computed(() => loaded.value?.email ?? ''),

    currentPassword,
    newPassword,
    confirmPassword,
    passwordRules: computed(() => passwordRules(newPassword.value)),
    passwordHint: PROFILE_MESSAGES.passwordHint,
    passwordChangedLabel: computed(() => passwordChangedLabel(loaded.value?.passwordChangedAt ?? null, now())),

    fieldErrors,
    touch,

    isDirty,
    saving: readonly(saving),
    save,
    discard,
    notice: readonly(notice),
    dismissNotice: () => (notice.value = null),
    formError: computed(() => formError.value),
  }
}
