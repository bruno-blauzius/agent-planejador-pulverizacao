<script lang="ts">
import farmMapUrl from '~/assets/images/map-farm.svg'
import thumbnailMapUrl from '~/assets/images/map-thumbnail.svg'
import { MAP_ATTRIBUTION } from '~/theme/map-style'
import type { MapBasemap, MapColorBy } from '~/theme/map-style'
import type { GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { SprayCondition } from '~/types/spray'
import type { MapDrawMode, MapDrawValue } from './mapDraw'

export type { MapDrawMode, MapDrawValue }

/** Projection key of the drawn polygon (badge anchor). */
export const DRAW_ANCHOR_ID = '__draw'

/** Badge over the drawn polygon (Figma 16:407: "NOVO TALHÃO / 41,2 ha"). */
export interface MapDrawBadge {
  readonly eyebrow: string
  readonly value: string
}

/** Polygon drawn on the map (farm perimeter or field). */
export interface MapFeature {
  readonly id: string
  readonly geometry: PerimeterGeometry
  /** Color by status (`colorBy="status"`); values come from the API. */
  readonly status?: SprayCondition | null
  /** Color slot by crop (`colorBy="crop"`), index into the Figma field palette. */
  readonly cropIndex?: number
  /** Color key of the layer (`colorBy="terrain"`), looked up in `terrainPalette`. */
  readonly terrainKey?: string
  /** Dashed outline (field without a value to paint, e.g. relief not calculated). */
  readonly dashed?: boolean
  /** Short text of the pill drawn over the polygon (Figma 15:279, e.g. "T1"); interactive maps only. */
  readonly label?: string
  /** Drawn as an outline only (the farm perimeter behind its fields). */
  readonly outlineOnly?: boolean
}

/** Pixel position inside the map box. */
export interface MapScreenPoint {
  readonly x: number
  readonly y: number
}

export interface MapBounds {
  readonly west: number
  readonly south: number
  readonly east: number
  readonly north: number
}

export type MapVariant = 'thumbnail' | 'farm'
</script>

<script setup lang="ts">

/**
 * Map facade (ADR 0012). Interactive MapLibre map rendered only on the client
 * (MapCanvas inside <ClientOnly>); the Figma illustration (7:174, 15:266) is
 * the SSR placeholder, the fallback without WebGL and the image used when no
 * geometry is given. Free overlays go in the default slot; the farm map
 * (Figma 15:265) also has positioned slots: `toolbar` (top left, e.g.
 * MapLayerSwitch), `legend` (bottom left, MapLegend) and `popover` (next to the
 * selected feature, MapPopover). Features with `label` get a pill at their
 * centroid; the selected one is highlighted (outline and pill).
 *
 * Drawing (Terra Draw, components/map/mapDraw.ts): `drawMode` ('polygon' |
 * 'point') turns it on and `v-model:draw-value` holds the geometry. The model
 * is updated only when a stroke or drag finishes, so each update is one undo
 * step; setting it from outside (initial value, undo, clear) redraws it.
 * `drawBadge` shows a badge on the drawn polygon; the `hint` slot sits at the
 * bottom center (Figma 16:410) and receives `descriptionId`: give it to the
 * element with the drawing instructions (MapDrawHint `description-id`), so the
 * map, exposed as role="application" while drawing, is described by it.
 *
 * `thumbnail`: not interactive, fitted to the geometry and turned into a static
 * image once rendered (frees the WebGL context: lists can have many cards).
 */
const props = withDefaults(
  defineProps<{
    variant?: MapVariant
    /** Accessible description of what the map shows. */
    alt: string
    rounded?: boolean
    /** Farm perimeter, drawn like a feature. */
    perimeter?: PerimeterGeometry | null
    features?: readonly MapFeature[]
    colorBy?: MapColorBy
    /** Color per `terrainKey` (`theme/map-style.ts` `terrainPalette(layer)`), for `colorBy="terrain"`. */
    terrainPalette?: Readonly<Record<string, string>>
    selectedId?: string | null
    basemap?: MapBasemap
    /** Defaults to true for `farm` and false for `thumbnail`. */
    interactive?: boolean
    /** Fixed CSS height instead of the variant aspect ratio (e.g. map mode of a list). */
    height?: string
    /** Shows the project zoom/fit controls (Figma 15:302); interactive maps only. */
    controls?: boolean
    /** Accessible name of the pen control (MapControls); omit to hide it. */
    drawLabel?: string
    /** Pressed state of the pen when it toggles a drawing tool. */
    drawActive?: boolean | null
    /** Enables the drawing tool (interactive maps only). */
    drawMode?: MapDrawMode | null
    drawBadge?: MapDrawBadge | null
    /** Start view when there is no geometry (with `drawMode`, the live map shows anyway). */
    initialView?: { center: GeoPoint, zoom: number } | null
  }>(),
  {
    variant: 'thumbnail',
    rounded: false,
    perimeter: null,
    features: () => [],
    colorBy: 'crop',
    terrainPalette: () => ({}),
    selectedId: null,
    basemap: 'vector',
    interactive: undefined,
    height: undefined,
    controls: false,
    drawLabel: undefined,
    drawActive: null,
    drawMode: null,
    drawBadge: null,
    initialView: null,
  },
)

const drawValue = defineModel<MapDrawValue | null>('drawValue', { default: null })

const emit = defineEmits<{
  /** The live map finished loading (MapCanvas `ready`). */
  ready: []
  select: [id: string]
  /** Click on the map background (no feature). */
  deselect: []
  move: [bounds: MapBounds]
  draw: []
}>()

defineSlots<{
  default?: () => unknown
  toolbar?: () => unknown
  legend?: () => unknown
  popover?: () => unknown
  hint?: (props: { descriptionId: string }) => unknown
}>()

const hintDescriptionId = useId()

// Natural sizes of the Figma exports: 540×180 and 724×643.556.
const VARIANTS: Record<MapVariant, { src: string, aspectRatio: string }> = {
  thumbnail: { src: thumbnailMapUrl, aspectRatio: '540 / 180' },
  farm: { src: farmMapUrl, aspectRatio: '724 / 643.556' },
}

const PERIMETER_ID = 'perimeter'
const THUMBNAIL_FIT_PADDING = 24
const FARM_FIT_PADDING = 48
/** Popover placement: gap to the feature anchor and minimum margin to the map edges (px). */
const POPOVER_OFFSET = 20
const POPOVER_MARGIN = 16

const canvasRef = ref<{ zoomIn: () => void, zoomOut: () => void, fit: () => void } | null>(null)
const isMapReady = ref(false)
const hasMapError = ref(false)
const snapshotUrl = ref<string | null>(null)
const rootRef = ref<HTMLElement | null>(null)
const popoverRef = ref<HTMLElement | null>(null)
const toolbarRef = ref<HTMLElement | null>(null)
const legendRef = ref<HTMLElement | null>(null)
/** Vertical band free of the toolbar (top) and the legend (bottom), in px. */
const freeBand = shallowRef({ top: 0, bottom: 0 })
const points = shallowRef<Readonly<Record<string, MapScreenPoint>>>({})
const boxSize = shallowRef({ width: 0, height: 0 })
const popoverSize = shallowRef({ width: 0, height: 0 })
let popoverObserver: ResizeObserver | null = null

const current = computed(() => VARIANTS[props.variant])
const allFeatures = computed<readonly MapFeature[]>(() =>
  props.perimeter
    ? [{ id: PERIMETER_ID, geometry: props.perimeter, outlineOnly: props.features.length > 0 }, ...props.features]
    : props.features,
)
const isThumbnail = computed(() => props.variant === 'thumbnail')
const isInteractive = computed(() => props.interactive ?? !isThumbnail.value)
// A drawing map is live even without geometry: there is nothing to fit yet, but something to draw.
const hasGeometry = computed(() => allFeatures.value.length > 0 || Boolean(props.drawMode && isInteractive.value))
const isDrawing = computed(() => Boolean(props.drawMode && isInteractive.value))
const isDrawReady = ref(false)
/** Until the map and Terra Draw are ready, clicks would be lost: the map shows a loading cover. */
const isDrawLoading = computed(() => isDrawing.value && !isDrawReady.value && !hasMapError.value)
const showLiveMap = computed(() => hasGeometry.value && !hasMapError.value && !snapshotUrl.value)
const showIllustration = computed(() => !hasGeometry.value || hasMapError.value || (!isMapReady.value && !snapshotUrl.value))
const sizeStyle = computed(() => (props.height ? { height: props.height } : { aspectRatio: current.value.aspectRatio }))

const labels = computed(() =>
  props.features.flatMap((feature) => {
    const point = points.value[feature.id]
    return feature.label && point ? [{ id: feature.id, text: feature.label, point, isSelected: feature.id === props.selectedId }] : []
  }),
)

const drawBadgePoint = computed(() => (props.drawBadge ? points.value[DRAW_ANCHOR_ID] : undefined))

const selectedPoint = computed(() => (props.selectedId ? points.value[props.selectedId] : undefined))

/** Below the anchor when it fits, otherwise above; always kept inside the map box. */
const popoverStyle = computed(() => {
  const anchor = selectedPoint.value
  if (!anchor) return undefined
  const { width, height } = boxSize.value
  const size = popoverSize.value
  // Keeps the toolbar and the legend usable; on maps too short for that, falls back to the edges.
  const band = freeBand.value
  const fitsBand = band.bottom - band.top >= size.height
  const minY = fitsBand ? band.top : POPOVER_MARGIN
  const maxX = Math.max(POPOVER_MARGIN, width - size.width - POPOVER_MARGIN)
  const maxY = Math.max(minY, (fitsBand ? band.bottom : height - POPOVER_MARGIN) - size.height)
  const below = anchor.y + POPOVER_OFFSET
  const above = anchor.y - POPOVER_OFFSET - size.height
  const y = below <= maxY ? below : above
  const x = anchor.x - size.width / 2
  return {
    transform: `translate(${clamp(x, POPOVER_MARGIN, maxX)}px, ${clamp(y, minY, maxY)}px)`,
    // Measured on the first frame before it is shown, so it never jumps.
    visibility: size.width ? undefined : ('hidden' as const),
  }
})

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}

function onMapReady() {
  isMapReady.value = true
  emit('ready')
}

function onSelect(id: string) {
  if (id !== PERIMETER_ID) emit('select', id)
}

function onProject(next: Readonly<Record<string, MapScreenPoint>>) {
  points.value = next
  const height = rootRef.value?.clientHeight ?? 0
  boxSize.value = { width: rootRef.value?.clientWidth ?? 0, height }
  const toolbar = toolbarRef.value
  const legend = legendRef.value
  freeBand.value = {
    top: toolbar ? toolbar.offsetTop + toolbar.offsetHeight + POPOVER_MARGIN / 2 : POPOVER_MARGIN,
    bottom: legend ? legend.offsetTop - POPOVER_MARGIN / 2 : height - POPOVER_MARGIN,
  }
}

function labelStyle(point: MapScreenPoint) {
  return { transform: `translate(${point.x}px, ${point.y}px) translate(-50%, -50%)` }
}

watch(popoverRef, (element) => {
  popoverObserver?.disconnect()
  popoverObserver = null
  if (!element) {
    popoverSize.value = { width: 0, height: 0 }
    return
  }
  popoverObserver = new ResizeObserver(() => {
    popoverSize.value = { width: element.offsetWidth, height: element.offsetHeight }
  })
  popoverObserver.observe(element)
})

onBeforeUnmount(() => popoverObserver?.disconnect())
</script>

<template>
  <div
    ref="rootRef"
    class="map-view"
    :class="{ 'map-view--rounded': rounded, 'map-view--interactive': isInteractive && showLiveMap }"
    :style="sizeStyle"
  >
    <!-- Only the map layer is the image; overlays below stay interactive siblings. -->
    <!-- While drawing, the focusable canvas is an application, described by the hint. -->
    <div
      class="map-view__media"
      :role="isDrawing ? 'application' : 'img'"
      :aria-label="alt"
      :aria-roledescription="isDrawing ? 'mapa de desenho' : undefined"
      :aria-describedby="isDrawing && $slots.hint ? hintDescriptionId : undefined"
    >
      <img
        v-if="showIllustration"
        :src="current.src"
        alt=""
        class="map-view__image"
        :class="{ 'map-view__image--placeholder': hasGeometry && !hasMapError }"
      >
      <img v-if="snapshotUrl" :src="snapshotUrl" alt="" class="map-view__image">
      <ClientOnly>
        <MapCanvas
          v-if="showLiveMap"
          ref="canvasRef"
          :features="allFeatures"
          :color-by="colorBy"
          :terrain-palette="terrainPalette"
          :selected-id="selectedId"
          :basemap="basemap"
          :interactive="isInteractive"
          :snapshot="isThumbnail"
          :fit-padding="isThumbnail ? THUMBNAIL_FIT_PADDING : FARM_FIT_PADDING"
          :draw-mode="isInteractive ? drawMode : null"
          :draw-value="drawValue"
          :initial-view="initialView"
          @ready="onMapReady"
          @select="onSelect"
          @move="emit('move', $event)"
          @deselect="emit('deselect')"
          @project="onProject"
          @draw-change="drawValue = $event"
          @draw-ready="isDrawReady = $event"
          @snapshot="snapshotUrl = $event"
          @error="hasMapError = true"
        />
      </ClientOnly>
    </div>
    <div v-if="isDrawLoading" class="map-view__loading" role="status">
      <span class="map-view__loading-text text-label-chip">Carregando mapa…</span>
    </div>
    <span v-if="hasGeometry && !hasMapError" class="map-view__attribution">{{ MAP_ATTRIBUTION }}</span>
    <!-- Visual only: the accessible names of the features live in the page (e.g. the field list). -->
    <div v-if="labels.length && showLiveMap" class="map-view__labels" aria-hidden="true">
      <span
        v-for="label in labels"
        :key="label.id"
        class="map-view__label text-label-badge"
        :class="{ 'map-view__label--selected': label.isSelected }"
        :style="labelStyle(label.point)"
      >
        {{ label.text }}
      </span>
    </div>
    <div
      v-if="drawBadge && drawBadgePoint && showLiveMap"
      class="map-view__draw-badge"
      :style="labelStyle(drawBadgePoint)"
      aria-hidden="true"
    >
      <span class="map-view__draw-badge-eyebrow text-label-micro">{{ drawBadge.eyebrow }}</span>
      <span class="map-view__draw-badge-value text-metric">{{ drawBadge.value }}</span>
    </div>
    <MapControls
      v-if="controls && showLiveMap && isMapReady"
      class="map-view__controls"
      :draw-label="drawLabel"
      :draw-active="drawActive"
      @zoom-in="canvasRef?.zoomIn()"
      @zoom-out="canvasRef?.zoomOut()"
      @fit="canvasRef?.fit()"
      @draw="emit('draw')"
    />
    <div v-if="$slots.toolbar" ref="toolbarRef" class="map-view__toolbar">
      <slot name="toolbar" />
    </div>
    <div v-if="$slots.legend" ref="legendRef" class="map-view__legend">
      <slot name="legend" />
    </div>
    <div v-if="$slots.hint" class="map-view__hint">
      <slot name="hint" :description-id="hintDescriptionId" />
    </div>
    <div v-if="$slots.popover && popoverStyle" ref="popoverRef" class="map-view__popover" :style="popoverStyle">
      <slot name="popover" />
    </div>
    <div v-if="$slots.default" class="map-view__overlay">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.map-view {
  position: relative;
  width: 100%;
  margin: 0;
  overflow: hidden;
  background: var(--color-green-900);
}

.map-view--rounded {
  border-radius: var(--radius-xl);
}

.map-view__media {
  position: absolute;
  inset: 0;
}

.map-view__controls {
  position: absolute;
  top: var(--space-16);
  right: var(--space-16);
  z-index: 3;
}

.map-view__toolbar {
  position: absolute;
  top: var(--space-16);
  left: var(--space-16);
  z-index: 3;
  max-width: calc(100% - 2 * var(--space-16));
}

.map-view__legend {
  position: absolute;
  bottom: var(--space-16);
  left: var(--space-16);
  z-index: 3;
  max-width: calc(100% - 2 * var(--space-16));
}

.map-view__labels {
  position: absolute;
  inset: 0;
  z-index: 1;
  overflow: hidden;
  pointer-events: none;
}

.map-view__label {
  position: absolute;
  top: 0;
  left: 0;
  padding: var(--padding-map-label);
  border-radius: var(--radius-pill);
  background: var(--color-map-label);
  color: var(--color-text-inverse);
  white-space: nowrap;
}

.map-view__label--selected {
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.map-view__draw-badge {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: center;
  padding: var(--space-8) var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-map-badge);
  white-space: nowrap;
  pointer-events: none;
}

.map-view__draw-badge-eyebrow {
  color: var(--color-sprout-400);
  letter-spacing: 0.1em;
}

.map-view__draw-badge-value {
  color: var(--color-text-inverse);
}

/* Full width minus the gutters, content centered: the hint keeps one line when it fits. */
.map-view__loading {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: grid;
  place-items: center;
  background: var(--color-overlay);
  cursor: progress;
}

.map-view__loading-text {
  padding: var(--space-10) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-secondary);
}

.map-view__hint {
  position: absolute;
  right: var(--space-16);
  bottom: var(--space-32);
  left: var(--space-16);
  z-index: 3;
  display: flex;
  justify-content: center;
  pointer-events: none;
}

.map-view__hint > :deep(*) {
  pointer-events: auto;
}

.map-view__popover {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 4;
}

.map-view__image {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* While the live map loads, the illustration is a dimmed skeleton. */
.map-view__image--placeholder {
  opacity: 0.4;
}

.map-view__attribution {
  position: absolute;
  right: var(--space-6);
  bottom: var(--space-4);
  z-index: 1;
  max-width: calc(100% - 2 * var(--space-6));
  overflow: hidden;
  color: var(--color-text-inverse-subtle);
  font: var(--font-caption-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}

.map-view__overlay {
  position: absolute;
  inset: 0;
  z-index: 2;
  pointer-events: none;
}

.map-view__overlay > :deep(*) {
  pointer-events: auto;
}
</style>
