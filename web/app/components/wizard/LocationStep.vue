<script setup lang="ts">
import type { MapDrawBadge, MapDrawValue } from '~/components/map/MapView.vue'
import type { SegmentOption } from '~/components/SegmentedControl.vue'
import type { OptionCardItem } from '~/components/OptionCardGroup.vue'
import type { ImportFact } from './PerimeterImportCard.vue'
import type { GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { LocationMode, PerimeterMethod } from '~/types/wizard'
import type { PerimeterImportSummary } from '~/utils/wizardLocation'
import { isPolygonValue } from '~/components/map/mapDraw'

/**
 * Step 2 — Localização (Figma 14:162 address + draw, 36:604 pin the
 * headquarters, 37:635 import a file). Presentation only: values via v-model,
 * geometry and actions through events; areas, validation and the import come
 * ready from the wizard composable.
 */
export type LocationField = 'postalCode' | 'city' | 'state' | 'carNumber' | 'latitude' | 'longitude'

const props = withDefaults(
  defineProps<{
    perimeter: PerimeterGeometry | null
    forecastPoint: GeoPoint | null
    errors?: Partial<Record<string, string>>
    /** "486,2 ha" (API) or "≈ 41,2 ha · aproximada" (drawn). */
    perimeterAreaLabel?: string | null
    importAccept: string
    importing?: boolean
    importError?: string | null
    importSummary?: PerimeterImportSummary | null
    locating?: boolean
    locateError?: string | null
    /** "Desfazer" available: there is an earlier perimeter (draw or import). */
    canUndo?: boolean
    /** Non-blocking: the headquarters is outside the perimeter. */
    headquartersWarning?: string | null
    /** Handles a pasted "lat, lng" pair; returns true when it filled both fields. */
    coordinatePaste?: (text: string) => boolean
  }>(),
  {
    errors: () => ({}),
    perimeterAreaLabel: null,
    importing: false,
    importError: null,
    importSummary: null,
    locating: false,
    locateError: null,
    headquartersWarning: null,
    canUndo: false,
    coordinatePaste: undefined,
  },
)

const mode = defineModel<LocationMode>('mode', { required: true })
const perimeterMethod = defineModel<PerimeterMethod>('perimeterMethod', { required: true })
const postalCode = defineModel<string>('postalCode', { required: true })
const city = defineModel<string>('city', { required: true })
const state = defineModel<string>('state', { required: true })
const carNumber = defineModel<string>('carNumber', { required: true })
const latitude = defineModel<string>('latitude', { required: true })
const longitude = defineModel<string>('longitude', { required: true })

const emit = defineEmits<{
  touch: [field: LocationField]
  drawPerimeter: [perimeter: PerimeterGeometry | null]
  clearPerimeter: []
  setForecastPoint: [point: GeoPoint | null]
  importFile: [file: File]
  locate: []
  undo: []
}>()

const MODE_OPTIONS: readonly SegmentOption<LocationMode>[] = [
  { value: 'address', label: 'Por endereço' },
  { value: 'pin', label: 'Marcar no mapa' },
  { value: 'file', label: 'Importar arquivo' },
]

const UNAVAILABLE_HINT = 'Em breve'

const DRAW_OPTION: OptionCardItem<PerimeterMethod> = {
  value: 'draw',
  title: 'Desenhar no mapa',
  description: 'Clique nos vértices',
  icon: 'pen',
}
const FILE_OPTION: OptionCardItem<PerimeterMethod> = {
  value: 'file',
  title: 'Importar arquivo',
  description: 'KML, KMZ ou Shapefile',
  icon: 'upload',
}
// No SICAR endpoint yet (contract tela-05, Passo 2).
const CAR_OPTION: OptionCardItem<PerimeterMethod> = {
  value: 'car',
  title: 'Usar o CAR',
  description: 'Perímetro do SICAR',
  icon: 'document',
  unavailableHint: UNAVAILABLE_HINT,
}

const HINTS = {
  drawStart: 'Clique no primeiro ponto para fechar o perímetro',
  drawDone: 'Perímetro fechado · arraste os vértices para ajustar',
  pinStart: 'Toque no mapa para marcar a sede da fazenda',
  pinDone: 'Depois de marcar a sede, desenhe o perímetro ou importe o arquivo',
} as const

// Start view with nothing drawn yet: the headquarters when known, else the south of
// Minas Gerais (the coffee region of the pilot farms); no geocoding endpoint for the city.
const REGION_VIEW = { center: { lat: -21.4, lng: -46.2 }, zoom: 9 } as const
const HEADQUARTERS_ZOOM = 14


/** Pin mode: the map draws the perimeter instead of moving the headquarters. */
const isPinDrawingPerimeter = ref(false)
/** File mode: the imported perimeter is being adjusted on the map. */
const isAdjustingImport = ref(false)
/** The "file has fields" banner was dismissed for this file. */
const isFieldBannerDismissed = ref(false)

const addressOptions = [DRAW_OPTION, CAR_OPTION]
const pinOptions = [DRAW_OPTION, FILE_OPTION, CAR_OPTION]

const drawMode = computed<'polygon' | 'point' | null>(() => {
  if (mode.value === 'pin') return isPinDrawingPerimeter.value ? 'polygon' : 'point'
  if (mode.value === 'file') return isAdjustingImport.value ? 'polygon' : null
  return 'polygon'
})

const editablePerimeter = computed(() => (props.perimeter?.type === 'Polygon' ? props.perimeter : null))

const drawValue = computed<MapDrawValue | null>({
  get: () => (drawMode.value === 'point' ? props.forecastPoint : editablePerimeter.value),
  set: (value) => {
    if (drawMode.value === 'point') {
      emit('setForecastPoint', value && !isPolygonValue(value) ? value : null)
      return
    }
    emit('drawPerimeter', isPolygonValue(value) ? value : null)
  },
})

/** Shown as a plain layer when the map is not editing it. */
const displayedPerimeter = computed(() => (drawMode.value === 'polygon' ? null : props.perimeter))

const initialView = computed(() =>
  props.forecastPoint ? { center: props.forecastPoint, zoom: HEADQUARTERS_ZOOM } : REGION_VIEW,
)

const drawBadge = computed<MapDrawBadge | null>(() =>
  drawMode.value === 'polygon' && props.perimeter && props.perimeterAreaLabel
    ? { eyebrow: 'Área do perímetro', value: props.perimeterAreaLabel }
    : null,
)

const hint = computed(() => {
  if (drawMode.value === 'point') return props.forecastPoint ? HINTS.pinDone : HINTS.pinStart
  return props.perimeter ? HINTS.drawDone : HINTS.drawStart
})

const canUndoDrawing = computed(() => drawMode.value === 'polygon' && props.canUndo)

const pinMethod = computed<PerimeterMethod | null>({
  get: () => (isPinDrawingPerimeter.value ? 'draw' : null),
  set: (value) => {
    if (value === 'file') {
      mode.value = 'file'
      return
    }
    isPinDrawingPerimeter.value = value === 'draw'
    if (value) perimeterMethod.value = value
  },
})

const importFacts = computed<ImportFact[]>(() =>
  props.importSummary
    ? [
        { label: 'Área calculada', value: props.importSummary.areaLabel },
        { label: 'Município', value: props.importSummary.cityLabel },
        { label: 'Sede (centro)', value: props.importSummary.forecastPointLabel },
        { label: 'Sistema', value: props.importSummary.datumLabel },
      ]
    : [],
)

const fieldCountMessage = computed(() => {
  const count = props.importSummary?.fieldCount ?? 0
  if (count <= 0 || isFieldBannerDismissed.value) return null
  const polygons = count === 1 ? '1 polígono interno' : `${count} polígonos internos`
  return `Encontramos ${polygons}. Você poderá cadastrá-los como talhões depois de salvar a fazenda.`
})

const mapAlt = computed(() =>
  drawMode.value === 'point'
    ? 'Mapa para marcar a sede da fazenda'
    : 'Mapa com o perímetro da fazenda',
)

function onImport(files: File[]) {
  const [file] = files
  if (!file) return
  isAdjustingImport.value = false
  isFieldBannerDismissed.value = false
  emit('importFile', file)
}

function removeImport() {
  isAdjustingImport.value = false
  emit('clearPerimeter')
}

function onCoordinatePaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text') ?? ''
  if (props.coordinatePaste?.(text)) event.preventDefault()
}

watch(mode, () => {
  isPinDrawingPerimeter.value = false
  isAdjustingImport.value = false
})
</script>

<template>
  <div class="location-step">
    <div class="location-step__group">
      <p class="location-step__label text-label-field" aria-hidden="true">Como localizar a fazenda?</p>
      <SegmentedControl
        v-model="mode"
        class="location-step__mode"
        :options="MODE_OPTIONS"
        label="Como localizar a fazenda?"
        block
      />
    </div>

    <!-- Por endereço (14:162) -->
    <template v-if="mode === 'address'">
      <div class="location-step__address-row">
        <TextField
          v-model="postalCode"
          class="location-step__postal-code"
          label="CEP"
          name="postalCode"
          placeholder="37800-000"
          inputmode="numeric"
          autocomplete="postal-code"
          :maxlength="9"
          :error="errors.postalCode"
          @blur="emit('touch', 'postalCode')"
        />
        <TextField
          v-model="city"
          label="Município"
          name="city"
          placeholder="Guaxupé"
          autocomplete="address-level2"
          required
          :error="errors.city"
          @blur="emit('touch', 'city')"
        />
        <TextField
          v-model="state"
          class="location-step__state"
          label="UF"
          name="state"
          placeholder="MG"
          autocomplete="address-level1"
          :maxlength="2"
          required
          :error="errors.state"
          @blur="emit('touch', 'state')"
        />
      </div>

      <TextField
        v-model="carNumber"
        label="Nº do CAR (opcional)"
        name="carNumber"
        placeholder="MG-3128709-8F2A…C41E"
        hint="Guardamos o número; a busca do perímetro no SICAR chega em breve."
        autocomplete="off"
        :error="errors.carNumber"
        @blur="emit('touch', 'carNumber')"
      />

      <OptionCardGroup v-model="perimeterMethod" label="Como definir o perímetro?" :options="addressOptions" variant="compact" />

      <MapView
        v-model:draw-value="drawValue"
        class="location-step__map"
        variant="farm"
        rounded
        :alt="mapAlt"
        :perimeter="displayedPerimeter"
        :draw-mode="drawMode"
        :draw-badge="drawBadge"
        :initial-view="initialView"
        height="var(--wizard-map-height-md)"
        controls
      >
        <template #hint>
          <MapDrawHint :text="hint" :can-undo="canUndoDrawing" @undo="emit('undo')" />
        </template>
      </MapView>
    </template>

    <!-- Marcar no mapa (36:604) -->
    <template v-else-if="mode === 'pin'">
      <p class="location-step__info text-body-sm">
        <AppIcon name="pin" :size="20" class="location-step__info-icon" />
        Toque no mapa para posicionar a sede da fazenda. Não precisa de CEP nem de endereço: a previsão do tempo usa as coordenadas.
      </p>

      <div class="location-step__coordinates" @paste="onCoordinatePaste">
        <TextField
          v-model="latitude"
          label="Latitude"
          name="latitude"
          placeholder="-21,3045"
          inputmode="decimal"
          autocomplete="off"
          :error="errors.latitude"
          @blur="emit('touch', 'latitude')"
        />
        <TextField
          v-model="longitude"
          label="Longitude"
          name="longitude"
          placeholder="-46,7128"
          inputmode="decimal"
          autocomplete="off"
          :error="errors.longitude"
          @blur="emit('touch', 'longitude')"
        />
        <BaseButton
          class="location-step__locate"
          variant="secondary"
          icon="locate"
          :loading="locating"
          @click="emit('locate')"
        >
          Usar minha localização
        </BaseButton>
      </div>
      <p v-if="locateError" class="location-step__error text-body-sm" role="alert">{{ locateError }}</p>
      <p v-if="headquartersWarning" class="location-step__warning text-body-sm" role="status">
        <AppIcon name="alert" :size="16" />
        {{ headquartersWarning }}
      </p>

      <div class="location-step__city-row">
        <TextField
          v-model="city"
          label="Município"
          name="city"
          placeholder="Guaxupé"
          autocomplete="address-level2"
          required
          :error="errors.city"
          @blur="emit('touch', 'city')"
        />
        <TextField
          v-model="state"
          class="location-step__state"
          label="UF"
          name="state"
          placeholder="MG"
          autocomplete="address-level1"
          :maxlength="2"
          required
          :error="errors.state"
          @blur="emit('touch', 'state')"
        />
      </div>

      <MapView
        v-model:draw-value="drawValue"
        class="location-step__map"
        variant="farm"
        rounded
        :alt="mapAlt"
        :perimeter="displayedPerimeter"
        :draw-mode="drawMode"
        :draw-badge="drawBadge"
        :initial-view="initialView"
        height="var(--wizard-map-height-lg)"
        controls
      >
        <template #hint>
          <MapDrawHint :text="hint" :can-undo="canUndoDrawing" @undo="emit('undo')" />
        </template>
      </MapView>

      <OptionCardGroup v-model="pinMethod" label="Depois: como definir o perímetro?" :options="pinOptions" variant="compact" />
    </template>

    <!-- Importar arquivo (37:635) -->
    <template v-else>
      <FileDropzone
        layout="hero"
        title="Arraste o arquivo da fazenda aqui"
        description="KML, KMZ, Shapefile (.zip) ou GeoJSON · até 20 MB · o arquivo do CAR também serve"
        action-label="Selecionar arquivo"
        :accept="importAccept"
        :disabled="importing"
        :error="importError ?? undefined"
        @select="onImport"
      />
      <p v-if="importing" class="location-step__status text-body-sm" role="status">Processando o arquivo…</p>
      <!-- The dropzone shows the error; this live region announces it when it appears. -->
      <p class="visually-hidden" role="alert">{{ importError }}</p>

      <WizardPerimeterImportCard
        v-if="importSummary"
        :file-name="importSummary.fileName"
        :detail="importSummary.detailLabel"
        :facts="importFacts"
        @remove="removeImport"
      />

      <div class="location-step__city-row">
        <TextField
          v-model="city"
          label="Município"
          name="city"
          placeholder="Guaxupé"
          autocomplete="address-level2"
          required
          :error="errors.city"
          @blur="emit('touch', 'city')"
        />
        <TextField
          v-model="state"
          class="location-step__state"
          label="UF"
          name="state"
          placeholder="MG"
          autocomplete="address-level1"
          :maxlength="2"
          required
          :error="errors.state"
          @blur="emit('touch', 'state')"
        />
      </div>

      <MapView
        v-if="perimeter"
        v-model:draw-value="drawValue"
        class="location-step__map"
        variant="farm"
        rounded
        :alt="mapAlt"
        :perimeter="displayedPerimeter"
        :draw-mode="drawMode"
        :draw-badge="drawBadge"
        :initial-view="initialView"
        height="var(--wizard-map-height-md)"
        controls
      >
        <template v-if="isAdjustingImport" #hint>
          <MapDrawHint :text="hint" :can-undo="canUndoDrawing" @undo="emit('undo')" />
        </template>
        <div v-if="!isAdjustingImport" class="location-step__map-actions">
          <button
            v-if="editablePerimeter"
            type="button"
            class="location-step__adjust text-label-badge"
            @click="isAdjustingImport = true"
          >
            <AppIcon name="pen" :size="14" />
            Ajustar vértices
          </button>
          <p v-if="perimeterAreaLabel" class="location-step__map-badge text-label-badge">
            <AppIcon name="check" :size="14" />
            Perímetro importado · {{ perimeterAreaLabel }}
          </p>
        </div>
      </MapView>

      <ToastAlert
        v-if="fieldCountMessage"
        tone="agent"
        title="O arquivo já tem os talhões"
        :message="fieldCountMessage"
        @dismiss="isFieldBannerDismissed = true"
      />
    </template>

    <p v-if="headquartersWarning && mode !== 'pin'" class="location-step__warning text-body-sm" role="status">
      <AppIcon name="alert" :size="16" />
      {{ headquartersWarning }}
    </p>

    <p
      v-if="errors.perimeter"
      class="location-step__error text-body-sm"
      role="alert"
      tabindex="-1"
      data-focus-on-error
    >
      {{ errors.perimeter }}
    </p>
  </div>
</template>

<style scoped>
.location-step {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.location-step__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.location-step__label {
  margin: 0;
  color: var(--color-text-primary);
}

.location-step__address-row {
  display: grid;
  grid-template-columns: var(--wizard-postal-code-width) minmax(0, 1fr) var(--wizard-city-state-width);
  gap: var(--space-16);
  align-items: start;
}

.location-step__city-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) var(--wizard-city-state-width);
  gap: var(--space-16);
  align-items: start;
}

.location-step__coordinates {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
  gap: var(--space-12);
  align-items: end;
}

.location-step__info {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  margin: 0;
  padding: var(--space-14);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.location-step__info-icon {
  flex-shrink: 0;
}

.location-step__map {
  overflow: hidden;
  border-radius: var(--radius-card-lg);
}

.location-step__map-actions {
  position: absolute;
  right: var(--space-16);
  /* Same line as the MapView hint: above the map attribution. */
  bottom: var(--space-32);
  left: var(--space-16);
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  justify-content: space-between;
  pointer-events: none;
}

.location-step__adjust,
.location-step__map-badge {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  margin: 0;
  padding: var(--space-8) var(--space-12);
  border-radius: var(--radius-tile);
  pointer-events: auto;
}

.location-step__adjust {
  border: 0;
  background: var(--color-white);
  color: var(--color-green-700);
  cursor: pointer;
}

.location-step__adjust:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.location-step__map-badge {
  margin-left: auto;
  background: var(--color-overlay-strong);
  color: var(--color-text-inverse);
}

.location-step__warning {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  color: var(--color-status-attention);
}

.location-step__status {
  margin: 0;
  color: var(--color-text-secondary);
}

.location-step__error {
  margin: 0;
  color: var(--color-status-unsuitable);
  outline: none;
}

@media (max-width: 599px) {
  /* Three long labels do not fit side by side on a phone: stack the segments. */
  .location-step__mode {
    flex-direction: column;
  }

  .location-step__mode :deep(.segmented__option) {
    width: 100%;
  }

  .location-step__address-row {
    grid-template-columns: minmax(0, 1fr) var(--wizard-city-state-width);
  }

  .location-step__postal-code {
    grid-column: 1 / -1;
  }

  .location-step__coordinates {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .location-step__locate {
    grid-column: 1 / -1;
  }
}
</style>
