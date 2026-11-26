/**
 * Trafficability fixtures of the mock world (ADR 0022), coherent with `seed.ts`
 * and with the relief of `terrainSeed.ts`. These are hand-written answers, not
 * a calculation: the soil state, the verdicts and the releases are what the
 * API's domain (`EvaluateMethodEligibility`, `PlanTrafficability`) returns for
 * these inputs, copied here so the screen has plausible data.
 *
 * - `field-str-4` (Santa Rita, Talhão 4): clay, waterlogged after 24 mm in 48 h
 *   (example `waterloggedClay` of the contract). Relief of `terrainSeed.ts`:
 *   knapsack and aerial compatible, tractor restricted (62 %), airblast
 *   restricted (52 %), self-propelled unsuitable (82 %).
 * - `field-fov-3` (Ouro Verde, Talhão 3): no rain source at all; also the
 *   fallback of every field without a fixture of its own.
 *
 * Days are offsets from today (0 = today) in `America/Sao_Paulo`.
 */
import type { SprayMethod } from '~/types/terrain'
import type {
  MethodEligibility,
  ModelSoilMoisture,
  RainSource,
  SoilBasis,
  SoilState,
  SoilTexture,
  ThresholdsSource,
  TrafficabilityDataIssue,
} from '~/types/trafficability'

/** Longest horizon the contract accepts (`days` maximum). */
export const MOCK_TRAFFICABILITY_MAX_DAYS = 7

export interface MockRainSeed {
  readonly source: RainSource
  readonly sourceName: string | null
  /** Last 7 days, oldest first, today last (measured). */
  readonly measuredMm: readonly number[]
  /** Next `MOCK_TRAFFICABILITY_MAX_DAYS` days, tomorrow first (forecast, always the model). */
  readonly forecastMm: readonly number[]
  /** Accumulations as the API reports them (24 h, 48 h, 72 h, 7 d). */
  readonly last24hMm: number
  readonly last48hMm: number
  readonly last72hMm: number
  readonly last7dMm: number
}

export interface MockReleaseSeed {
  readonly method: SprayMethod
  readonly requiredState: SoilState
  /** Day offset of the release; null when it does not happen within the longest horizon. */
  readonly dayOffset: number | null
}

export interface MockTrafficabilityFixture {
  readonly state: SoilState
  readonly basis: SoilBasis
  readonly texture: SoilTexture | null
  readonly thresholdsSource: ThresholdsSource
  /** Null without any rain source (no measured nor forecast rain). */
  readonly rain: MockRainSeed | null
  readonly modelMoisture: ModelSoilMoisture | null
  /** State of today and of each projected day: `MOCK_TRAFFICABILITY_MAX_DAYS + 1` entries. */
  readonly dayStates: readonly SoilState[]
  /**
   * Verdict of every method for each state of `dayStates`, and for every state a
   * person may record in an inspection (the mock shows today's verdicts for it).
   */
  readonly methodsByState: Readonly<Partial<Record<SoilState, readonly MethodEligibility[]>>>
  readonly releases: readonly MockReleaseSeed[]
  readonly dataIssues: readonly TrafficabilityDataIssue[]
}

const apt = (method: SprayMethod): MethodEligibility => ({ method, verdict: 'apt', reasons: [], areaAboveLimitPct: null })

/** Dry or ideal soil frees all ground traffic: on Talhão 4 only the relief limits. */
const TALHAO_4_DRY_OR_IDEAL: readonly MethodEligibility[] = [
  apt('knapsack'),
  { method: 'tractor_boom', verdict: 'attention', reasons: ['slope_restricted'], areaAboveLimitPct: 62 },
  { method: 'self_propelled', verdict: 'unfit', reasons: ['slope_unsuitable'], areaAboveLimitPct: 82 },
  { method: 'airblast', verdict: 'attention', reasons: ['slope_restricted'], areaAboveLimitPct: 52 },
  apt('aircraft'),
  apt('drone'),
]

/** Compatible relief (the fallback): with dry or ideal soil every method is apt. */
const ALL_APT: readonly MethodEligibility[] = (['knapsack', 'tractor_boom', 'self_propelled', 'airblast', 'aircraft', 'drone'] as const).map(apt)

/** Talhão 4: clay, 24 mm in 48 h, 2 mm tomorrow and 28 mm on the fifth day. */
export const WATERLOGGED_CLAY_FIXTURE: MockTrafficabilityFixture = {
  state: 'waterlogged',
  basis: 'measured_rain',
  texture: 'clay',
  thresholdsSource: 'heuristic',
  rain: {
    source: 'gauge',
    sourceName: 'Pluviômetro da sede',
    measuredMm: [0, 10, 0, 6, 31, 18, 6],
    forecastMm: [2, 0, 0, 0, 28, 0, 0],
    last24hMm: 6,
    last48hMm: 24,
    last72hMm: 55,
    last7dMm: 71,
  },
  modelMoisture: { volumetricM3m3: 0.41, fieldCapacityM3m3: 0.36, fieldCapacityPct: 113.9 },
  dayStates: ['waterlogged', 'waterlogged', 'moist', 'ideal', 'ideal', 'waterlogged', 'waterlogged', 'waterlogged'],
  methodsByState: {
    waterlogged: [
      { method: 'knapsack', verdict: 'attention', reasons: ['soil_waterlogged'], areaAboveLimitPct: null },
      { method: 'tractor_boom', verdict: 'unfit', reasons: ['slope_restricted', 'soil_waterlogged'], areaAboveLimitPct: 62 },
      { method: 'self_propelled', verdict: 'unfit', reasons: ['slope_unsuitable', 'soil_waterlogged'], areaAboveLimitPct: 82 },
      { method: 'airblast', verdict: 'unfit', reasons: ['slope_restricted', 'soil_waterlogged'], areaAboveLimitPct: 52 },
      apt('aircraft'),
      apt('drone'),
    ],
    moist: [
      apt('knapsack'),
      { method: 'tractor_boom', verdict: 'unfit', reasons: ['slope_restricted', 'soil_moist', 'slope_restricted_on_wet_soil'], areaAboveLimitPct: 62 },
      { method: 'self_propelled', verdict: 'unfit', reasons: ['slope_unsuitable', 'soil_moist'], areaAboveLimitPct: 82 },
      { method: 'airblast', verdict: 'unfit', reasons: ['slope_restricted', 'soil_moist', 'slope_restricted_on_wet_soil'], areaAboveLimitPct: 52 },
      apt('aircraft'),
      apt('drone'),
    ],
    ideal: TALHAO_4_DRY_OR_IDEAL,
    dry: TALHAO_4_DRY_OR_IDEAL,
  },
  releases: [
    { method: 'knapsack', requiredState: 'moist', dayOffset: 2 },
    { method: 'tractor_boom', requiredState: 'ideal', dayOffset: 3 },
    { method: 'self_propelled', requiredState: 'ideal', dayOffset: 3 },
    { method: 'airblast', requiredState: 'ideal', dayOffset: 3 },
  ],
  dataIssues: [],
}

/**
 * No gauge, station nor model for the field: the state is `unknown`, which
 * alerts but never blocks, so every ground method is in attention and nothing
 * is released. Relief taken as compatible (the generic fallback knows none).
 */
export const NO_RAIN_SOURCE_FIXTURE: MockTrafficabilityFixture = {
  state: 'unknown',
  basis: 'none',
  texture: 'medium',
  thresholdsSource: 'heuristic',
  rain: null,
  modelMoisture: null,
  dayStates: Array.from({ length: MOCK_TRAFFICABILITY_MAX_DAYS + 1 }, (): SoilState => 'unknown'),
  methodsByState: {
    unknown: [
      { method: 'knapsack', verdict: 'attention', reasons: ['soil_unknown'], areaAboveLimitPct: null },
      { method: 'tractor_boom', verdict: 'attention', reasons: ['soil_unknown'], areaAboveLimitPct: null },
      { method: 'self_propelled', verdict: 'attention', reasons: ['soil_unknown'], areaAboveLimitPct: null },
      { method: 'airblast', verdict: 'attention', reasons: ['soil_unknown'], areaAboveLimitPct: null },
      apt('aircraft'),
      apt('drone'),
    ],
    dry: ALL_APT,
    ideal: ALL_APT,
    moist: [
      apt('knapsack'),
      { method: 'tractor_boom', verdict: 'attention', reasons: ['soil_moist'], areaAboveLimitPct: null },
      { method: 'self_propelled', verdict: 'attention', reasons: ['soil_moist'], areaAboveLimitPct: null },
      { method: 'airblast', verdict: 'attention', reasons: ['soil_moist'], areaAboveLimitPct: null },
      apt('aircraft'),
      apt('drone'),
    ],
    waterlogged: [
      { method: 'knapsack', verdict: 'attention', reasons: ['soil_waterlogged'], areaAboveLimitPct: null },
      { method: 'tractor_boom', verdict: 'unfit', reasons: ['soil_waterlogged'], areaAboveLimitPct: null },
      { method: 'self_propelled', verdict: 'unfit', reasons: ['soil_waterlogged'], areaAboveLimitPct: null },
      { method: 'airblast', verdict: 'unfit', reasons: ['soil_waterlogged'], areaAboveLimitPct: null },
      apt('aircraft'),
      apt('drone'),
    ],
  },
  releases: [
    { method: 'knapsack', requiredState: 'moist', dayOffset: null },
    { method: 'tractor_boom', requiredState: 'ideal', dayOffset: null },
    { method: 'self_propelled', requiredState: 'ideal', dayOffset: null },
    { method: 'airblast', requiredState: 'ideal', dayOffset: null },
  ],
  dataIssues: ['no_rain_source'],
}

/** Fixture by field id; any other field gets `NO_RAIN_SOURCE_FIXTURE`. */
export const MOCK_TRAFFICABILITY_FIXTURES: Readonly<Record<string, MockTrafficabilityFixture>> = {
  'field-str-4': WATERLOGGED_CLAY_FIXTURE,
  'field-fov-3': NO_RAIN_SOURCE_FIXTURE,
}
