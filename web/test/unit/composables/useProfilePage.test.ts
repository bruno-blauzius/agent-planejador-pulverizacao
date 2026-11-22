import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { useProfilePage } from '~/composables/useProfilePage'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useSession } from '~/composables/useSession'
import { ApiError } from '~/services/errors'
import type { ApiServices } from '~/services/ports'
import { PROFILE_MESSAGES } from '~/utils/profile'
import { createTestApi, SEED_USER_PASSWORD, SEED_USER_TOKEN } from '../../support/mockApi'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const NOW = () => new Date('2026-11-21T12:00:00Z')

async function setup(overrides: Partial<ApiServices> = {}) {
  const { api } = createTestApi()
  const services = { ...api, ...overrides }
  const fakes = installNuxtFakes(services)
  fakes.cookie(AUTH_TOKEN_COOKIE).value = SEED_USER_TOKEN
  await useSession().fetchCurrentUser()
  const page = useProfilePage({ now: NOW })
  await flushPromises()
  return { page, api: services }
}

describe('useProfilePage', () => {
  it('loads the identity card and the form', async () => {
    const { page } = await setup()

    expect(page.identity.value).toEqual({
      name: 'Bruno Blauzius',
      initials: 'BB',
      email: 'bruno@agentesagro.test',
      roleLabel: 'Super admin',
      companyName: 'Agentes Agro',
      managedTitle: PROFILE_MESSAGES.managedTitle,
      managedText: PROFILE_MESSAGES.managedText,
    })
    expect(page.name.value).toBe('Bruno Blauzius')
    expect(page.email.value).toBe('bruno@agentesagro.test')
    expect(page.passwordChangedLabel.value).toBe('Última alteração: há 3 meses')
    expect(page.photo.isAvailable).toBe(false)
    expect(page.isDirty.value).toBe(false)
  })

  it('derives the form from the loaded profile without watchers (SSR)', async () => {
    const { api } = createTestApi()
    const fakes = installNuxtFakes(api)
    fakes.cookie(AUTH_TOKEN_COOKIE).value = SEED_USER_TOKEN
    // A paused scope runs no watcher callbacks, like the server render.
    const scope = effectScope()
    const page = scope.run(() => useProfilePage({ now: NOW }))
    scope.pause()
    await flushPromises()

    expect(page?.name.value).toBe('Bruno Blauzius')
    expect(page?.isDirty.value).toBe(false)
    scope.stop()
  })

  it('saves the name and refreshes the session user', async () => {
    const { page } = await setup()
    page.name.value = 'Bruno B. Silva'

    expect(await page.save()).toBe(true)

    expect(page.notice.value).toEqual({ kind: 'success', title: PROFILE_MESSAGES.profileSaved, description: null })
    expect(useSession().user.value?.name).toBe('Bruno B. Silva')
    expect(page.isDirty.value).toBe(false)
  })

  it('validates the password section only when something is typed', async () => {
    const { page } = await setup()
    page.newPassword.value = 'abc'
    page.confirmPassword.value = 'abd'

    expect(page.passwordRules.value.map(rule => rule.met)).toEqual([false, true, false])
    expect(await page.save()).toBe(false)
    expect(page.fieldErrors.value).toEqual({
      currentPassword: PROFILE_MESSAGES.currentPasswordRequired,
      newPassword: PROFILE_MESSAGES.newPasswordWeak,
      confirmPassword: PROFILE_MESSAGES.confirmMismatch,
    })

    page.discard()
    expect(page.fieldErrors.value).toEqual({})
    expect(page.isDirty.value).toBe(false)
  })

  it('changes the password and clears the fields', async () => {
    const { page } = await setup()
    page.currentPassword.value = SEED_USER_PASSWORD
    page.newPassword.value = 'novaSenha123'
    page.confirmPassword.value = 'novaSenha123'

    expect(await page.save()).toBe(true)

    expect(page.notice.value?.title).toBe(PROFILE_MESSAGES.passwordSaved)
    expect(page.newPassword.value).toBe('')
    expect(page.passwordChangedLabel.value).not.toBe('Última alteração: há 3 meses')
  })

  it('maps a wrong current password to its field, telling that the name was saved', async () => {
    const { page } = await setup()
    page.name.value = 'Bruno B.'
    page.currentPassword.value = 'errada'
    page.newPassword.value = 'novaSenha123'
    page.confirmPassword.value = 'novaSenha123'

    expect(await page.save()).toBe(false)

    expect(page.fieldErrors.value).toEqual({ currentPassword: 'senha atual incorreta' })
    expect(page.notice.value).toEqual({ kind: 'error', title: PROFILE_MESSAGES.nameSavedPasswordFailed, description: null })

    page.currentPassword.value = SEED_USER_PASSWORD
    await nextTick()
    expect(page.fieldErrors.value).toEqual({})
  })

  it('reports a failure without field details honestly', async () => {
    const base = createTestApi().api
    const { page } = await setup({
      profile: {
        ...base.profile,
        update: async () => {
          throw new ApiError({ code: 'network_error', message: 'Sem conexão.', status: 0 })
        },
      },
    })
    page.name.value = 'Outro nome'

    expect(await page.save()).toBe(false)
    expect(page.formError.value).toBe('Sem conexão.')
    expect(page.saving.value).toBe(false)
  })

  it('exposes the load error', async () => {
    const base = createTestApi().api
    const { page } = await setup({
      profile: {
        ...base.profile,
        get: async () => {
          throw new ApiError({ code: 'internal_error', message: 'Erro interno.', status: 500 })
        },
      },
    })
    expect(page.error.value?.code).toBe('internal_error')
    expect(page.identity.value).toBeNull()
  })
})
