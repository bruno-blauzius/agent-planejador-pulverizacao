import { afterEach, describe, expect, it, vi } from 'vitest'
import { useGreeting } from '~/composables/useGreeting'
import { useSession } from '~/composables/useSession'
import { salutationFor } from '~/utils/format'
import { createTestApi, SEED_USER_EMAIL, SEED_USER_PASSWORD } from '../../support/mockApi'
import { installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

/** Local-time date at the given hour (salutation uses the viewer's clock). */
const at = (hour: number) => () => new Date(2026, 10, 21, hour, 30)

async function signIn() {
  await useSession().login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })
}

describe('salutationFor', () => {
  it.each([[0, 'Bom dia'], [11, 'Bom dia'], [12, 'Boa tarde'], [17, 'Boa tarde'], [18, 'Boa noite'], [23, 'Boa noite']])(
    '%sh → %s',
    (hour, expected) => {
      expect(salutationFor(at(hour)())).toBe(expected)
    },
  )
})

describe('useGreeting', () => {
  it('greets by the client hour once Nuxt is ready', async () => {
    installNuxtFakes(createTestApi(null).api)
    await signIn()

    expect(useGreeting({ now: at(9) }).greeting.value).toBe('Bom dia, Bruno')
    expect(useGreeting({ now: at(20) }).greeting.value).toBe('Boa noite, Bruno')
  })

  it('renders a neutral greeting on the server (no client hour yet)', async () => {
    installNuxtFakes(createTestApi(null).api)
    vi.stubGlobal('onNuxtReady', () => undefined)
    await signIn()

    expect(useGreeting({ now: at(9) }).greeting.value).toBe('Olá, Bruno')
  })

  it('omits the name without a session', () => {
    installNuxtFakes(createTestApi(null).api)
    expect(useGreeting({ now: at(14) }).greeting.value).toBe('Boa tarde')
  })
})
