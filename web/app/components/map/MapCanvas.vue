<script setup lang="ts">
import type { GeoJSONSource, Map as MapLibreMap, MapLayerMouseEvent } from 'maplibre-gl'
import {
  boundsOf,
  createMapStyle,
  FEATURE_FILL_OPACITY,
  FEATURE_OUTLINE_COLOR,
  FEATURE_OUTLINE_OPACITY,
  FEATURE_OUTLINE_WIDTH,
  FEATURE_SELECTED_COLOR,
  FEATURE_SELECTED_WIDTH,
  FIT_MAX_ZOOM,
  featureFillColor,
  labelAnchorOf,
  LAYER_FEATURES_FILL,
  LAYER_FEATURES_OUTLINE,
  LAYER_FEATURES_SELECTED,
  SOURCE_FEATURES,
  toCropIndex,
} from '~/theme/map-style'
import type { MapBasemap, MapColorBy } from '~/theme/map-style'
import type { GeoJsonPosition, GeoPoint, PerimeterGeometry } from '~/types/geo'
import type { MapBounds, MapFeature, MapScreenPoint } from './MapView.vue'
import { DRAW_ANCHOR_ID } from './MapView.vue'
import type { DrawController, MapDrawMode, MapDrawValue } from './mapDraw'

type GeoJsonData = Parameters<GeoJSONSource['setData']>[0]

/** Project geometries are deeply readonly; MapLibre expects mutable GeoJSON arrays. */
function toMutableGeometry(geometry: PerimeterGeometry) {
  if (geometry.type === 'Polygon') {
    return { type: 'Polygon' as const, coordinates: geometry.coordinates.map((ring) => ring.map(([lng, lat]) => [lng, lat])) }
  }
  return {
    type: 'MultiPolygon' as const,
    coordinates: geometry.coordinates.map((polygon) => polygon.map((ring) => ring.map(([lng, lat]) => [lng, lat]))),
  }
}

/**
 * The only place that talks to maplibre-gl (ADR 0012). Rendered inside
 * <ClientOnly> by MapView; the library and its CSS are imported dynamically
 * so they stay out of SSR and of pages without a map.
 */
const props = defineProps<{
  features: readonly MapFeature[]
  colorBy: MapColorBy
  selectedId: string | null
  basemap: MapBasemap
  interactive: boolean
  /** Render once, replace the canvas with an image and free the WebGL context. */
  snapshot: boolean
  /** Inner padding (px) when fitting the map to the features. */
  fitPadding: number
  /** Drawing tool (mapDraw.ts); null keeps the map read-only. */
  drawMode: MapDrawMode | null
  drawValue: MapDrawValue | null
  /** Start view when there is no geometry to fit (e.g. drawing a new perimeter). */
  initialView: { center: GeoPoint, zoom: number } | null
}>()

const emit = defineEmits<{
  ready: []
  select: [id: string]
  move: [bounds: MapBounds]
  snapshot: [dataUrl: string]
  error: []
  /** Background click (no feature under the pointer). */
  deselect: []
  /** Screen position (px, relative to the map) of each feature anchor, on every frame that moves the map. */
  project: [points: Readonly<Record<string, MapScreenPoint>>]
  /** A finished stroke or drag of the drawing tool. */
  'draw-change': [value: MapDrawValue | null]
  /** Whether the drawing tool accepts input (false while terra-draw loads or after a style swap). */
  'draw-ready': [isReady: boolean]
}>()

/** Margin (px) inside which a selected feature counts as visible. */
const SELECTION_VISIBLE_MARGIN = 48

const config = useRuntimeConfig()
const containerRef = ref<HTMLElement | null>(null)
let map: MapLibreMap | null = null
let projectFrame: number | null = null
let drawController: DrawController | null = null
/** Last value seen by the drawing tool (from the parent or from a finished stroke). */
let latestDrawValue: MapDrawValue | null = props.drawValue

const anchors = computed(() => {
  const result = new Map<string, GeoJsonPosition>()
  for (const feature of props.features) {
    const anchor = labelAnchorOf(feature.geometry)
    if (anchor) result.set(feature.id, anchor)
  }
  const drawn = props.drawValue
  if (drawn && 'type' in drawn) {
    const anchor = labelAnchorOf(drawn)
    if (anchor) result.set(DRAW_ANCHOR_ID, anchor)
  }
  return result
})

const featureCollection = computed<GeoJsonData>(() => ({
  type: 'FeatureCollection' as const,
  features: props.features.map((feature) => ({
    type: 'Feature' as const,
    id: feature.id,
    geometry: toMutableGeometry(feature.geometry),
    properties: {
      id: feature.id,
      status: feature.status ?? null,
      cropIndex: toCropIndex(feature.cropIndex),
      outlineOnly: feature.outlineOnly === true,
    },
  })),
}))

function styleOptions() {
  return {
    tilesUrl: String(config.public.mapTilesUrl),
    satelliteUrl: String(config.public.mapSatelliteUrl ?? ''),
    basemap: props.basemap,
  }
}

function addDataLayers(target: MapLibreMap) {
  // A style swap may fire more than once; the data layers are added only once per style.
  if (target.getSource(SOURCE_FEATURES)) return
  target.addSource(SOURCE_FEATURES, { type: 'geojson', data: featureCollection.value })
  target.addLayer({
    id: LAYER_FEATURES_FILL,
    type: 'fill',
    source: SOURCE_FEATURES,
    paint: {
      'fill-color': featureFillColor(props.colorBy),
      'fill-opacity': ['case', ['get', 'outlineOnly'], 0, FEATURE_FILL_OPACITY],
    },
  })
  target.addLayer({
    id: LAYER_FEATURES_OUTLINE,
    type: 'line',
    source: SOURCE_FEATURES,
    paint: { 'line-color': FEATURE_OUTLINE_COLOR, 'line-opacity': FEATURE_OUTLINE_OPACITY, 'line-width': FEATURE_OUTLINE_WIDTH },
  })
  target.addLayer({
    id: LAYER_FEATURES_SELECTED,
    type: 'line',
    source: SOURCE_FEATURES,
    filter: ['==', ['get', 'id'], props.selectedId ?? ''],
    paint: { 'line-color': FEATURE_SELECTED_COLOR, 'line-width': FEATURE_SELECTED_WIDTH },
  })
}

function fitToFeatures(animate: boolean) {
  const drawn = props.drawValue && 'type' in props.drawValue ? [props.drawValue] : []
  // The drawing counts too: an edited field must be in view even outside the other features.
  const geometries = [...props.features.map((feature) => feature.geometry), ...drawn]
  const bounds = boundsOf(geometries)
  if (!map || !bounds) return
  map.fitBounds(bounds, { padding: props.fitPadding, animate, maxZoom: FIT_MAX_ZOOM })
}

function emitBounds() {
  if (!map) return
  const bounds = map.getBounds()
  emit('move', { west: bounds.getWest(), south: bounds.getSouth(), east: bounds.getEast(), north: bounds.getNorth() })
}

/** Ids of the selectable features under a point, topmost first (outline-only ones are not selectable). */
function selectableIdsAt(event: MapLayerMouseEvent): string[] {
  if (!map) return []
  return map
    .queryRenderedFeatures(event.point, { layers: [LAYER_FEATURES_FILL] })
    .filter((feature) => feature.properties?.outlineOnly !== true)
    .map((feature) => feature.properties?.id)
    .filter((id): id is string => typeof id === 'string')
}

function onMapClick(event: MapLayerMouseEvent) {
  const [id] = selectableIdsAt(event)
  if (id) emit('select', id)
  else emit('deselect')
}

function onPointerMove(event: MapLayerMouseEvent) {
  if (map) map.getCanvas().style.cursor = selectableIdsAt(event).length ? 'pointer' : ''
}

function emitProjection() {
  projectFrame = null
  if (!map) return
  const points: Record<string, MapScreenPoint> = {}
  for (const [id, [lng, lat]] of anchors.value) {
    const { x, y } = map.project([lng, lat])
    points[id] = { x, y }
  }
  emit('project', points)
}

/** Coalesces the many `move` events of an animation into one update per frame. */
function scheduleProjection() {
  if (projectFrame !== null) return
  projectFrame = requestAnimationFrame(emitProjection)
}

/** Brings the selected feature into view when it is outside the visible area (e.g. picked from the list). */
function revealSelected(id: string | null) {
  const anchor = id ? anchors.value.get(id) : undefined
  if (!map || !anchor) return
  const { x, y } = map.project([anchor[0], anchor[1]])
  const canvas = map.getCanvas()
  const isVisible = x >= SELECTION_VISIBLE_MARGIN
    && y >= SELECTION_VISIBLE_MARGIN
    && x <= canvas.clientWidth - SELECTION_VISIBLE_MARGIN
    && y <= canvas.clientHeight - SELECTION_VISIBLE_MARGIN
  if (!isVisible) map.easeTo({ center: [anchor[0], anchor[1]] })
}

function takeSnapshot(target: MapLibreMap) {
  emit('snapshot', target.getCanvas().toDataURL('image/png'))
  target.remove()
  map = null
}

async function createMap() {
  if (!containerRef.value) return
  const [{ Map: MapLibre, setWorkerUrl }, { default: workerUrl }] = await Promise.all([
    import('maplibre-gl'),
    // The worker ships as a separate module; Vite's dependency pre-bundling breaks
    // MapLibre's relative worker URL, so it is resolved here as an asset URL.
    import('maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url'),
    import('maplibre-gl/dist/maplibre-gl.css'),
  ])
  setWorkerUrl(workerUrl)
  const instance = new MapLibre({
    container: containerRef.value,
    style: createMapStyle(styleOptions()),
    interactive: props.interactive,
    ...(props.initialView
      ? { center: [props.initialView.center.lng, props.initialView.center.lat] as [number, number], zoom: props.initialView.zoom }
      : {}),
    attributionControl: false,
    // Needed to read the canvas back for thumbnails.
    canvasContextAttributes: { preserveDrawingBuffer: props.snapshot },
    fadeDuration: 0,
  })
  map = instance
  let isLoaded = false
  // Before `load` an error means the map cannot render (style, WebGL): fall back
  // to the illustration. Later errors (a missing tile) are not fatal.
  instance.on('error', (event) => {
    if (import.meta.dev) console.warn('[MapCanvas]', event.error)
    if (!isLoaded) emit('error')
  })
  instance.on('load', () => {
    isLoaded = true
    addDataLayers(instance)
    fitToFeatures(false)
    emit('ready')
    if (props.snapshot) {
      instance.once('idle', () => takeSnapshot(instance))
      return
    }
    if (props.interactive) {
      instance.on('mousemove', onPointerMove)
      instance.on('moveend', emitBounds)
      instance.on('click', onMapClick)
      instance.on('move', scheduleProjection)
      instance.on('resize', scheduleProjection)
      emitProjection()
      setupDraw(instance)
    }
  })
}

/** Zoom controls for MapControls (the default MapLibre controls are not used). */
function zoomBy(delta: number) {
  map?.easeTo({ zoom: map.getZoom() + delta })
}

defineExpose({ zoomIn: () => zoomBy(1), zoomOut: () => zoomBy(-1), fit: () => fitToFeatures(true) })

watch(featureCollection, (data) => {
  const source = map?.getSource<GeoJSONSource>(SOURCE_FEATURES)
  source?.setData(data)
  fitToFeatures(true)
  scheduleProjection()
})

// Before the style loads (or during a basemap swap) the data layers do not exist yet;
// addDataLayers reads the current props when it creates them.
watch(() => props.colorBy, (colorBy) => {
  if (map?.getLayer(LAYER_FEATURES_FILL)) map.setPaintProperty(LAYER_FEATURES_FILL, 'fill-color', featureFillColor(colorBy))
})

watch(() => props.selectedId, (id) => {
  if (!map?.getLayer(LAYER_FEATURES_SELECTED)) return
  map.setFilter(LAYER_FEATURES_SELECTED, ['==', ['get', 'id'], id ?? ''])
  revealSelected(id)
})

/** Starts (or restarts, after a style swap) the drawing tool with the latest value. */
function setupDraw(target: MapLibreMap) {
  const mode = props.drawMode
  if (!mode) return
  void import('./mapDraw')
    .then(({ createDrawController }) => createDrawController({
      map: target,
      mode,
      initialValue: latestDrawValue,
      onChange: (value) => {
        latestDrawValue = value
        emit('draw-change', value)
      },
    }))
    .then((controller) => {
      // The map may have been removed while terra-draw was loading.
      if (map !== target) {
        controller.destroy()
        return
      }
      drawController = controller
      controller.setValue(latestDrawValue)
      emit('draw-ready', true)
    })
    .catch((error: unknown) => {
      if (import.meta.dev) console.warn('[MapCanvas] draw', error)
    })
}

function teardownDraw() {
  drawController?.destroy()
  drawController = null
  emit('draw-ready', false)
}

/**
 * The start view changed after mounting (e.g. coordinates typed outside the
 * map): move there only when the new center is out of view, so a point placed
 * by clicking the map itself never recenters it.
 */
watch(
  () => (props.initialView ? [props.initialView.center.lng, props.initialView.center.lat, props.initialView.zoom] as const : null),
  (view) => {
    if (!map || !view) return
    const [lng, lat, zoom] = view
    const { x, y } = map.project([lng, lat])
    const canvas = map.getCanvas()
    const isVisible = x >= SELECTION_VISIBLE_MARGIN
      && y >= SELECTION_VISIBLE_MARGIN
      && x <= canvas.clientWidth - SELECTION_VISIBLE_MARGIN
      && y <= canvas.clientHeight - SELECTION_VISIBLE_MARGIN
    if (!isVisible) map.easeTo({ center: [lng, lat], zoom: Math.max(map.getZoom(), zoom) })
  },
)

watch(() => props.drawValue, (value) => {
  latestDrawValue = value
  drawController?.setValue(value)
  scheduleProjection()
})

watch(() => props.drawMode, () => {
  teardownDraw()
  if (map?.loaded()) setupDraw(map)
})

watch(() => props.basemap, () => {
  if (!map) return
  const current = map
  teardownDraw()
  current.setStyle(createMapStyle(styleOptions()))
  current.once('style.load', () => {
    addDataLayers(current)
    setupDraw(current)
  })
})

onMounted(() => {
  createMap().catch((error: unknown) => {
    if (import.meta.dev) console.warn('[MapCanvas]', error)
    emit('error')
  })
})

onBeforeUnmount(() => {
  if (projectFrame !== null) cancelAnimationFrame(projectFrame)
  teardownDraw()
  map?.remove()
  map = null
})
</script>

<template>
  <div ref="containerRef" class="map-canvas" />
</template>

<style scoped>
.map-canvas {
  position: absolute;
  inset: 0;
}
</style>
