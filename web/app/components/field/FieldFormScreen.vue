<script setup lang="ts">
import type { MapDrawBadge, MapDrawValue } from '~/components/map/MapView.vue'
import type { TabItem } from '~/components/TabsNav.vue'
import { FARM_MAP_MODE_LABELS, type FarmDetailTab } from '~/utils/farmDetail'
import type { FieldFormValues } from '~/utils/fieldForm'

/**
 * "Novo talhão" / "Editar talhão" (Figma 16:82), shared by the two routes
 * (pages/farms/[id]/fields/new.vue and .../[fieldId]/edit.vue). Thin screen
 * shell: all state comes from useFieldForm; the map draws the field
 * (MapView drawing tool) over the farm perimeter and the other fields.
 */
const props = defineProps<{
  farmId: string
  /** Edit mode when set. */
  fieldId?: string | null
}>()

const config = useRuntimeConfig()
const form = useFieldForm({ farmId: () => props.farmId, fieldId: () => props.fieldId ?? null })
const {
  mode,
  header,
  tabs,
  eyebrow,
  panelTitle,
  values,
  namePlaceholder,
  cropChips,
  selectCrop,
  standHint,
  polygon,
  undo,
  canUndo,
  clearDrawing,
  drawHint,
  areaPreview,
  perimeterWarning,
  step,
  steps,
  goToDraw,
  goToDetails,
  mapPerimeter,
  contextFeatures,
  mapMode,
  mapColorBy,
  mapBasemap,
  fieldErrors,
  formError,
  touch,
  submit,
  saving,
  cancelTo,
  pending,
  error,
  isNotFound,
} = form

useHead({ title: () => `${mode.value === 'edit' ? 'Editar talhão' : 'Novo talhão'} · Agentes Agro` })

const isSatelliteAvailable = Boolean(config.public.mapSatelliteUrl)
const TAB_UNAVAILABLE_HINT = 'Em breve'
const isFirstLoad = computed(() => pending.value && !header.value.title)

const tabItems = computed<readonly TabItem<FarmDetailTab>[]>(() =>
  tabs.value.map((item) => ({
    value: item.value,
    label: item.label,
    count: item.count ?? undefined,
    disabled: item.disabled,
    hint: item.disabled ? TAB_UNAVAILABLE_HINT : undefined,
  })),
)
// The tabs only show where the screen sits; "Áreas e talhões" is the farm detail.
const activeTab = computed<FarmDetailTab>({
  get: (): FarmDetailTab => 'fields',
  set: () => {
    void navigateTo(cancelTo.value)
  },
})

/** The drawing tool works with polygons here; a point never comes back in polygon mode. */
const drawValue = computed<MapDrawValue | null>({
  get: () => polygon.value,
  set: (value) => {
    polygon.value = value && 'type' in value ? value : null
  },
})

const drawBadge = computed<MapDrawBadge | null>(() =>
  areaPreview.value.approximate || mode.value === 'edit'
    ? { eyebrow: mode.value === 'edit' ? 'TALHÃO' : 'NOVO TALHÃO', value: areaPreview.value.label }
    : null,
)

function updateValue(key: Exclude<keyof FieldFormValues, 'cropCode'>, value: string) {
  values[key] = value
}

const panelRef = ref<{ focusFirstInvalid: () => void } | null>(null)
const DRAW_HELP = 'Clique com o botão direito num vértice para apagá-lo; clique no ponto do meio de uma aresta para inserir um vértice.'

async function onSubmit() {
  const saved = await submit()
  if (saved) return
  await nextTick()
  panelRef.value?.focusFirstInvalid()
}

const mapAlt = computed(() => `Mapa da ${header.value.title || 'fazenda'} para desenhar o talhão`)
</script>

<template>
  <div class="field-screen">
    <EmptyState
      v-if="isNotFound"
      icon="map"
      title="Talhão não encontrado"
      description="A fazenda ou o talhão não existe mais."
    >
      <template #actions>
        <BaseButton :to="cancelTo">Voltar para a fazenda</BaseButton>
      </template>
    </EmptyState>

    <template v-else>
      <PageHeader :title="isFirstLoad ? 'Carregando fazenda…' : header.title" :breadcrumbs="header.breadcrumbs">
        <template #badge>
          <StatusBadge v-if="header.condition" :tone="header.condition.value" :label="header.condition.label" />
        </template>
      </PageHeader>

      <TabsNav v-model="activeTab" :items="tabItems" label="Seções da fazenda" />

      <ToastAlert
        v-if="error"
        tone="error"
        title="Não foi possível carregar a fazenda"
        :message="error.message"
        :dismissible="false"
      >
        <template #actions>
          <BaseButton variant="secondary" size="sm" :to="cancelTo">Voltar</BaseButton>
        </template>
      </ToastAlert>

      <div v-else class="field-screen__body">
        <MapView
          v-model:draw-value="drawValue"
          class="field-screen__map"
          variant="farm"
          rounded
          height="var(--field-form-map-height)"
          :alt="mapAlt"
          :perimeter="mapPerimeter"
          :features="contextFeatures"
          :color-by="mapColorBy"
          :basemap="mapBasemap"
          draw-mode="polygon"
          :draw-badge="drawBadge"
          controls
          draw-label="Desenhar de novo"
          @draw="clearDrawing"
        >
          <template #toolbar>
            <MapLayerSwitch v-model="mapMode" :labels="FARM_MAP_MODE_LABELS" :satellite-available="isSatelliteAvailable" />
          </template>
          <template #hint="{ descriptionId }">
            <MapDrawHint
              :text="drawHint"
              :help="DRAW_HELP"
              :description-id="descriptionId"
              :can-undo="canUndo"
              @undo="undo"
            />
          </template>
        </MapView>

        <div v-if="pending && isFirstLoad" class="field-screen__skeleton" aria-busy="true" aria-label="Carregando formulário" />
        <FieldFormPanel
          v-else
          ref="panelRef"
          class="field-screen__panel"
          :values="values"
          :eyebrow="eyebrow"
          :title="panelTitle"
          :steps="steps"
          :area-preview="areaPreview"
          :perimeter-warning="perimeterWarning"
          :name-placeholder="namePlaceholder"
          :crop-chips="cropChips"
          :stand-hint="standHint"
          :field-errors="fieldErrors"
          :form-error="formError"
          :saving="saving"
          :cancel-to="cancelTo"
          :step="step"
          @update-value="updateValue"
          @select-crop="selectCrop"
          @touch="touch"
          @go-to-draw="goToDraw"
          @go-to-details="goToDetails"
          @submit="onSubmit"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.field-screen {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

/* Figma 16:82: map filling the row + 380 px panel; the map is as tall as the panel. */
.field-screen__body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) var(--field-form-panel-width);
  gap: var(--space-20);
  align-items: start;
}

/* Fixed to the viewport, independent of the panel: the warning appearing never moves the drawing. */
.field-screen {
  --field-form-map-height: clamp(var(--field-form-map-min-height), calc(100vh - var(--field-form-map-offset)), var(--field-form-map-max-height));
}

.field-screen__map {
  position: sticky;
  top: var(--space-16);
}

.field-screen__skeleton {
  min-height: var(--farm-map-view-height);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

@media (max-width: 1099px) {
  .field-screen__body {
    grid-template-columns: minmax(0, 1fr);
  }

  .field-screen {
    --field-form-map-height: var(--farm-map-view-height-compact);
  }

  .field-screen__map {
    /* Not sticky on narrow screens, but still the positioning box of the map layers. */
    position: relative;
    top: auto;
  }
}
</style>
