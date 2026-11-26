import { ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useRainGauges } from '~/composables/useRainGauges'
import { useRainReadingForm } from '~/composables/useRainReadingForm'
import { useSoilInspectionForm } from '~/composables/useSoilInspectionForm'
import { useSoilProfileForm } from '~/composables/useSoilProfileForm'
import { ApiError } from '~/services/errors'
import { MOCK_SEDE_GAUGE_ID, MOCK_SOUTH_GAUGE_ID } from '~/services/mock/fieldConditionsMock'
import type { RainGaugeService, SoilConditionsService } from '~/services/ports'
import type { RainGauge, RainReading } from '~/types/trafficability'
import { flushPromises } from '../../support/nuxtFakes'
import { SEED_LOGINS, signInAs } from '../../support/session'

afterEach(() => {
  vi.unstubAllGlobals()
})

/** 07:00 in São Paulo; later than the seed clock, so "now" of the forms is under test control. */
const NOW = new Date('2026-11-26T10:00:00Z')
const FARM = 'farm-santa-rita'

const gauge = (id: string, name: string): RainGauge => ({ id, farmId: FARM, name, location: null, createdAt: NOW, updatedAt: NOW, version: 1 })

function readingService(answer: () => Promise<RainReading>) {
  const calls: { gaugeId: string, rainMm: number, observedAt: Date }[] = []
  const unused = () => Promise.reject(new Error('not used in this test'))
  const service: RainGaugeService = {
    list: unused,
    create: unused,
    update: unused,
    listReadings: unused,
    async recordReading(_farmId, gaugeId, input) {
      calls.push({ gaugeId, ...input })
      return answer()
    },
  }
  return { service, calls }
}

const savedReading: RainReading = { id: 'r1', rainGaugeId: 'g1', rainMm: 1, observedAt: NOW, recordedBy: { id: 'u', name: 'U' }, recordedAt: NOW }

describe('useRainGauges', () => {
  it('lists the farm gauges, registers and renames with registry.write, reloading the list', async () => {
    await signInAs(SEED_LOGINS.admin)
    const gauges = useRainGauges(FARM)
    await flushPromises()
    expect(gauges.canWrite.value).toBe(true)
    expect(gauges.gauges.value.map(item => item.id)).toEqual([MOCK_SOUTH_GAUGE_ID, MOCK_SEDE_GAUGE_ID])

    const created = await gauges.create({ name: 'Norte' })
    expect(created?.name).toBe('Norte')
    expect(gauges.gauges.value.map(item => item.name)).toContain('Norte')
    await gauges.rename(created!.id, 'Área Norte')
    expect(gauges.gauges.value.map(item => item.name)).toContain('Área Norte')

    expect(await gauges.create({ name: 'área norte' })).toBeNull()
    expect(gauges.createError.value?.details.map(item => item.field)).toEqual(['name'])
  })

  it('tells an operator it cannot write, and the API answers 403', async () => {
    await signInAs(SEED_LOGINS.operator)
    const gauges = useRainGauges(FARM)
    await flushPromises()
    expect(gauges.canWrite.value).toBe(false)
    expect(await gauges.create({ name: 'X' })).toBeNull()
    expect(gauges.createError.value?.code).toBe('forbidden')
  })

  it('calls nothing while disabled', async () => {
    await signInAs(SEED_LOGINS.admin)
    const enabled = ref(false)
    const gauges = useRainGauges(FARM, { enabled })
    await flushPromises()
    expect(gauges.gauges.value).toEqual([])
    enabled.value = true
    await flushPromises()
    expect(gauges.gauges.value).toHaveLength(2)
  })
})

describe('useRainReadingForm', () => {
  it('opens at now with the field gauge, or the only gauge, preselected', async () => {
    await signInAs(SEED_LOGINS.operator)
    const gauges = ref([gauge('g1', 'Sede'), gauge('g2', 'Sul')])
    const preferred = ref<string | null>('g2')
    const form = useRainReadingForm(FARM, { gauges, defaultGaugeId: preferred, now: () => NOW, service: readingService(async () => savedReading).service })
    form.reset()
    expect(form.values).toEqual({ gaugeId: 'g2', rainMm: '', date: '2026-11-26', time: '07:00' })
    expect(form.gaugeOptions.value).toEqual([{ value: 'g1', label: 'Sede' }, { value: 'g2', label: 'Sul' }])

    preferred.value = null
    form.reset()
    expect(form.values.gaugeId).toBeNull()
    gauges.value = [gauge('g1', 'Sede')]
    form.reset()
    expect(form.values.gaugeId).toBe('g1')
  })

  it('shows local errors only after trying, and sends nothing while invalid', async () => {
    await signInAs(SEED_LOGINS.operator)
    const fake = readingService(async () => savedReading)
    const form = useRainReadingForm(FARM, { gauges: [gauge('g1', 'Sede')], now: () => NOW, service: fake.service })
    form.reset()
    expect(form.errors.value).toEqual({})
    Object.assign(form.values, { rainMm: '600', time: '08:00' })
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({ rainMm: 'Use um valor de 0 a 500 mm.', time: 'O horário da leitura não pode estar no futuro.' })
    expect(fake.calls).toEqual([])
  })

  it('saves the local time as an instant and then reloads the trafficability', async () => {
    await signInAs(SEED_LOGINS.operator)
    const fake = readingService(async () => savedReading)
    const onSaved = vi.fn(async () => {})
    const form = useRainReadingForm(FARM, { gauges: [gauge('g1', 'Sede')], now: () => NOW, onSaved, service: fake.service })
    form.reset()
    Object.assign(form.values, { rainMm: '12,5', time: '06:30' })
    expect(await form.submit()).toBe(true)
    expect(fake.calls).toEqual([{ gaugeId: 'g1', rainMm: 12.5, observedAt: new Date('2026-11-26T09:30:00Z') }])
    expect(onSaved).toHaveBeenCalledWith(savedReading)
  })

  it('puts the 422 of the API on the field until the value changes; a 403 becomes the form error', async () => {
    await signInAs(SEED_LOGINS.operator)
    let failure = new ApiError({ code: 'validation_failed', message: 'dados inválidos', status: 422, details: [{ field: 'observed_at', message: 'já existe uma leitura neste horário' }] })
    const fake = readingService(async () => { throw failure })
    const form = useRainReadingForm(FARM, { gauges: [gauge('g1', 'Sede')], now: () => NOW, service: fake.service })
    form.reset()
    form.values.rainMm = '3'
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({ time: 'já existe uma leitura neste horário' })
    expect(form.formError.value).toBeNull()
    form.values.time = '06:00'
    expect(form.errors.value).toEqual({})

    failure = new ApiError({ code: 'forbidden', message: 'sem permissão', status: 403 })
    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toMatch(/Seu perfil não permite/)
  })

  it('records through the mock end to end', async () => {
    await signInAs(SEED_LOGINS.operator)
    const form = useRainReadingForm(FARM, { gauges: [gauge(MOCK_SEDE_GAUGE_ID, 'Pluviômetro da sede')], now: () => new Date('2026-11-21T12:00:00Z') })
    form.reset()
    form.values.rainMm = '2'
    expect(await form.submit()).toBe(true)
  })
})

describe('useSoilInspectionForm', () => {
  function inspectionService() {
    const calls: unknown[] = []
    const unused = () => Promise.reject(new Error('not used in this test'))
    const service: SoilConditionsService = {
      getProfile: unused,
      updateProfile: unused,
      async recordInspection(farmId, fieldId, input) {
        calls.push({ farmId, fieldId, input })
        return { id: 'i1', fieldId, farmId, state: input.state, inspectedAt: NOW, validUntil: NOW, inspectedBy: { id: 'u', name: 'U' }, note: null }
      },
    }
    return { service, calls }
  }

  it('opens at now, with the validity notice and the four states', async () => {
    await signInAs(SEED_LOGINS.operator)
    const form = useSoilInspectionForm(FARM, 'field-str-4', { now: () => NOW, service: inspectionService().service })
    form.reset()
    expect(form.values).toEqual({ state: null, date: '2026-11-26', time: '07:00', note: '' })
    expect(form.validityNotice).toMatch(/12 horas/)
    expect(form.stateOptions).toHaveLength(4)
    expect(form.noteMaxLength).toBe(500)
  })

  it('requires the state, then saves and reloads', async () => {
    await signInAs(SEED_LOGINS.operator)
    const fake = inspectionService()
    const onSaved = vi.fn()
    const form = useSoilInspectionForm(FARM, 'field-str-4', { now: () => NOW, onSaved, service: fake.service })
    form.reset()
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({ state: 'Escolha o estado do solo.' })
    form.values.state = 'moist'
    form.values.note = 'barro'
    expect(await form.submit()).toBe(true)
    expect(fake.calls).toEqual([{ farmId: FARM, fieldId: 'field-str-4', input: { state: 'moist', inspectedAt: NOW, note: 'barro' } }])
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('sends nothing without a field', async () => {
    await signInAs(SEED_LOGINS.operator)
    const fake = inspectionService()
    const form = useSoilInspectionForm(FARM, null, { now: () => NOW, service: fake.service })
    form.reset()
    form.values.state = 'dry'
    expect(await form.submit()).toBe(false)
    expect(fake.calls).toEqual([])
  })
})

describe('useSoilProfileForm', () => {
  it('loads the profile, fills the form on reset and saves through the mock', async () => {
    await signInAs(SEED_LOGINS.admin)
    const gauges = useRainGauges(FARM)
    const onSaved = vi.fn()
    const form = useSoilProfileForm(FARM, 'field-str-1', { gauges: gauges.gauges, onSaved })
    await flushPromises()
    expect(form.profile.value).toMatchObject({ fieldId: 'field-str-1', soilTexture: 'medium', rainGauge: null })
    form.reset()
    expect(form.values).toEqual({ soilTexture: 'medium', rainGaugeId: null })
    expect(form.gaugeOptions.value.at(-1)).toEqual({ value: null, label: 'Nenhum (usa o modelo de previsão)' })

    Object.assign(form.values, { soilTexture: 'sandy', rainGaugeId: MOCK_SOUTH_GAUGE_ID })
    expect(await form.submit()).toBe(true)
    expect(form.profile.value).toMatchObject({ soilTexture: 'sandy', rainGauge: { id: MOCK_SOUTH_GAUGE_ID } })
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('refuses a gauge not in the list before calling the API; the API 403s an operator', async () => {
    await signInAs(SEED_LOGINS.operator)
    const gauges = useRainGauges(FARM)
    const form = useSoilProfileForm(FARM, 'field-str-1', { gauges: gauges.gauges })
    await flushPromises()
    form.reset()
    form.values.rainGaugeId = 'gauge-unknown'
    expect(await form.submit()).toBe(false)
    expect(form.errors.value).toEqual({ rainGaugeId: 'Escolha um pluviômetro desta fazenda.' })

    form.values.rainGaugeId = null
    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toMatch(/Seu perfil não permite/)
  })

  it('calls nothing while disabled', async () => {
    await signInAs(SEED_LOGINS.admin)
    const form = useSoilProfileForm(FARM, 'field-str-1', { gauges: [], enabled: false })
    await flushPromises()
    expect(form.profile.value).toBeNull()
    expect(form.pending.value).toBe(false)
  })
})
