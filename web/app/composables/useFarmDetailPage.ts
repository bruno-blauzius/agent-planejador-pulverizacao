import { computed, ref, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref, type WritableComputedRef } from 'vue'
import type { ApiError } from '~/services/errors'
import type { PerimeterGeometry } from '~/types/geo'
import type { MapBasemap, MapColorBy } from '~/theme/map-style'
import {
  blockKey,
  COMING_SOON,
  cultivarColorSlots,
  farmDetailTabs,
  mapBasemapFor,
  mapColorByFor,
  toFieldMapFeatures,
  isFarmDetailTabEnabled,
  fieldShortLabel,
  newFieldRoute,
  statusLegend,
  toConditionView,
  toFieldBlockView,
  toFieldPopoverView,
  toFieldRowView,
  toFieldsSummaryView,
  type ConditionView,
  type DisabledAction,
  type FarmDetailTab,
  type FarmDetailTabView,
  type FarmMapMode,
  type FieldBlockView,
  type FieldMapFeature,
  type FieldPopoverView,
  type FieldsSummaryView,
  type MapLegendView,
} from '~/utils/farmDetail'
import { useFarm } from './useFarm'
import { useFarmFields } from './useFarmFields'
import { usePermissions } from './usePermissions'

export interface FarmDetailHeaderView {
  /** Farm name; empty until loaded. */
  readonly title: string
  readonly breadcrumbs: readonly { readonly label: string, readonly to?: string }[]
  /** Badge next to the title, from `farm.sprayOutlook` (API). */
  readonly condition: ConditionView | null
}

export interface FarmDetailPage {
  /**
   * `registry.write` of the session (ADR 0013): hide "Nova fazenda", "Novo talhão",
   * "Editar" and similar actions when false. Presentation only; the API answers 403.
   */
  readonly canWrite: ComputedRef<boolean>
  readonly header: ComputedRef<FarmDetailHeaderView>
  /** Stays on `fields`: the other tabs have no screen yet and are disabled. */
  readonly tab: WritableComputedRef<FarmDetailTab>
  readonly tabs: ComputedRef<readonly FarmDetailTabView[]>
  readonly summary: ComputedRef<FieldsSummaryView>
  readonly blocks: ComputedRef<readonly FieldBlockView[]>
  readonly toggleBlock: (key: string) => void
  /** `?field=<id>` (replace); unknown ids read as null. */
  readonly selectedFieldId: WritableComputedRef<string | null>
  /** Selects a field (or clears with null) and expands its block. */
  readonly selectField: (id: string | null) => void
  readonly popover: ComputedRef<FieldPopoverView | null>
  readonly mapMode: Ref<FarmMapMode>
  readonly mapColorBy: ComputedRef<MapColorBy>
  readonly mapBasemap: ComputedRef<MapBasemap>
  readonly mapFeatures: ComputedRef<readonly FieldMapFeature[]>
  readonly mapPerimeter: ComputedRef<PerimeterGeometry | null>
  readonly mapLabels: ComputedRef<readonly { readonly id: string, readonly label: string }[]>
  readonly legend: ComputedRef<MapLegendView>
  /** "Novo talhão" (screen 10). */
  readonly newFieldTo: ComputedRef<string>
  /** "Adicionar talhão à Área Norte": block of the selected field, else the first one. */
  readonly addFieldToBlock: ComputedRef<{ readonly label: string, readonly to: string }>
  /** "Nova área": blocks are not an entity of the API yet. */
  readonly newArea: DisabledAction
  readonly pending: ComputedRef<boolean>
  /** Error of the farm, else of the fields. */
  readonly error: ComputedRef<ApiError | null>
  readonly isNotFound: ComputedRef<boolean>
  /** 403 (`forbidden` or `out_of_scope`): the farm is outside the session's permission or farm scope. */
  readonly isForbidden: ComputedRef<boolean>
  /** Farm loaded without any field. */
  readonly isEmpty: ComputedRef<boolean>
  readonly refresh: () => Promise<void>
}

function isForbiddenCode(code: ApiError['code'] | undefined): boolean {
  return code === 'forbidden' || code === 'out_of_scope'
}

/** Screen "Fazenda · Áreas e talhões" (Figma 15:126). */
export function useFarmDetailPage(farmId: MaybeRefOrGetter<string>, options: { now?: () => Date } = {}): FarmDetailPage {
  const now = options.now ?? (() => new Date())
  const route = useRoute()
  const farmResource = useFarm(farmId)
  const fieldsResource = useFarmFields(farmId)

  const activeTab = ref<FarmDetailTab>('fields')
  const collapsedBlocks = ref<ReadonlySet<string>>(new Set())
  const mapMode = ref<FarmMapMode>('crop')

  const fields = fieldsResource.fields
  const colorSlots = computed(() => cultivarColorSlots(fieldsResource.blocks.value.flatMap(block => block.fields)))

  const selectedFieldId = computed<string | null>({
    get: () => {
      const raw = route.query.field
      const candidate = Array.isArray(raw) ? raw[0] : raw
      return fields.value.find(field => field.id === candidate)?.id ?? null
    },
    set: (id) => {
      void navigateTo({ query: { ...route.query, field: id ?? undefined } }, { replace: true })
    },
  })
  const selectedField = computed(() => fields.value.find(field => field.id === selectedFieldId.value) ?? null)

  function setCollapsed(key: string, collapsed: boolean): void {
    const next = new Set(collapsedBlocks.value)
    if (collapsed) next.add(key)
    else next.delete(key)
    collapsedBlocks.value = next
  }

  function selectField(id: string | null): void {
    const field = fields.value.find(item => item.id === id)
    if (field) setCollapsed(blockKey(field.block), false)
    selectedFieldId.value = field?.id ?? null
  }

  const blocks = computed(() => fieldsResource.blocks.value.map((block) => {
    const rows = block.fields.map(field => toFieldRowView(field, colorSlots.value.indexOf(field), selectedFieldId.value))
    return toFieldBlockView(block, rows, !collapsedBlocks.value.has(blockKey(block.name)))
  }))

  const farm = farmResource.farm
  const header = computed<FarmDetailHeaderView>(() => {
    const name = farm.value?.name ?? ''
    return {
      title: name,
      breadcrumbs: name ? [{ label: 'Fazendas', to: '/farms' }, { label: name }] : [{ label: 'Fazendas', to: '/farms' }],
      condition: toConditionView(farm.value?.sprayOutlook?.condition),
    }
  })

  const tab = computed<FarmDetailTab>({
    get: () => activeTab.value,
    set: (value) => {
      if (isFarmDetailTabEnabled(value)) activeTab.value = value
    },
  })

  const hasFieldsData = computed(() => !fieldsResource.error.value && (!fieldsResource.pending.value || fields.value.length > 0))

  const addFieldToBlock = computed(() => {
    const block = selectedField.value?.block ?? fieldsResource.blocks.value.find(item => item.name !== null)?.name ?? null
    const id = toValue(farmId)
    return block
      ? { label: `Adicionar talhão à ${block}`, to: newFieldRoute(id, block) }
      : { label: 'Adicionar talhão', to: newFieldRoute(id) }
  })

  const farmError = farmResource.error
  const permissions = usePermissions()
  const error = computed(() => farmError.value ?? fieldsResource.error.value)
  const pending = computed(() => farmResource.pending.value || fieldsResource.pending.value)

  return {
    header,
    tab,
    tabs: computed(() => farmDetailTabs({
      fields: fieldsResource.pending.value && fields.value.length === 0 ? null : fields.value.length,
      owners: farm.value ? farm.value.owners.length : null,
    })),
    summary: computed(() => toFieldsSummaryView(fieldsResource.blocks.value, hasFieldsData.value)),
    blocks,
    toggleBlock: key => setCollapsed(key, !collapsedBlocks.value.has(key)),
    selectedFieldId,
    selectField,
    popover: computed(() => (selectedField.value ? toFieldPopoverView(selectedField.value, now()) : null)),
    mapMode,
    mapColorBy: computed(() => mapColorByFor(mapMode.value)),
    mapBasemap: computed(() => mapBasemapFor(mapMode.value)),
    mapFeatures: computed(() => toFieldMapFeatures(fields.value, colorSlots.value.indexOf)),
    mapPerimeter: computed(() => farm.value?.location.perimeter ?? farm.value?.perimeter ?? null),
    mapLabels: computed(() => fields.value.map(field => ({ id: field.id, label: fieldShortLabel(field.name) }))),
    legend: computed<MapLegendView>(() => (mapMode.value === 'status'
      ? { kind: 'status', items: statusLegend(fields.value) }
      : { kind: 'crop', items: colorSlots.value.legend })),
    newFieldTo: computed(() => newFieldRoute(toValue(farmId))),
    addFieldToBlock,
    newArea: COMING_SOON,
    pending,
    error,
    isNotFound: computed(() => farmError.value?.code === 'not_found'),
    isForbidden: computed(() => isForbiddenCode(farmError.value?.code) || isForbiddenCode(fieldsResource.error.value?.code)),
    isEmpty: computed(() => !pending.value && !error.value && fields.value.length === 0),
    refresh: async () => {
      await Promise.all([farmResource.refresh(), fieldsResource.refresh()])
    },
    canWrite: computed(() => permissions.can('registry.write')),
  }
}
