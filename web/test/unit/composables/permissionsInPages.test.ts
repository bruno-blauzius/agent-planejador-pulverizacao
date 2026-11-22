import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFarmDetailPage } from '~/composables/useFarmDetailPage'
import { useFarmListPage } from '~/composables/useFarmListPage'
import { useFieldForm } from '~/composables/useFieldForm'
import { useOwnerForm } from '~/composables/useOwnerForm'
import { ApiError } from '~/services/errors'
import type { GeoJsonPolygon } from '~/types/geo'
import { FORBIDDEN_MESSAGES, forbiddenMessageFor } from '~/utils/permissions'
import { flushPromises } from '../../support/nuxtFakes'
import { installSignedIn, TEST_USERS } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

const INSIDE_SANTA_RITA: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.712, -21.302], [-46.708, -21.302], [-46.708, -21.298], [-46.712, -21.298], [-46.712, -21.302]]],
}

describe('forbiddenMessageFor', () => {
  it('turns 403 codes into honest pt-BR messages', () => {
    expect(forbiddenMessageFor(new ApiError({ code: 'forbidden', message: 'x', status: 403 }))).toBe(FORBIDDEN_MESSAGES.forbidden)
    expect(forbiddenMessageFor(new ApiError({ code: 'out_of_scope', message: 'x', status: 403 }))).toBe(FORBIDDEN_MESSAGES.out_of_scope)
    expect(forbiddenMessageFor(new ApiError({ code: 'conflict', message: 'x', status: 409 }))).toBeNull()
    expect(forbiddenMessageFor(null)).toBeNull()
  })
})

describe('canWrite in page composables (ADR 0013)', () => {
  it.each([
    [TEST_USERS.superAdmin, true],
    [TEST_USERS.admin, true],
    [TEST_USERS.operator, false],
  ])('%s → canWrite %s on the farm list and the farm detail', async (identifier, expected) => {
    const fakes = await installSignedIn(identifier)
    fakes.route.path = '/farms'
    expect(useFarmListPage().canWrite.value).toBe(expected)
    expect(useFarmDetailPage('farm-santa-rita').canWrite.value).toBe(expected)
  })

  it('still lets the operator read the farms in scope', async () => {
    await installSignedIn(TEST_USERS.operator)
    const page = useFarmListPage()
    await flushPromises()
    expect(page.cards.value.map(card => card.id)).toEqual(['farm-santa-rita'])
  })
})

describe('403 on writes', () => {
  it('shows the honest message when the field form is refused', async () => {
    const fakes = await installSignedIn(TEST_USERS.operator)
    fakes.route.query = {}
    const form = useFieldForm({ farmId: 'farm-santa-rita' })
    await flushPromises()
    form.polygon.value = INSIDE_SANTA_RITA
    Object.assign(form.values, { name: 'Talhão 10' })
    form.selectCrop('coffee_arabica')

    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toBe(FORBIDDEN_MESSAGES.forbidden)
  })

  it('shows the honest message when the owner form is refused', async () => {
    await installSignedIn(TEST_USERS.operator)
    const form = useOwnerForm({ mode: 'create', today: () => new Date('2026-11-21T12:00:00Z') })
    form.name.value = 'Ana Souza'
    form.document.value = '111.444.777-35'
    form.phone.value = '35998120099'

    expect(await form.submit()).toBeNull()
    expect(form.formError.value).toBe(FORBIDDEN_MESSAGES.forbidden)
  })
})
