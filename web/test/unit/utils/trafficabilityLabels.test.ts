import { describe, expect, it } from 'vitest'
import {
  ELIGIBILITY_REASONS,
  ELIGIBILITY_VERDICTS,
  SOIL_STATES,
  TRAFFICABILITY_DATA_ISSUES,
} from '~/types/trafficability'
import {
  ELIGIBILITY_REASON_LABELS,
  ELIGIBILITY_VERDICT_LABELS,
  SOIL_STATE_LABELS,
  TRAFFICABILITY_DATA_ISSUE_LABELS,
  TRAFFICABILITY_DATA_ISSUE_TITLES,
  eligibilityReasonsLabel,
  measuredRainCaption,
  methodEligibilityLabel,
  rainSourceLabel,
  sprayMethodLabel,
} from '~/utils/trafficabilityLabels'

describe('trafficability labels', () => {
  it('names every soil state', () => {
    expect(SOIL_STATES.map(state => SOIL_STATE_LABELS[state])).toEqual(['Seco', 'Ideal', 'Úmido', 'Encharcado', 'Desconhecido'])
  })

  it('names every verdict', () => {
    expect(ELIGIBILITY_VERDICTS.map(verdict => ELIGIBILITY_VERDICT_LABELS[verdict])).toEqual(['Apto', 'Atenção', 'Inapto'])
  })

  it('translates every reason code', () => {
    expect(ELIGIBILITY_REASONS.every(reason => ELIGIBILITY_REASON_LABELS[reason].length > 0)).toBe(true)
    expect(ELIGIBILITY_REASON_LABELS).toMatchObject({
      soil_waterlogged: 'solo encharcado',
      slope_restricted: 'só na área permitida',
      slope_restricted_on_wet_soil: 'encosta com solo úmido',
      soil_unknown: 'sem dado de chuva',
      terrain_unknown: 'relevo não calculado',
      soil_moist: 'solo úmido',
      slope_unsuitable: 'declive acima do limite',
    })
  })

  it('explains every data issue', () => {
    expect(TRAFFICABILITY_DATA_ISSUES.every(issue => TRAFFICABILITY_DATA_ISSUE_LABELS[issue].length > 0)).toBe(true)
  })

  it('joins reasons and puts them after the verdict', () => {
    expect(eligibilityReasonsLabel([])).toBe('')
    expect(eligibilityReasonsLabel(['slope_restricted', 'soil_waterlogged'])).toBe('só na área permitida · solo encharcado')
    expect(methodEligibilityLabel({ verdict: 'apt', reasons: [] })).toBe('Apto')
    expect(methodEligibilityLabel({ verdict: 'unfit', reasons: ['soil_waterlogged'] })).toBe('Inapto · solo encharcado')
  })

  it('says where the rain number came from, warning that the model is not the field', () => {
    expect(rainSourceLabel('gauge')).toBe('pluviômetro')
    expect(rainSourceLabel('station')).toBe('estação')
    expect(rainSourceLabel('model')).toBe('modelo (não é o talhão)')
    expect(rainSourceLabel('gauge', 'Pluviômetro da sede')).toBe('Pluviômetro da sede (pluviômetro)')
    expect(rainSourceLabel('model', '  ')).toBe('modelo (não é o talhão)')
  })

  it('reuses the method names of the relief view', () => {
    expect(sprayMethodLabel('knapsack')).toBe('Costal')
    expect(sprayMethodLabel('drone')).toBe('Drone')
  })
})

describe('trafficability legends', () => {
  it('titles every data issue', () => {
    expect(TRAFFICABILITY_DATA_ISSUES.every(issue => TRAFFICABILITY_DATA_ISSUE_TITLES[issue].length > 0)).toBe(true)
    expect(TRAFFICABILITY_DATA_ISSUE_TITLES.weather_unavailable).toBe('Previsão indisponível')
  })

  it('captions the measured rain by source, with the reading hour in São Paulo', () => {
    // 10:05 UTC = 07:05 in São Paulo.
    const observedAt = new Date('2026-11-26T10:05:00Z')
    expect(measuredRainCaption({ source: 'gauge', sourceName: 'Pluviômetro da sede', observedAt })).toBe('Medida: Pluviômetro da sede, lançado 07:05')
    expect(measuredRainCaption({ source: 'station', sourceName: 'Estação Sede', observedAt })).toBe('Medida: Estação Sede, observado 07:05')
    expect(measuredRainCaption({ source: 'station', sourceName: null, observedAt })).toBe('Medida: estação, observado 07:05')
    expect(measuredRainCaption({ source: 'model', sourceName: null, observedAt })).toBe('Estimativa do modelo (não é o talhão)')
  })
})
