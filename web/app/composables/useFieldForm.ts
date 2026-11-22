import { computed, reactive, ref, shallowRef, toRaw, toValue, watch, type MaybeRefOrGetter } from 'vue'
import type { Field } from '~/types/field'
import type { GeoJsonPolygon } from '~/types/geo'
import {
  cultivarColorSlots,
  farmDetailTabs,
  mapBasemapFor,
  mapColorByFor,
  toConditionView,
  toFieldMapFeatures,
  formatFieldArea,
  type FarmMapMode,
} from '~/utils/farmDetail'
import {
  cropChips,
  drawHintFor,
  emptyFieldFormValues,
  FIELD_FORM_FIELDS,
  FIELD_FORM_MESSAGES,
  fieldErrorsFromApi,
  fieldToFormValues,
  nextFieldName,
  samePolygon,
  toFieldInput,
  validateFieldForm,
  type FieldFormErrors,
  type FieldFormField,
  type FieldFormStep,
  type FieldFormValues,
} from '~/utils/fieldForm'
import { approximateAreaHa, isDrawablePolygon, vertexCount, verticesInside } from '~/utils/geoArea'
import type { ApiError } from '~/services/errors'
import { forbiddenMessageFor } from '~/utils/permissions'
import { useCrops } from './useCrops'
import { useFarm } from './useFarm'
import { useFarmFields } from './useFarmFields'
import { useFlash } from './useFlash'

export interface FieldFormOptions {
  readonly farmId: MaybeRefOrGetter<string>
  /** Edit mode when set; the field is looked up in the farm's field list. */
  readonly fieldId?: MaybeRefOrGetter<string | null | undefined>
}

const STEP_LABELS: Readonly<Record<FieldFormStep, string>> = { draw: 'Desenho', details: '2 · Detalhes' }
const PERIMETER_WARNING = 'Parte do desenho parece estar fora do perímetro da fazenda. Confira antes de salvar.'

function firstQueryValue(value: unknown): string | null {
  const candidate = Array.isArray(value) ? value[0] : value
  return typeof candidate === 'string' && candidate.trim() ? candidate.trim() : null
}

function formErrorFor(error: ApiError): string | null {
  const forbidden = forbiddenMessageFor(error)
  if (forbidden) return forbidden
  if (error.code === 'validation_failed' && error.details.length > 0) return null
  if (error.code === 'network_error') return FIELD_FORM_MESSAGES.network
  if (error.code === 'not_found') return FIELD_FORM_MESSAGES.notFound
  return FIELD_FORM_MESSAGES.generic
}

/**
 * Screen "Novo talhão" / "Editar talhão" (Figma 16:82): drawing (v-model of the
 * draw control, with undo), details form, approximate area preview and submit.
 * On success it shows a flash and goes back to the farm with the field selected.
 */
export function useFieldForm(options: FieldFormOptions) {
  const farmId = () => toValue(options.farmId)
  const fieldId = () => toValue(options.fieldId) ?? null
  const route = useRoute()
  const flash = useFlash()
  const farmResource = useFarm(farmId)
  const fieldsResource = useFarmFields(farmId)
  const cropsResource = useCrops()

  const mode = computed(() => (fieldId() ? 'edit' : 'create'))
  const editedField = computed<Field | null>(() => fieldsResource.fields.value.find(field => field.id === fieldId()) ?? null)

  const values = reactive<FieldFormValues>(emptyFieldFormValues(firstQueryValue(route.query.block)))
  const currentPolygon = shallowRef<GeoJsonPolygon | null>(null)
  const history = shallowRef<readonly (GeoJsonPolygon | null)[]>([])
  const step = ref<FieldFormStep>(mode.value === 'edit' ? 'details' : 'draw')
  const mapMode = ref<FarmMapMode>('crop')
  const touched = reactive<Record<FieldFormField, boolean>>(
    Object.fromEntries(FIELD_FORM_FIELDS.map(field => [field, false])) as Record<FieldFormField, boolean>,
  )
  const showApiError = ref(false)
  const initializedFor = ref<string | null>(null)

  // Edit mode: fill the form once, when the field shows up in the list.
  watch(editedField, (field) => {
    if (!field || initializedFor.value === field.id) return
    initializedFor.value = field.id
    Object.assign(values, fieldToFormValues(field))
    // Plain object, not the reactive proxy of the list: it goes to the draw
    // control and into the request body (the mock structured-clones it).
    currentPolygon.value = structuredClone(toRaw(field.polygon))
    history.value = []
  }, { immediate: true })

  const polygon = computed<GeoJsonPolygon | null>({
    get: () => currentPolygon.value,
    set: (next) => {
      if (samePolygon(next, currentPolygon.value)) return
      history.value = [...history.value, currentPolygon.value]
      applyPolygon(next)
    },
  })

  /**
   * The first closed drawing moves on to the details, as in the Figma steps;
   * losing the drawing (undo, clear) goes back to the drawing step.
   */
  function applyPolygon(next: GeoJsonPolygon | null): void {
    const wasDrawable = isDrawablePolygon(currentPolygon.value)
    currentPolygon.value = next
    const isDrawable = isDrawablePolygon(next)
    if (!wasDrawable && isDrawable) step.value = 'details'
    if (!isDrawable) step.value = 'draw'
  }

  function undo(): void {
    const previous = history.value.at(-1)
    if (previous === undefined) return
    history.value = history.value.slice(0, -1)
    applyPolygon(previous)
  }

  function clearDrawing(): void {
    polygon.value = null
  }

  const localErrors = computed(() => validateFieldForm(values, currentPolygon.value))
  const actionError = computed(() => (mode.value === 'edit' ? fieldsResource.updateError.value : fieldsResource.createError.value))
  const apiError = computed(() => (showApiError.value ? actionError.value : null))

  watch([() => ({ ...values }), currentPolygon], () => {
    showApiError.value = false
  })

  const fieldErrors = computed<FieldFormErrors>(() => {
    const visibleLocal: FieldFormErrors = {}
    for (const field of FIELD_FORM_FIELDS) {
      if (touched[field] && localErrors.value[field]) visibleLocal[field] = localErrors.value[field]
    }
    return { ...fieldErrorsFromApi(apiError.value?.details ?? []), ...visibleLocal }
  })

  function touch(field: FieldFormField): void {
    touched[field] = true
  }

  function goToDetails(): boolean {
    if (!isDrawablePolygon(currentPolygon.value)) {
      touch('polygon')
      return false
    }
    step.value = 'details'
    return true
  }

  const saving = computed(() => fieldsResource.creating.value || fieldsResource.updating.value)

  async function submit(): Promise<boolean> {
    FIELD_FORM_FIELDS.forEach(touch)
    const drawn = currentPolygon.value
    if (Object.keys(localErrors.value).length > 0 || !isDrawablePolygon(drawn) || saving.value) return false

    const editing = editedField.value
    const input = toFieldInput(values, drawn, editing?.season ?? null)
    const saved = editing
      ? await fieldsResource.updateField(editing.id, input)
      : await fieldsResource.createField(input)
    showApiError.value = saved === null
    if (!saved) return false

    flash.show({
      kind: 'success',
      title: `${saved.name} ${editing ? 'atualizado' : 'cadastrado'}`,
      description: `Área de ${formatFieldArea(saved.areaHa)}.`,
      highlightId: saved.id,
    })
    await navigateTo(`/farms/${encodeURIComponent(farmId())}?field=${encodeURIComponent(saved.id)}`)
    return true
  }

  const farm = farmResource.farm
  const areaPreview = computed(() => {
    const drawn = currentPolygon.value
    if (!isDrawablePolygon(drawn)) return { label: '—', caption: 'desenhe o talhão no mapa', approximate: false }
    const original = editedField.value
    if (original && samePolygon(drawn, original.polygon)) {
      return { label: formatFieldArea(original.areaHa), caption: 'área do cadastro', approximate: false }
    }
    return { label: formatFieldArea(approximateAreaHa(drawn)), caption: 'aproximados, calculados a partir do desenho', approximate: true }
  })

  const mapPerimeter = computed(() => farm.value?.location.perimeter ?? farm.value?.perimeter ?? null)
  const colorSlots = computed(() => cultivarColorSlots(fieldsResource.blocks.value.flatMap(block => block.fields)))

  const blockLabel = computed(() => values.block.trim().toLocaleUpperCase('pt-BR'))

  const pending = computed(() => farmResource.pending.value || fieldsResource.pending.value || cropsResource.pending.value)
  const loadError = computed(() => farmResource.error.value ?? fieldsResource.error.value ?? cropsResource.error.value)
  const fieldsLoaded = computed(() => !fieldsResource.pending.value && !fieldsResource.error.value)

  return {
    mode,
    header: computed(() => {
      const name = farm.value?.name ?? ''
      const id = farmId()
      const current = mode.value === 'edit' ? (editedField.value?.name ?? 'Editar talhão') : 'Novo talhão'
      return {
        title: name,
        breadcrumbs: [
          { label: 'Fazendas', to: '/farms' },
          ...(name ? [{ label: name, to: `/farms/${encodeURIComponent(id)}` }] : []),
          { label: current },
        ],
        condition: toConditionView(farm.value?.sprayOutlook?.condition),
      }
    }),
    tabs: computed(() => farmDetailTabs({
      fields: fieldsResource.pending.value && fieldsResource.fields.value.length === 0 ? null : fieldsResource.fields.value.length,
      owners: farm.value ? farm.value.owners.length : null,
    })),
    eyebrow: computed(() => {
      const prefix = mode.value === 'edit' ? 'EDITAR TALHÃO' : 'NOVO TALHÃO'
      return blockLabel.value ? `${prefix} · ${blockLabel.value}` : prefix
    }),
    panelTitle: 'Detalhes do talhão',

    values,
    namePlaceholder: computed(() => nextFieldName(fieldsResource.fields.value)),
    cropChips: computed(() => cropChips(cropsResource.crops.value, values.cropCode)),
    selectCrop: (code: string) => {
      values.cropCode = code
      touch('cropCode')
    },
    standHint: computed(() => {
      const plantsPerHa = editedField.value?.plantsPerHa
      const unchanged = editedField.value && fieldToFormValues(editedField.value).spacing === values.spacing
      if (plantsPerHa && unchanged) {
        return `≈ ${new Intl.NumberFormat('pt-BR').format(plantsPerHa)} plantas/ha · usado no cálculo de calda`
      }
      return 'Estande calculado ao salvar · usado no cálculo de calda'
    }),

    polygon,
    undo,
    canUndo: computed(() => history.value.length > 0),
    clearDrawing,
    vertexCount: computed(() => vertexCount(currentPolygon.value)),
    drawHint: computed(() => drawHintFor(vertexCount(currentPolygon.value))),
    areaPreview,
    perimeterWarning: computed(() => {
      const drawn = currentPolygon.value
      const perimeter = mapPerimeter.value
      if (!perimeter || !isDrawablePolygon(drawn)) return null
      return verticesInside(drawn, perimeter) ? null : PERIMETER_WARNING
    }),

    step,
    steps: computed(() => (['draw', 'details'] as const).map(value => ({
      value,
      label: STEP_LABELS[value],
      done: value === 'draw' && step.value === 'details',
      active: value === step.value,
    }))),
    goToDraw: () => {
      step.value = 'draw'
    },
    goToDetails,

    mapPerimeter,
    contextFeatures: computed(() => toFieldMapFeatures(
      fieldsResource.fields.value.filter(field => field.id !== fieldId()),
      colorSlots.value.indexOf,
    )),
    mapMode,
    mapColorBy: computed(() => mapColorByFor(mapMode.value)),
    mapBasemap: computed(() => mapBasemapFor(mapMode.value)),

    fieldErrors,
    formError: computed(() => (apiError.value ? formErrorFor(apiError.value) : null)),
    touch,
    submit,
    saving,
    cancelTo: computed(() => {
      const base = `/farms/${encodeURIComponent(farmId())}`
      const id = fieldId()
      return id ? `${base}?field=${encodeURIComponent(id)}` : base
    }),

    pending,
    error: loadError,
    isNotFound: computed(() => farmResource.error.value?.code === 'not_found'
      || (mode.value === 'edit' && fieldsLoaded.value && editedField.value === null)),
  }
}
