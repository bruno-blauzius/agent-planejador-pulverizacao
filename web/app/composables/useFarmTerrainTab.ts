import { computed, ref, toValue, watch, type ComputedRef, type MaybeRefOrGetter, type Ref, type WritableComputedRef } from 'vue'
import type { LocationQueryRaw } from 'vue-router'
import type { ApiError } from '~/services/errors'
import type { TerrainCalculationScope } from '~/types/terrain'
import type { FieldTrafficability } from '~/types/trafficability'
import { blockKey } from '~/utils/farmDetail'
import {
  elevationRangeOf,
  indexTerrainSummaries,
  TERRAIN_METHODOLOGY,
  terrainFailures,
  terrainFieldIdFrom,
  terrainLegend,
  terrainDetailViewFrom,
  terrainTabState,
  terrainViewFrom,
  toFarmTerrainIndicators,
  toFieldTerrainDetailView,
  toTerrainBlockViews,
  toTerrainMapFeatures,
  toTerrainMapLayerOptions,
  toTerrainMissingBanner,
  toTerrainNotCalculatedView,
  toTerrainPanelHeader,
  toTerrainPopoverView,
  toTerrainProgressView,
  type FarmTerrainIndicatorsView,
  type FieldTerrainDetailView,
  type TerrainBlockView,
  type TerrainDetailView,
  type TerrainFailureView,
  type TerrainLegendView,
  type TerrainMapFeature,
  type TerrainMapLayer,
  type TerrainMapLayerOptionView,
  type TerrainMissingBannerView,
  type TerrainNotCalculatedView,
  type TerrainPanelHeaderView,
  type TerrainPopoverView,
  type TerrainProgressView,
  type TerrainTabState,
  type TerrainView,
} from '~/utils/terrain'
import type { RainGaugeService, SoilConditionsService, TerrainService, TrafficabilityService } from '~/services/ports'
import type { useFarmFields } from './useFarmFields'
import { useFarmTerrain } from './useFarmTerrain'
import { useFieldTerrain } from './useFieldTerrain'
import { useFieldTrafficability } from './useFieldTrafficability'
import { useRainGauges, type RainGaugesResource } from './useRainGauges'
import { useRainReadingForm, type RainReadingForm } from './useRainReadingForm'
import { useSoilInspectionForm, type SoilInspectionForm } from './useSoilInspectionForm'
import { useSoilProfileForm, type SoilProfileForm } from './useSoilProfileForm'
import { usePermissions } from './usePermissions'
import { useTerrainAdjustmentForm, type TerrainAdjustmentForm } from './useTerrainAdjustmentForm'

export interface FarmTerrainTab {
  readonly state: ComputedRef<TerrainTabState>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  readonly header: ComputedRef<TerrainPanelHeaderView>
  readonly indicators: ComputedRef<FarmTerrainIndicatorsView>
  readonly blocks: ComputedRef<readonly TerrainBlockView[]>
  readonly toggleBlock: (key: string) => void
  /** 04e: never calculated, or calculating with nothing ready yet. */
  readonly notCalculated: ComputedRef<TerrainNotCalculatedView | null>
  readonly progress: ComputedRef<TerrainProgressView | null>
  readonly failures: ComputedRef<readonly TerrainFailureView[]>
  /** "2 talhões sem relevo" with "Calcular" while others already have relief. */
  readonly missingBanner: ComputedRef<TerrainMissingBannerView | null>
  readonly calculate: (scope: TerrainCalculationScope) => Promise<void>
  /** Request in flight or calculation running. */
  readonly calculating: ComputedRef<boolean>
  readonly calculateError: Readonly<Ref<ApiError | null>>
  /** Dismisses the calculation error alert. */
  readonly clearCalculateError: () => void
  readonly mapLayer: Ref<TerrainMapLayer>
  readonly mapLayers: ComputedRef<readonly TerrainMapLayerOptionView[]>
  readonly mapFeatures: ComputedRef<readonly TerrainMapFeature[]>
  readonly legend: ComputedRef<TerrainLegendView>
  /** `?terrainField=<id>` (replace); unknown ids read as null. */
  readonly selectedFieldId: WritableComputedRef<string | null>
  /** Reopens the block of the field; null also leaves the detail card. */
  readonly selectField: (id: string | null) => void
  /** Only in the map view. */
  readonly popover: ComputedRef<TerrainPopoverView | null>
  /** `?terrainView=detail` (replace); "detail" without a selected field reads as "map". */
  readonly view: WritableComputedRef<TerrainView>
  readonly openDetail: () => void
  readonly backToMap: () => void
  readonly detail: ComputedRef<FieldTerrainDetailView | null>
  readonly detailPending: ComputedRef<boolean>
  readonly detailError: ComputedRef<ApiError | null>
  readonly refreshDetail: () => Promise<void>
  /**
   * `?terrainDetailView=rain` (replace): "Relevo | Chuva e solo" of the detail card (ADR 0022).
   * Reads "relief" outside the detail; cleared by `backToMap` and `selectField(null)`.
   */
  readonly detailView: WritableComputedRef<TerrainDetailView>
  /** Rain and soil of the selected field; loaded only with the tab active, the detail open and the "rain" view. */
  readonly trafficability: ComputedRef<FieldTrafficability | null>
  /**
   * Nothing to show yet: the field list is still loading for a "rain" URL, or the
   * selected field has neither an answer nor an error.
   */
  readonly trafficabilityPending: ComputedRef<boolean>
  /** Reloading with the previous answer on screen. */
  readonly trafficabilityRefreshing: ComputedRef<boolean>
  readonly trafficabilityError: ComputedRef<ApiError | null>
  readonly refreshTrafficability: () => Promise<void>
  /** Projection days asked for ("próximos 5 dias"); never derived from the answer. */
  readonly trafficabilityDays: ComputedRef<number>
  /** `field_conditions.record`: "Lançar leitura" and "Registrar vistoria". Presentation only; the API answers 403. */
  readonly canRecordConditions: ComputedRef<boolean>
  /** `registry.write`: soil profile of the field and the farm's rain gauges. */
  readonly canEditSoilProfile: ComputedRef<boolean>
  /** Gauges of the farm (register, rename); loaded once "Chuva e solo" is opened. */
  readonly rainGauges: RainGaugesResource
  /** Reading of a gauge; the field's gauge comes preselected. Saving reloads the trafficability. */
  readonly rainReadingForm: RainReadingForm
  /** Inspection of the selected field. Saving reloads the trafficability. */
  readonly soilInspectionForm: SoilInspectionForm
  /** Texture and gauge of the selected field. Saving reloads the trafficability. */
  readonly soilProfileForm: SoilProfileForm
  /** `registry.write`: adjust, remove adjustment and calculate. Presentation only; the API answers 403. */
  readonly canWrite: ComputedRef<boolean>
  readonly removeAdjustment: () => Promise<boolean>
  readonly removing: Readonly<Ref<boolean>>
  readonly removeError: Readonly<Ref<ApiError | null>>
  /** Form of the selected field. */
  readonly adjustmentForm: TerrainAdjustmentForm
  readonly methodology: { readonly title: string, readonly paragraphs: readonly string[], readonly attribution: string }
  /** For the farm screen: tab badge and header buttons. */
  readonly restrictedFieldCount: ComputedRef<number | null>
  readonly headerState: ComputedRef<{ readonly calculatedFieldCount: number, readonly running: boolean, readonly requesting: boolean } | null>
}

/**
 * Tab "Relevo" of the farm screen (Figma 67:960). It starts from the fields
 * (the same resource the "Áreas e talhões" tab uses) and crosses each one with
 * its relief row by id. Classes, method fit and the outdated flag come from the
 * API; this only presents them.
 */
export function useFarmTerrainTab(options: {
  readonly farmId: MaybeRefOrGetter<string>
  /** True while the tab is shown. */
  readonly active: MaybeRefOrGetter<boolean>
  readonly fields: ReturnType<typeof useFarmFields>
  /** Test seams. */
  readonly pollIntervalMs?: number
  readonly service?: TerrainService
  readonly trafficabilityService?: TrafficabilityService
  readonly rainGaugeService?: RainGaugeService
  readonly soilConditionsService?: SoilConditionsService
  /** Clock of the reading and inspection forms. */
  readonly now?: () => Date
  /** Projection days of the "Chuva e solo" view; defaults to the contract's. */
  readonly trafficabilityDays?: number
}): FarmTerrainTab {
  const route = useRoute()
  const permissions = usePermissions()
  const { farmId, fields: fieldsResource } = options
  const fields = fieldsResource.fields

  const farmTerrain = useFarmTerrain(farmId, {
    active: options.active,
    pollIntervalMs: options.pollIntervalMs,
    service: options.service,
  })
  const terrain = farmTerrain.terrain

  const collapsedBlocks = ref<ReadonlySet<string>>(new Set())
  const mapLayer = ref<TerrainMapLayer>('slope')

  // ---- selection (query) ----------------------------------------------------

  function replaceQuery(patch: { terrainField?: string | null, terrainView?: TerrainView, terrainDetailView?: TerrainDetailView }): void {
    const query: LocationQueryRaw = { ...route.query }
    if ('terrainField' in patch) query.terrainField = patch.terrainField ?? undefined
    if ('terrainView' in patch) query.terrainView = patch.terrainView === 'detail' ? 'detail' : undefined
    if ('terrainDetailView' in patch) query.terrainDetailView = patch.terrainDetailView === 'rain' ? 'rain' : undefined
    void navigateTo({ query }, { replace: true })
  }

  const selectedFieldId = computed<string | null>({
    get: () => {
      const candidate = terrainFieldIdFrom(route.query.terrainField)
      return fields.value.find(field => field.id === candidate)?.id ?? null
    },
    // Without a field the detail card has nothing to show, so it leaves the URL too.
    set: id => replaceQuery(id === null ? { terrainField: null, terrainView: 'map', terrainDetailView: 'relief' } : { terrainField: id }),
  })
  const selectedField = computed(() => fields.value.find(field => field.id === selectedFieldId.value) ?? null)

  const view = computed<TerrainView>({
    get: () => (selectedFieldId.value ? terrainViewFrom(route.query.terrainView) : 'map'),
    set: value => replaceQuery({ terrainView: value }),
  })

  const detailView = computed<TerrainDetailView>({
    get: () => (view.value === 'detail' ? terrainDetailViewFrom(route.query.terrainDetailView) : 'relief'),
    set: value => replaceQuery({ terrainDetailView: value }),
  })

  function selectField(id: string | null): void {
    const field = fields.value.find(item => item.id === id)
    if (field) setCollapsed(blockKey(field.block), false)
    selectedFieldId.value = field?.id ?? null
  }

  function setCollapsed(key: string, collapsed: boolean): void {
    const next = new Set(collapsedBlocks.value)
    if (collapsed) next.add(key)
    else next.delete(key)
    collapsedBlocks.value = next
  }

  // ---- field detail -------------------------------------------------------------

  const fieldTerrain = useFieldTerrain(farmId, () => (toValue(options.active) ? selectedFieldId.value : null), {
    onChanged: farmTerrain.refresh,
    service: options.service,
  })
  /** The loaded detail belongs to the selected field (a stale one from the previous selection is ignored). */
  const loadedDetail = computed(() => {
    const detail = fieldTerrain.terrain.value
    return detail && detail.fieldId === selectedFieldId.value ? detail : null
  })

  // ---- rain and soil (ADR 0022) ---------------------------------------------------

  const wantsTrafficability = computed(() => toValue(options.active) && view.value === 'detail' && detailView.value === 'rain')
  /**
   * Last field the "rain" view asked for. It stays while the user goes back to
   * "Relevo" (or the map), so the resource key does not change and returning to
   * "Chuva e solo" on the same field shows the answer already loaded, without a new GET.
   */
  const trafficabilityFieldId = ref<string | null>(null)
  watch(() => (wantsTrafficability.value ? selectedFieldId.value : null), (id) => {
    if (id) trafficabilityFieldId.value = id
  }, { immediate: true })
  const fieldTrafficability = useFieldTrafficability(farmId, trafficabilityFieldId, {
    days: options.trafficabilityDays,
    service: options.trafficabilityService,
  })
  /** Shown only for the selected field while the "rain" view is open. */
  const showsTrafficability = computed(() => wantsTrafficability.value && trafficabilityFieldId.value === selectedFieldId.value)
  const shownTrafficability = computed(() => (showsTrafficability.value ? fieldTrafficability.trafficability.value : null))
  const shownTrafficabilityError = computed(() => (showsTrafficability.value ? fieldTrafficability.error.value : null))
  /**
   * A URL that asks for "rain" while the fields still load: the selection (and so
   * the detail) is only known once they arrive, but the panel must not sit empty.
   */
  const awaitingFieldsForRain = computed(() => toValue(options.active)
    && fieldsResource.pending.value
    && selectedFieldId.value === null
    && terrainViewFrom(route.query.terrainView) === 'detail'
    && terrainDetailViewFrom(route.query.terrainDetailView) === 'rain'
    && terrainFieldIdFrom(route.query.terrainField) !== null)
  const trafficabilityPending = computed(() => awaitingFieldsForRain.value
    || (wantsTrafficability.value && selectedFieldId.value !== null && shownTrafficability.value === null && shownTrafficabilityError.value === null))

  // ---- what people record: gauges, readings, inspections, soil profile (phase 2) ------------

  /** Once "Chuva e solo" was opened the gauges stay loaded (no reload on every switch). */
  const rainViewOpened = ref(false)
  watch(wantsTrafficability, (wants) => {
    if (wants) rainViewOpened.value = true
  }, { immediate: true })
  const rainGauges = useRainGauges(farmId, {
    enabled: rainViewOpened,
    // The gauge name shows in the soil profile and in the trafficability (source of the rain).
    onRenamed: async () => {
      await Promise.all([soilProfileForm.refresh(), fieldTrafficability.refresh()])
    },
    service: options.rainGaugeService,
  })
  const soilProfileForm = useSoilProfileForm(farmId, trafficabilityFieldId, {
    gauges: rainGauges.gauges,
    gaugesPending: rainGauges.pending,
    gaugesError: rainGauges.error,
    enabled: rainViewOpened,
    onSaved: () => fieldTrafficability.refresh(),
    service: options.soilConditionsService,
  })
  const rainReadingForm = useRainReadingForm(farmId, {
    gauges: rainGauges.gauges,
    defaultGaugeId: () => soilProfileForm.profile.value?.rainGauge?.id ?? null,
    onSaved: () => fieldTrafficability.refresh(),
    now: options.now,
    service: options.rainGaugeService,
  })
  const soilInspectionForm = useSoilInspectionForm(farmId, selectedFieldId, {
    onSaved: () => fieldTrafficability.refresh(),
    now: options.now,
    service: options.soilConditionsService,
  })

  const summariesById = computed(() => indexTerrainSummaries(terrain.value?.items ?? []))
  const range = computed(() => elevationRangeOf(terrain.value?.summary))
  const canWrite = computed(() => permissions.can('registry.write'))
  const state = computed(() => terrainTabState({
    terrain: terrain.value,
    pending: farmTerrain.pending.value,
    hasError: farmTerrain.error.value !== null,
  }))
  const calculatedFieldCount = computed(() => terrain.value?.summary.calculatedFieldCount ?? 0)

  // ---- calculation -------------------------------------------------------------

  async function calculate(scope: TerrainCalculationScope): Promise<void> {
    await farmTerrain.calculate(scope)
  }

  // When the calculation ends the open detail is stale (new values, method fit).
  watch(farmTerrain.isRunning, (running, wasRunning) => {
    if (wasRunning && !running && selectedFieldId.value) void fieldTerrain.refresh()
  })

  const fieldsWithoutRelief = computed(() => fields.value.filter(field => !summariesById.value.get(field.id)?.measurement).length)

  return {
    state,
    error: farmTerrain.error,
    refresh: farmTerrain.refresh,
    header: computed(() => toTerrainPanelHeader(terrain.value)),
    indicators: computed(() => toFarmTerrainIndicators(terrain.value)),
    blocks: computed(() => toTerrainBlockViews(fieldsResource.blocks.value, summariesById.value, collapsedBlocks.value, selectedFieldId.value)),
    toggleBlock: key => setCollapsed(key, !collapsedBlocks.value.has(key)),
    notCalculated: computed(() => {
      const emptyState = state.value === 'not_calculated' || (state.value === 'calculating' && calculatedFieldCount.value === 0)
      return emptyState ? toTerrainNotCalculatedView(terrain.value?.summary.fieldCount ?? fields.value.length, canWrite.value) : null
    }),
    progress: computed(() => (terrain.value?.progress ? toTerrainProgressView(terrain.value.progress) : null)),
    failures: computed(() => terrainFailures(terrain.value?.items ?? [])),
    missingBanner: computed(() => (state.value === 'ready' ? toTerrainMissingBanner(fieldsWithoutRelief.value, canWrite.value) : null)),
    calculate,
    calculating: computed(() => farmTerrain.calculating.value || farmTerrain.isRunning.value),
    calculateError: farmTerrain.calculateError,
    clearCalculateError: farmTerrain.clearCalculateError,
    mapLayer,
    mapLayers: computed(() => toTerrainMapLayerOptions(calculatedFieldCount.value > 0)),
    mapFeatures: computed(() => toTerrainMapFeatures(fields.value, summariesById.value, mapLayer.value, range.value)),
    legend: computed(() => terrainLegend(mapLayer.value, fields.value.map(field => summariesById.value.get(field.id) ?? null), range.value)),
    selectedFieldId,
    selectField,
    popover: computed(() => (selectedField.value && view.value === 'map'
      ? toTerrainPopoverView(selectedField.value, summariesById.value.get(selectedField.value.id) ?? null, loadedDetail.value)
      : null)),
    view,
    openDetail: () => {
      if (selectedFieldId.value) view.value = 'detail'
    },
    backToMap: () => {
      replaceQuery({ terrainView: 'map', terrainDetailView: 'relief' })
    },
    detail: computed(() => (selectedField.value && loadedDetail.value ? toFieldTerrainDetailView(selectedField.value, loadedDetail.value) : null)),
    detailPending: computed(() => Boolean(selectedFieldId.value) && loadedDetail.value === null && fieldTerrain.error.value === null),
    detailError: fieldTerrain.error,
    refreshDetail: fieldTerrain.refresh,
    detailView,
    trafficability: shownTrafficability,
    trafficabilityPending,
    trafficabilityRefreshing: computed(() => showsTrafficability.value && fieldTrafficability.refreshing.value),
    trafficabilityError: shownTrafficabilityError,
    refreshTrafficability: fieldTrafficability.refresh,
    trafficabilityDays: fieldTrafficability.days,
    canRecordConditions: computed(() => permissions.can('field_conditions.record')),
    canEditSoilProfile: canWrite,
    rainGauges,
    rainReadingForm,
    soilInspectionForm,
    soilProfileForm,
    canWrite,
    removeAdjustment: async () => (await fieldTerrain.removeAdjustment()) !== null,
    removing: fieldTerrain.removing,
    removeError: fieldTerrain.removeError,
    adjustmentForm: useTerrainAdjustmentForm({ fieldName: () => selectedField.value?.name ?? '', field: fieldTerrain }),
    methodology: TERRAIN_METHODOLOGY,
    restrictedFieldCount: computed(() => (terrain.value ? terrain.value.summary.restrictedFieldCount : null)),
    headerState: computed(() => (terrain.value
      ? { calculatedFieldCount: calculatedFieldCount.value, running: farmTerrain.isRunning.value, requesting: farmTerrain.calculating.value }
      : null)),
  }
}
