import { ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFarmFields } from '~/composables/useFarmFields'
import { useFarmTerrainTab } from '~/composables/useFarmTerrainTab'
import { useRainGauges } from '~/composables/useRainGauges'
import { useRainReadingForm } from '~/composables/useRainReadingForm'
import { SOIL_INSPECTION_NO_FIELD_MESSAGE, useSoilInspectionForm } from '~/composables/useSoilInspectionForm'
import { SOIL_PROFILE_NOT_READY_MESSAGE, useSoilProfileForm } from '~/composables/useSoilProfileForm'
import { ApiError } from '~/services/errors'
import { MOCK_SEDE_GAUGE_ID, MOCK_SOUTH_GAUGE_ID } from '~/services/mock/fieldConditionsMock'
import type { RainGaugeService, SoilConditionsService } from '~/services/ports'
import type { RainGauge, SoilProfile, SoilProfileInput } from '~/types/trafficability'
import { flushPromises } from '../../support/nuxtFakes'
import { SEED_LOGINS, SEED_NOW, signInAs } from '../../support/session'

afterEach(() => {
  vi.unstubAllGlobals()
})

const FARM = 'farm-santa-rita'
/** 07:00 in São Paulo. */
const NOW = new Date('2026-11-26T10:00:00Z')
const gauge = (id: string, name: string): RainGauge => ({ id, farmId: FARM, name, location: null, createdAt: NOW, updatedAt: NOW, version: 1 })
const unused = () => Promise.reject(new Error('not used in this test'))

/** Soil service whose GET answers only when the test releases it. */
function heldProfileService(profile: SoilProfile) {
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => { release = resolve })
  const puts: SoilProfileInput[] = []
  const service: SoilConditionsService = {
    recordInspection: unused,
    async getProfile() {
      await gate
      return profile
    },
    async updateProfile(_farmId, _fieldId, input) {
      puts.push(input)
      return { ...profile, soilTexture: input.soilTexture, rainGauge: input.rainGaugeId ? { id: input.rainGaugeId, name: 'x' } : null }
    },
  }
  return { service, puts, release: () => release() }
}

const CLAY_PROFILE: SoilProfile = { fieldId: 'field-4', farmId: FARM, soilTexture: 'clay', rainGauge: { id: 'g1', name: 'Sede' } }

describe('useSoilProfileForm: opened before the profile arrives (review item 1)', () => {
  it('takes the profile when it arrives, is not ready before, and never PUTs empty values', async () => {
    await signInAs(SEED_LOGINS.admin)
    const held = heldProfileService(CLAY_PROFILE)
    const form = useSoilProfileForm(FARM, 'field-4', { gauges: [gauge('g1', 'Sede')], service: held.service })
    form.open()
    expect(form.ready.value).toBe(false)
    expect(form.values).toEqual({ soilTexture: null, rainGaugeId: null })

    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toBe(SOIL_PROFILE_NOT_READY_MESSAGE)
    expect(held.puts).toEqual([])

    held.release()
    await flushPromises()
    expect(form.ready.value).toBe(true)
    expect(form.values).toEqual({ soilTexture: 'clay', rainGaugeId: 'g1' })
    expect(form.formError.value).toBeNull()
  })

  it('does not overwrite what the user already typed when the profile arrives', async () => {
    await signInAs(SEED_LOGINS.admin)
    const held = heldProfileService(CLAY_PROFILE)
    const form = useSoilProfileForm(FARM, 'field-4', { gauges: [gauge('g1', 'Sede')], service: held.service })
    form.open()
    form.values.soilTexture = 'sandy'
    held.release()
    await flushPromises()
    expect(form.values.soilTexture).toBe('sandy')
  })
})

describe('useSoilProfileForm: open vs. coming back from the gauge manager (review item 4)', () => {
  it('keeps the edits when the container does not call open again, and preselects a new gauge', async () => {
    await signInAs(SEED_LOGINS.admin)
    const gauges = useRainGauges(FARM)
    const form = useSoilProfileForm(FARM, 'field-str-1', { gauges: gauges.gauges, gaugesPending: gauges.pending, gaugesError: gauges.error })
    await flushPromises()
    form.open()
    form.values.soilTexture = 'sandy'

    // Goes to the manager, registers a gauge and comes back (no `open`).
    const created = await gauges.create({ name: 'Norte' })
    form.selectGauge(created!.id)
    expect(form.values).toEqual({ soilTexture: 'sandy', rainGaugeId: created!.id })

    expect(await form.submit()).toBe(true)
    expect(form.profile.value).toMatchObject({ soilTexture: 'sandy', rainGauge: { id: created!.id, name: 'Norte' } })

    // A new opening starts from the saved profile.
    form.values.soilTexture = 'clay'
    form.open()
    expect(form.values.soilTexture).toBe('sandy')
  })
})

describe('useSoilProfileForm: gauge validation (review item 5)', () => {
  it('only checks the gauge once the list loaded, and always accepts the profile gauge', async () => {
    await signInAs(SEED_LOGINS.admin)
    const held = heldProfileService({ ...CLAY_PROFILE, rainGauge: { id: 'g-old', name: 'Antigo' } })
    const pending = ref(true)
    const form = useSoilProfileForm(FARM, 'field-4', { gauges: [], gaugesPending: pending, service: held.service })
    held.release()
    await flushPromises()
    expect(form.gaugesPending.value).toBe(true)
    // The profile gauge shows in the select even without the list.
    expect(form.gaugeOptions.value.map(option => option.value)).toEqual(['g-old', null])

    form.values.rainGaugeId = 'g-other'
    expect(await form.submit()).toBe(true)

    pending.value = false
    form.values.rainGaugeId = 'g-old'
    expect(await form.submit()).toBe(true)
    form.values.rainGaugeId = 'g-other'
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({ rainGaugeId: 'Escolha um pluviômetro desta fazenda.' })
  })
})

describe('useRainReadingForm: default gauge arriving late (review item 1)', () => {
  it('follows the field gauge until the user picks one', async () => {
    await signInAs(SEED_LOGINS.operator)
    const gauges = ref<readonly RainGauge[]>([])
    const preferred = ref<string | null>(null)
    const service: RainGaugeService = { list: unused, create: unused, update: unused, listReadings: unused, recordReading: unused }
    const form = useRainReadingForm(FARM, { gauges, defaultGaugeId: preferred, now: () => NOW, service })
    form.reset()
    expect(form.values.gaugeId).toBeNull()

    gauges.value = [gauge('g1', 'Sede'), gauge('g2', 'Sul')]
    preferred.value = 'g2'
    await flushPromises()
    expect(form.values.gaugeId).toBe('g2')

    form.values.gaugeId = 'g1'
    preferred.value = null
    await flushPromises()
    expect(form.values.gaugeId).toBe('g1')

    form.selectGauge('g2')
    expect(form.values.gaugeId).toBe('g2')
  })
})

describe('dates and times of the forms (review item 2)', () => {
  it('accepts a one-digit hour, normalizes it and exposes today as maxDate', async () => {
    await signInAs(SEED_LOGINS.operator)
    const calls: Date[] = []
    const service: RainGaugeService = {
      list: unused,
      create: unused,
      update: unused,
      listReadings: unused,
      async recordReading(_farmId, gaugeId, input) {
        calls.push(input.observedAt)
        return { id: 'r', rainGaugeId: gaugeId, rainMm: input.rainMm, observedAt: input.observedAt, recordedBy: { id: 'u', name: 'U' }, recordedAt: NOW }
      },
    }
    const form = useRainReadingForm(FARM, { gauges: [gauge('g1', 'Sede')], now: () => NOW, service })
    form.reset()
    expect(form.maxDate.value).toBe('2026-11-26')
    Object.assign(form.values, { rainMm: '1', time: '6:30' })
    expect(await form.submit()).toBe(true)
    expect(form.values.time).toBe('06:30')
    expect(calls).toEqual([new Date('2026-11-26T09:30:00Z')])
  })

  it('puts a future date on the date field', async () => {
    await signInAs(SEED_LOGINS.operator)
    const form = useSoilInspectionForm(FARM, 'field-4', { now: () => NOW })
    form.reset()
    expect(form.maxDate.value).toBe('2026-11-26')
    Object.assign(form.values, { state: 'dry', date: '2026-11-27', time: '06:00' })
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({ date: 'A data não pode estar no futuro.' })
  })
})

describe('useSoilInspectionForm without a field (review item 6)', () => {
  it('says why it was not sent', async () => {
    await signInAs(SEED_LOGINS.operator)
    const form = useSoilInspectionForm(FARM, null, { now: () => NOW })
    form.reset()
    form.values.state = 'dry'
    expect(form.formError.value).toBeNull()
    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toBe(SOIL_INSPECTION_NO_FIELD_MESSAGE)
  })
})

describe('useRainGauges errors (review item 3)', () => {
  it('puts the 422 on `name`, and clears it on demand', async () => {
    await signInAs(SEED_LOGINS.admin)
    const gauges = useRainGauges(FARM)
    await flushPromises()
    expect(await gauges.create({ name: 'pluviômetro da sede' })).toBeNull()
    expect(gauges.createErrors.value).toEqual({ name: 'já existe um pluviômetro com este nome na fazenda' })
    expect(gauges.createFormError.value).toBeNull()
    gauges.clearCreateError()
    expect(gauges.createErrors.value).toEqual({})
  })

  it('explains a 403 and a 409 in plain words', async () => {
    await signInAs(SEED_LOGINS.operator)
    const gauges = useRainGauges(FARM)
    await flushPromises()
    expect(await gauges.create({ name: 'X' })).toBeNull()
    expect(gauges.createFormError.value).toMatch(/Seu perfil não permite/)

    const conflicting: RainGaugeService = {
      list: async () => [gauge('g1', 'Sede')],
      create: unused,
      update: async () => { throw new ApiError({ code: 'conflict', message: 'versão', status: 409 }) },
      listReadings: unused,
      recordReading: unused,
    }
    const other = useRainGauges(FARM, { service: conflicting })
    expect(await other.rename('g1', 'Nova')).toBeNull()
    expect(other.renameFormError.value).toBe('Outra pessoa alterou este registro. Recarregue e tente de novo.')
    other.clearRenameError()
    expect(other.renameFormError.value).toBeNull()
  })

  it('after a rename, the tab reloads the soil profile and the trafficability', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { tab: 'terrain', terrainField: 'field-str-4', terrainView: 'detail', terrainDetailView: 'rain' }
    const tab = useFarmTerrainTab({ farmId: FARM, active: true, fields: useFarmFields(FARM), now: () => SEED_NOW })
    await flushPromises()
    expect(tab.soilProfileForm.profile.value?.rainGauge?.name).toBe('Pluviômetro da sede')

    expect(await tab.rainGauges.rename(MOCK_SEDE_GAUGE_ID, 'Sede nova')).not.toBeNull()
    expect(tab.soilProfileForm.profile.value?.rainGauge?.name).toBe('Sede nova')
    expect(tab.trafficability.value?.soil.rain?.sourceName).toBe('Sede nova')
    expect(tab.rainGauges.gauges.value.map(item => item.id)).toContain(MOCK_SOUTH_GAUGE_ID)
  })
})
