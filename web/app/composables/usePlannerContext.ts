import { computed, watch } from 'vue'
import type { Field } from '~/types/field'
import type { PlannerRequestContext, ProductKind } from '~/types/planner'
import { fieldShortLabel } from '~/utils/farmDetail'
import { PRODUCT_KIND_LABELS } from '~/utils/planner'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useFarms } from './useFarms'
import { registerLogoutCleanup } from './useLogoutCleanup'
import { useSession } from './useSession'

const FARM_OPTIONS_PAGE_SIZE = 100
/** `farmId` undefined = not chosen yet: the user's own farm applies (so a new user never inherits the previous one). */
interface ContextState {
  farmId?: string | null
  fieldIds: readonly string[]
  productKind: ProductKind | null
}

const initialState = (): ContextState => ({ fieldIds: [], productKind: 'fungicide' })

const PRODUCT_ORDER: readonly ProductKind[] = ['fungicide', 'insecticide', 'herbicide', 'foliar', 'other']

/**
 * "Fazenda / Talhões / Produto" bar of the Planner (screen 16). The selection is
 * shared (`useState`) by the chat, the window map and the conditions chart. The
 * farm list already comes scoped by the API (operators only see their farms).
 */
export function usePlannerContext() {
  const { fields: fieldService } = useApiServices()
  const { user } = useSession()
  const state = useState<ContextState>('planner:context', initialState)
  const farmId = computed(() => (state.value.farmId === undefined ? user.value?.primaryFarm?.id ?? null : state.value.farmId))

  // The selection belongs to the signed-in user: logout or another login starts over.
  registerLogoutCleanup('planner-context', () => {
    state.value = initialState()
  })
  watch(() => user.value?.id, (id, previous) => {
    if (id !== previous) state.value = initialState()
  })

  const farms = useFarms({ pageSize: FARM_OPTIONS_PAGE_SIZE })
  const fields = useResource<readonly Field[]>(
    () => `planner:fields:${farmId.value ?? 'none'}`,
    () => (farmId.value ? fieldService.listByFarm(farmId.value) : Promise.resolve([])),
    () => [],
  )

  const fieldOptions = computed(() => fields.data.value.map(field => ({
    value: field.id,
    label: fieldShortLabel(field.name),
    name: field.name,
    areaHa: field.areaHa,
  })))

  /** Valid only with a farm and at least one field: what `POST /planner/messages` accepts. */
  const requestContext = computed<PlannerRequestContext | null>(() => {
    const { fieldIds, productKind } = state.value
    return farmId.value && fieldIds.length > 0 ? { farmId: farmId.value, fieldIds: [...fieldIds], productKind } : null
  })

  function selectFarm(next: string | null) {
    if (next === farmId.value) return
    // Fields belong to one farm: a new farm starts with no field selected.
    state.value = { ...state.value, farmId: next, fieldIds: [] }
  }

  function setFields(fieldIds: readonly string[]) {
    state.value = { ...state.value, fieldIds: [...new Set(fieldIds)] }
  }

  function toggleField(fieldId: string) {
    const selected = state.value.fieldIds
    setFields(selected.includes(fieldId) ? selected.filter(id => id !== fieldId) : [...selected, fieldId])
  }

  function setProductKind(productKind: ProductKind | null) {
    state.value = { ...state.value, productKind }
  }

  return {
    farmId,
    fieldIds: computed(() => state.value.fieldIds),
    productKind: computed(() => state.value.productKind),

    farmOptions: computed(() => farms.farms.value.map(farm => ({ value: farm.id, label: farm.name }))),
    fieldOptions,
    productOptions: PRODUCT_ORDER.map(value => ({ value, label: PRODUCT_KIND_LABELS[value] })),
    /** "T3 · T7"; empty while nothing is selected. */
    selectedFieldsLabel: computed(() => fieldOptions.value
      .filter(option => state.value.fieldIds.includes(option.value))
      .map(option => option.label)
      .join(' · ')),

    requestContext,
    isReady: computed(() => requestContext.value !== null),
    pending: computed(() => farms.pending.value || fields.pending.value),
    error: computed(() => farms.error.value ?? fields.error.value),
    /** No farm released for this user. */
    isEmpty: computed(() => !farms.pending.value && !farms.error.value && farms.farms.value.length === 0),

    selectFarm,
    setFields,
    toggleField,
    setProductKind,
    refresh: async () => {
      await Promise.all([farms.refresh(), fields.refresh()])
    },
  }
}
