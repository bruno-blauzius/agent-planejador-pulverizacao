import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import type { RainGauge } from '~/types/trafficability'
import {
  emptyRainReadingValues,
  emptySoilInspectionValues,
  fromLocalDateTime,
  normalizeTimeInput,
  rainGaugeErrorsFromApi,
  submissionErrorMessage,
  todayInFarmZone,
  rainReadingErrorsFromApi,
  SOIL_INSPECTION_STATE_OPTIONS,
  SOIL_TEXTURE_OPTIONS,
  soilInspectionErrorsFromApi,
  soilProfileErrorsFromApi,
  soilProfileGaugeOptions,
  toLocalDateTime,
  toRainReadingInput,
  toSoilInspectionInput,
  validateRainGaugeName,
  validateRainReading,
  validateSoilInspection,
  validateSoilProfile,
} from '~/utils/fieldConditionsForm'

/** 07:00 in São Paulo. */
const NOW = new Date('2026-11-26T10:00:00Z')

const gauge = (id: string, name: string): RainGauge => ({
  id, farmId: 'farm-1', name, location: null, createdAt: NOW, updatedAt: NOW, version: 1,
})

describe('local date and time of the farm', () => {
  it('formats an instant in São Paulo and reads it back', () => {
    expect(toLocalDateTime(NOW)).toEqual({ date: '2026-11-26', time: '07:00' })
    expect(toLocalDateTime(new Date('2026-11-27T02:30:00Z'))).toEqual({ date: '2026-11-26', time: '23:30' })
    expect(fromLocalDateTime('2026-11-26', '07:00')).toEqual(NOW)
    expect(fromLocalDateTime('2026-11-26', '23:30')).toEqual(new Date('2026-11-27T02:30:00Z'))
  })

  it('refuses impossible dates and malformed times', () => {
    expect(fromLocalDateTime('2026-02-30', '07:00')).toBeNull()
    expect(fromLocalDateTime('26/11/2026', '07:00')).toBeNull()
    expect(fromLocalDateTime('2026-11-26', '24:00')).toBeNull()
    expect(fromLocalDateTime('2026-11-26', '7h')).toBeNull()
  })
})

describe('rain reading form rules', () => {
  const valid = { gaugeId: 'gauge-1', rainMm: '12,5', date: '2026-11-26', time: '06:30' }

  it('starts at now with the given gauge', () => {
    expect(emptyRainReadingValues(NOW, 'gauge-1')).toEqual({ gaugeId: 'gauge-1', rainMm: '', date: '2026-11-26', time: '07:00' })
  })

  it('accepts 0–500 mm with comma or dot and builds the instant', () => {
    expect(validateRainReading(valid, NOW)).toEqual({})
    expect(toRainReadingInput(valid, NOW)).toEqual({ gaugeId: 'gauge-1', input: { rainMm: 12.5, observedAt: new Date('2026-11-26T09:30:00Z') } })
    expect(toRainReadingInput({ ...valid, rainMm: '0' }, NOW)?.input.rainMm).toBe(0)
    expect(toRainReadingInput({ ...valid, rainMm: '500' }, NOW)?.input.rainMm).toBe(500)
  })

  it('reports each problem on its field', () => {
    expect(validateRainReading({ ...valid, gaugeId: null }, NOW)).toEqual({ gaugeId: 'Escolha o pluviômetro.' })
    expect(validateRainReading({ ...valid, rainMm: '' }, NOW).rainMm).toBe('Informe a chuva em mm.')
    expect(validateRainReading({ ...valid, rainMm: '-1' }, NOW).rainMm).toBe('Use um número, como 12,5.')
    expect(validateRainReading({ ...valid, rainMm: '500,1' }, NOW).rainMm).toBe('Use um valor de 0 a 500 mm.')
    expect(validateRainReading({ ...valid, time: '07:01' }, NOW)).toEqual({ time: 'O horário da leitura não pode estar no futuro.' })
    expect(validateRainReading({ ...valid, date: '' }, NOW)).toEqual({ date: 'Informe a data.' })
    expect(validateRainReading({ ...valid, time: '7h' }, NOW)).toEqual({ time: 'Use o formato 07:30.' })
    expect(validateRainReading({ ...valid, date: '2026-02-30' }, NOW)).toEqual({ date: 'Use uma data válida.' })
    expect(toRainReadingInput({ ...valid, rainMm: 'x' }, NOW)).toBeNull()
  })

  it('maps the 422 of the API to the form fields', () => {
    expect(rainReadingErrorsFromApi([
      { field: 'body.rain_mm', message: 'fora da faixa' },
      { field: 'observed_at', message: 'já existe leitura' },
      { field: 'other', message: 'x' },
    ])).toEqual({ rainMm: 'fora da faixa', time: 'já existe leitura' })
  })
})

describe('soil inspection form rules', () => {
  it('offers only the four known states and starts at now', () => {
    expect(SOIL_INSPECTION_STATE_OPTIONS.map(option => option.label)).toEqual(['Seco', 'Ideal', 'Úmido', 'Encharcado'])
    expect(emptySoilInspectionValues(NOW)).toEqual({ state: null, date: '2026-11-26', time: '07:00', note: '' })
  })

  it('requires a state, keeps the note within 500 and refuses the future', () => {
    const values = { state: 'moist' as const, date: '2026-11-26', time: '07:00', note: '  barro  ' }
    expect(toSoilInspectionInput(values, NOW)).toEqual({ state: 'moist', inspectedAt: NOW, note: 'barro' })
    expect(toSoilInspectionInput({ ...values, note: ' ' }, NOW)?.note).toBeNull()
    expect(validateSoilInspection({ ...values, state: null }, NOW)).toEqual({ state: 'Escolha o estado do solo.' })
    expect(validateSoilInspection({ ...values, note: 'x'.repeat(501) }, NOW)).toEqual({ note: 'Use no máximo 500 caracteres.' })
    expect(validateSoilInspection({ ...values, time: '08:00' }, NOW)).toEqual({ time: 'O horário da vistoria não pode estar no futuro.' })
    expect(soilInspectionErrorsFromApi([{ field: 'inspected_at', message: 'futuro' }, { field: 'state', message: 'desconhecido' }]))
      .toEqual({ time: 'futuro', state: 'desconhecido' })
  })
})

describe('soil profile form rules', () => {
  const gauges = [gauge('gauge-1', 'Sede'), gauge('gauge-2', 'Sul')]

  it('offers the textures and the gauges with a "none" option, both nullable', () => {
    expect(SOIL_TEXTURE_OPTIONS).toEqual([
      { value: 'sandy', label: 'Arenoso' },
      { value: 'medium', label: 'Médio' },
      { value: 'clay', label: 'Argiloso' },
      { value: null, label: 'Não informada' },
    ])
    expect(soilProfileGaugeOptions(gauges).map(option => option.value)).toEqual(['gauge-1', 'gauge-2', null])
  })

  it('accepts only a gauge of the farm', () => {
    expect(validateSoilProfile({ soilTexture: null, rainGaugeId: null }, gauges)).toEqual({})
    expect(validateSoilProfile({ soilTexture: 'clay', rainGaugeId: 'gauge-9' }, gauges)).toEqual({ rainGaugeId: 'Escolha um pluviômetro desta fazenda.' })
    expect(soilProfileErrorsFromApi([{ field: 'rain_gauge_id', message: 'de outra fazenda' }])).toEqual({ rainGaugeId: 'de outra fazenda' })
  })
})

describe('rain gauge name', () => {
  const gauges = [gauge('gauge-1', 'Pluviômetro da sede')]

  it('requires 1–100 characters not repeated in the farm', () => {
    expect(validateRainGaugeName('Norte', gauges)).toBeNull()
    expect(validateRainGaugeName('  ', gauges)).toBe('Informe o nome do pluviômetro.')
    expect(validateRainGaugeName('x'.repeat(101), gauges)).toBe('Use no máximo 100 caracteres.')
    expect(validateRainGaugeName('PLUVIÔMETRO DA SEDE', gauges)).toBe('Já existe um pluviômetro com este nome nesta fazenda.')
    // Renaming a gauge to its own name is fine.
    expect(validateRainGaugeName('Pluviômetro da sede', gauges, 'gauge-1')).toBeNull()
  })
})

describe('review of the forms: times, future dates and messages', () => {
  it('normalizes a one-digit hour and reads it', () => {
    expect(normalizeTimeInput(' 7:30 ')).toBe('07:30')
    expect(normalizeTimeInput('07:30')).toBe('07:30')
    expect(normalizeTimeInput('7h')).toBe('7h')
    expect(fromLocalDateTime('2026-11-26', '7:00')).toEqual(NOW)
    expect(validateRainReading({ gaugeId: 'g', rainMm: '1', date: '2026-11-26', time: '6:30' }, NOW)).toEqual({})
  })

  it('reports a date after today on the date field, and gives today for `max`', () => {
    expect(todayInFarmZone(NOW)).toBe('2026-11-26')
    // 23:30 in São Paulo is already the next day in UTC.
    expect(todayInFarmZone(new Date('2026-11-27T02:30:00Z'))).toBe('2026-11-26')
    expect(validateSoilInspection({ state: 'dry', date: '2026-11-27', time: '00:10', note: '' }, NOW)).toEqual({ date: 'A data não pode estar no futuro.' })
    expect(validateSoilInspection({ state: 'dry', date: '2026-11-26', time: '07:10', note: '' }, NOW)).toEqual({ time: 'O horário da vistoria não pode estar no futuro.' })
  })

  it('words a 403 and a 409, and leaves a 422 to its fields', () => {
    const error = (code: 'forbidden' | 'conflict' | 'validation_failed' | 'internal_error', message = 'falhou') => new ApiError({ code, message, status: 400 })
    expect(submissionErrorMessage(error('forbidden'), false)).toMatch(/Seu perfil não permite/)
    expect(submissionErrorMessage(error('conflict'), false)).toBe('Outra pessoa alterou este registro. Recarregue e tente de novo.')
    expect(submissionErrorMessage(error('validation_failed'), true)).toBeNull()
    expect(submissionErrorMessage(error('internal_error', 'erro interno'), false)).toBe('erro interno')
    expect(rainGaugeErrorsFromApi([{ field: 'body.name', message: 'repetido' }])).toEqual({ name: 'repetido' })
  })

  it('validates the gauge of the profile only with the list loaded, always accepting the current one', () => {
    const values = { soilTexture: null, rainGaugeId: 'g-x' }
    expect(validateSoilProfile(values, [], { gaugesLoaded: false, profileGaugeId: null })).toEqual({})
    expect(validateSoilProfile(values, [], { gaugesLoaded: true, profileGaugeId: 'g-x' })).toEqual({})
    expect(validateSoilProfile(values, [], { gaugesLoaded: true, profileGaugeId: null })).toEqual({ rainGaugeId: 'Escolha um pluviômetro desta fazenda.' })
    expect(soilProfileGaugeOptions([gauge('gauge-1', 'Sede')], { id: 'gauge-1', name: 'Sede' }).map(option => option.value)).toEqual(['gauge-1', null])
  })
})
