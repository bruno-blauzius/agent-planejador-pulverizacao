<script setup lang="ts">
import type { MapLegendItem } from '~/components/map/MapLegend.vue'
import type { SegmentOption } from '~/components/SegmentedControl.vue'
import type { FarmTerrainTab } from '~/composables/useFarmTerrainTab'
import { TERRAIN_FALLBACK_COLOR, terrainPalette } from '~/theme/map-style'
import type { PerimeterGeometry } from '~/types/geo'
import type { TerrainMapLayer } from '~/utils/terrain'
import type { FieldConditionsDialog } from '~/components/trafficability/FieldConditionsDialogs.vue'

/**
 * Content of the "Terreno" tab (formerly "Relevo") (Figma 04b 67:961, 04c 67:1276, 04e 67:1906):
 * on the left the relief list (or, before any calculation, the "ainda não
 * calculado" card); on the right the map with its layers, legend and popover,
 * or the field detail (04c). Hosts the adjustment modal. The "Como calculamos"
 * dialog belongs to the page, because its button is in the page header.
 * Presentation only: every state and action comes from `tab` (useFarmTerrainTab).
 * The field detail switches between "Relevo" and "Chuva e solo" (Figma 04f),
 * kept in the URL by the tab (`detailView`, `?terrainDetailView=rain`).
 */
const props = withDefaults(defineProps<{
  tab: FarmTerrainTab
  /** Farm perimeter drawn behind the fields. */
  perimeter?: PerimeterGeometry | null
  /** Accessible description of the map. */
  mapAlt: string
  /** The farm's fields are still loading. */
  fieldsLoading?: boolean
  /** Farm without fields. */
  empty?: boolean
  /** "Desenhe o primeiro talhão" (only with `registry.write`). */
  addField?: { readonly label: string, readonly to: string } | null
}>(), { perimeter: null, fieldsLoading: false, empty: false, addField: null })

// The tab object is created once per page; its refs are unwrapped here for the template.
const {
  state,
  error,
  refresh,
  header,
  indicators,
  blocks,
  toggleBlock,
  notCalculated,
  progress,
  failures,
  missingBanner,
  calculate,
  calculating,
  calculateError,
  clearCalculateError,
  mapLayer,
  mapLayers,
  mapFeatures,
  legend,
  selectedFieldId,
  selectField,
  popover,
  view,
  openDetail,
  backToMap,
  detail,
  detailPending,
  detailError,
  refreshDetail,
  detailView,
  trafficability,
  trafficabilityPending,
  trafficabilityRefreshing,
  trafficabilityError,
  refreshTrafficability,
  trafficabilityDays,
  canRecordConditions,
  canEditSoilProfile,
  soilProfileForm,
  canWrite,
  removeAdjustment,
  removing,
  removeError,
  adjustmentForm,
} = props.tab

const MAP_LAYER_UNAVAILABLE = 'Disponível depois de calcular o relevo'
/** Figma 67:2093: replaces the legend while no field has relief. */
const LAYERS_AFTER_CALCULATION_NOTICE = 'As camadas de declividade, altitude e exposição aparecem depois do cálculo.'

const listPanelRef = ref<{ focusField: (id: string) => void } | null>(null)
const detailRef = ref<{ focus: () => void } | null>(null)
const adjustOpen = ref(false)
/** Open dialog of the "Chuva e solo" view (reading, inspection, profile, gauges). */
const conditionsDialog = ref<FieldConditionsDialog | null>(null)
const soilProfile = soilProfileForm.profile
const soilProfilePending = soilProfileForm.pending

const isLoading = computed(() => state.value === 'loading' || props.fieldsLoading)
const isDetailOpen = computed(() => view.value === 'detail' && selectedFieldId.value !== null)

const layerOptions = computed<readonly SegmentOption<TerrainMapLayer>[]>(() =>
  mapLayers.value.map((option) => ({
    value: option.value,
    label: option.label,
    disabled: option.disabled,
    hint: option.disabled ? MAP_LAYER_UNAVAILABLE : undefined,
  })),
)

/** Title of the detail card before (or without) the relief: the field name from the list. */
const selectedFieldName = computed(() => {
  const id = selectedFieldId.value
  if (!id) return null
  for (const block of blocks.value) {
    const row = block.fields.find((field) => field.id === id)
    if (row) return row.name
  }
  return null
})

const palette = computed(() => terrainPalette(legend.value.layer))

const legendItems = computed<readonly MapLegendItem[]>(() =>
  legend.value.items.map((item) => ({ key: item.key, label: item.label, color: palette.value[item.key] ?? TERRAIN_FALLBACK_COLOR })),
)

const mapErrorMessage = computed(() => error.value?.message ?? null)

/** Returns focus to the list row of the selected field (after closing the popover or the detail). */
async function focusSelectedRow(id: string | null) {
  if (!id) return
  await nextTick()
  listPanelRef.value?.focusField(id)
}

async function closePopover() {
  const id = selectedFieldId.value
  selectField(null)
  await focusSelectedRow(id)
}

async function onOpenDetail() {
  openDetail()
  await nextTick()
  detailRef.value?.focus()
}

async function onBackToMap() {
  const id = selectedFieldId.value
  backToMap()
  await focusSelectedRow(id)
}

// Escape closes the popover wherever the focus is (map canvas, list, body).
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && popover.value && !adjustOpen.value && !conditionsDialog.value && !event.defaultPrevented) {
    event.preventDefault()
    void closePopover()
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="terrain-tab">
    <EmptyState
      v-if="empty"
      icon="pen"
      title="Nenhum talhão cadastrado"
      description="O relevo é calculado por talhão. Cadastre os talhões da fazenda para calcular declividade, altitude e exposição."
    >
      <template v-if="addField" #actions>
        <BaseButton :to="addField.to" icon="plus">{{ addField.label }}</BaseButton>
      </template>
    </EmptyState>

    <template v-else>
      <ToastAlert
        v-if="calculateError"
        tone="error"
        title="Não foi possível calcular o relevo"
        :message="calculateError.message"
        @dismiss="clearCalculateError()"
      />

      <div class="terrain-tab__body">
        <TerrainEmptyState
          v-if="notCalculated"
          class="terrain-tab__list"
          :view="notCalculated"
          :progress="progress"
          :failures="failures"
          :calculating="calculating"
          @calculate="calculate('missing')"
        />
        <TerrainFieldListPanel
          v-else
          ref="listPanelRef"
          class="terrain-tab__list"
          :header="header"
          :indicators="indicators"
          :blocks="blocks"
          :loading="isLoading && !blocks.length"
          :error="mapErrorMessage"
          :progress="progress"
          :failures="failures"
          :missing-banner="missingBanner"
          :calculating="calculating"
          @select="selectField"
          @toggle="toggleBlock"
          @calculate="calculate('missing')"
          @retry="refresh()"
        />

        <TerrainFieldDetail
          v-if="isDetailOpen"
          ref="detailRef"
          v-model:view="detailView"
          class="terrain-tab__main"
          :detail="detail"
          :pending="detailPending"
          :error="detailError?.message ?? null"
          :can-write="canWrite"
          :removing="removing"
          :remove-error="removeError?.message ?? null"
          :field-name="selectedFieldName"
          @back="onBackToMap"
          @adjust="adjustOpen = true"
          @remove="removeAdjustment()"
          @retry="refreshDetail()"
        >
          <template v-if="canRecordConditions" #rainActions>
            <BaseButton variant="secondary" size="sm" icon="rain" @click="conditionsDialog = 'reading'">Lançar pluviômetro</BaseButton>
            <BaseButton variant="ghost" size="sm" icon="drop" @click="conditionsDialog = 'inspection'">Registrar vistoria</BaseButton>
          </template>
          <template #rain>
            <TrafficabilityPanel
              :trafficability="trafficability"
              :pending="trafficabilityPending"
              :refreshing="trafficabilityRefreshing"
              :error="trafficabilityError?.message ?? null"
              :horizon-days="trafficabilityDays"
              :soil-profile="soilProfile"
              :soil-profile-pending="soilProfilePending"
              :can-edit-profile="canEditSoilProfile"
              @retry="refreshTrafficability()"
              @edit-profile="conditionsDialog = 'profile'"
            />
          </template>
        </TerrainFieldDetail>

        <MapView
          v-else
          class="terrain-tab__main terrain-tab__map"
          variant="farm"
          rounded
          height="var(--terrain-map-height)"
          :alt="mapAlt"
          :perimeter="perimeter"
          :features="mapFeatures"
          color-by="terrain"
          :terrain-palette="palette"
          :selected-id="selectedFieldId"
          controls
          @select="selectField"
          @deselect="selectField(null)"
        >
          <template #toolbar>
            <div class="terrain-tab__layers" :class="{ 'terrain-tab__layers--off': notCalculated }" :aria-disabled="notCalculated ? 'true' : undefined">
              <SegmentedControl v-model="mapLayer" :options="layerOptions" label="Camada do mapa" variant="contrast" />
            </div>
          </template>
          <template v-if="notCalculated" #legend>
            <p class="terrain-tab__notice text-label-chip" role="status">{{ LAYERS_AFTER_CALCULATION_NOTICE }}</p>
          </template>
          <template v-else-if="legendItems.length" #legend>
            <MapLegend :items="legendItems" :label="`Legenda: ${legend.title}`" />
          </template>
          <template v-if="popover" #popover>
            <TerrainMapPopover :popover="popover" @close="closePopover" @open-detail="onOpenDetail" />
          </template>
        </MapView>
      </div>
    </template>

    <TrafficabilityFieldConditionsDialogs v-model:dialog="conditionsDialog" :tab="tab" :field-name="selectedFieldName" />
    <TerrainAdjustmentModal v-if="adjustOpen" :form="adjustmentForm" @close="adjustOpen = false" />
  </div>
</template>

<style scoped>
.terrain-tab {
  --terrain-map-height: var(--farm-map-view-height);
  /* Figma 67:960: the relief list is wider than the field list (420 × 360 px). */
  --terrain-panel-width: 420px;

  display: flex;
  flex-direction: column;
  gap: var(--space-16);
}

/* 420 px list + the rest, 20 px apart (Figma 67:961). */
.terrain-tab__body {
  display: grid;
  grid-template-columns: var(--terrain-panel-width) minmax(0, 1fr);
  gap: var(--space-20);
  align-items: start;
}

/* Not calculated (Figma 67:2093): layers are off, so the switch is dimmed and inert. */
.terrain-tab__layers--off {
  opacity: 0.5;
  pointer-events: none;
}

.terrain-tab__notice {
  margin: 0;
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-primary);
}

/* Narrower screens: the Figma is 1440 px; the map (or the detail) goes above the list. */
@media (max-width: 1099px) {
  .terrain-tab {
    --terrain-map-height: var(--farm-map-view-height-compact);
  }

  .terrain-tab__body {
    grid-template-columns: minmax(0, 1fr);
  }

  .terrain-tab__main {
    order: -1;
  }
}
</style>
