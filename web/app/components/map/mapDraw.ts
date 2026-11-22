/**
 * Drawing on the map with Terra Draw (ADR 0012). Only MapCanvas uses this
 * module, and only on the client: terra-draw and its MapLibre adapter are
 * imported dynamically, so they stay out of SSR and of maps that do not draw.
 *
 * Contract with the caller:
 * - `polygon`: click the vertices and close on the first one; afterwards the
 *   polygon stays selected to drag vertices, insert one by dragging a midpoint
 *   or delete one (right click / long press, Terra Draw default).
 * - `point`: each click places the point (replacing the previous one).
 * - `onChange` fires only when a stroke or a drag FINISHES (never on every
 *   pointer move), so each call is one undo step for the caller.
 * - `setValue` replaces the drawing from outside (initial value, undo, clear)
 *   without calling `onChange`.
 */
import type { Map as MapLibreMap } from 'maplibre-gl'
import type { GeoJsonPolygon, GeoPoint } from '~/types/geo'
import { palette } from '~/theme/palette'

export type MapDrawMode = 'polygon' | 'point'
/** Polygon in `polygon` mode, point in `point` mode; null when nothing is drawn. */
export type MapDrawValue = GeoJsonPolygon | GeoPoint

export interface DrawController {
  setValue: (value: MapDrawValue | null) => void
  destroy: () => void
}

interface DrawOptions {
  map: MapLibreMap
  mode: MapDrawMode
  initialValue: MapDrawValue | null
  onChange: (value: MapDrawValue | null) => void
}

type HexColor = `#${string}`

/** Terra Draw styles need `#rrggbb`: taken from the palette (single color source). */
function hex(color: string): HexColor {
  return color as HexColor
}

/** Terra Draw validates coordinate precision (9 decimals by default). */
const COORDINATE_DECIMALS = 9
const SELECT_MODE = 'select'

function round(value: number): number {
  const factor = 10 ** COORDINATE_DECIMALS
  return Math.round(value * factor) / factor
}

/** Figma 16:396: sprout outline, translucent sprout fill, white vertices. */
const DRAW_STYLE = {
  fill: hex(palette['sprout-400']),
  outline: hex(palette['sprout-400']),
  vertex: hex(palette.white),
  vertexOutline: hex(palette['green-950']),
  fillOpacity: 0.25,
  outlineWidth: 2.5,
  vertexWidth: 5,
  vertexOutlineWidth: 1,
  /** Midpoints insert a vertex when dragged: large enough to be found. */
  midpointWidth: 4.5,
  midpointOutlineWidth: 1.5,
  pointWidth: 7,
  pointOutlineWidth: 2,
} as const

export function isPolygonValue(value: MapDrawValue | null): value is GeoJsonPolygon {
  return value !== null && 'type' in value && value.type === 'Polygon'
}

function toFeature(value: MapDrawValue, mode: MapDrawMode) {
  if (isPolygonValue(value)) {
    return {
      type: 'Feature' as const,
      geometry: {
        type: 'Polygon' as const,
        coordinates: value.coordinates.map((ring) => ring.map(([lng, lat]) => [round(lng), round(lat)])),
      },
      properties: { mode },
    }
  }
  return {
    type: 'Feature' as const,
    geometry: { type: 'Point' as const, coordinates: [round(value.lng), round(value.lat)] },
    properties: { mode },
  }
}

/** Reads a store feature back as the project's value types (plain, immutable-friendly objects). */
function fromGeometry(geometry: { type: string, coordinates: unknown }): MapDrawValue | null {
  if (geometry.type === 'Polygon' && Array.isArray(geometry.coordinates)) {
    const rings = geometry.coordinates as number[][][]
    return { type: 'Polygon', coordinates: rings.map((ring) => ring.map(([lng = 0, lat = 0]) => [lng, lat] as const)) }
  }
  if (geometry.type === 'Point' && Array.isArray(geometry.coordinates)) {
    const [lng = 0, lat = 0] = geometry.coordinates as number[]
    return { lat, lng }
  }
  return null
}

function sameValue(a: MapDrawValue | null, b: MapDrawValue | null): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

export async function createDrawController({ map, mode, initialValue, onChange }: DrawOptions): Promise<DrawController> {
  const [{ TerraDraw, TerraDrawPolygonMode, TerraDrawPointMode, TerraDrawSelectMode }, { TerraDrawMapLibreGLAdapter }] = await Promise.all([
    import('terra-draw'),
    import('terra-draw-maplibre-gl-adapter'),
  ])

  const draw = new TerraDraw({
    adapter: new TerraDrawMapLibreGLAdapter({ map }),
    modes: [
      new TerraDrawPolygonMode({
        styles: {
          fillColor: DRAW_STYLE.fill,
          fillOpacity: DRAW_STYLE.fillOpacity,
          outlineColor: DRAW_STYLE.outline,
          outlineWidth: DRAW_STYLE.outlineWidth,
          closingPointColor: DRAW_STYLE.vertex,
          closingPointWidth: DRAW_STYLE.vertexWidth,
          closingPointOutlineColor: DRAW_STYLE.vertexOutline,
          closingPointOutlineWidth: DRAW_STYLE.vertexOutlineWidth,
        },
      }),
      new TerraDrawPointMode({
        styles: {
          pointColor: DRAW_STYLE.outline,
          pointWidth: DRAW_STYLE.pointWidth,
          pointOutlineColor: DRAW_STYLE.vertex,
          pointOutlineWidth: DRAW_STYLE.pointOutlineWidth,
        },
      }),
      new TerraDrawSelectMode({
        flags: {
          polygon: {
            feature: {
              draggable: false,
              coordinates: { draggable: true, midpoints: true, deletable: true },
            },
          },
        },
        styles: {
          selectedPolygonColor: DRAW_STYLE.fill,
          selectedPolygonFillOpacity: DRAW_STYLE.fillOpacity,
          selectedPolygonOutlineColor: DRAW_STYLE.outline,
          selectedPolygonOutlineWidth: DRAW_STYLE.outlineWidth,
          selectionPointColor: DRAW_STYLE.vertex,
          selectionPointWidth: DRAW_STYLE.vertexWidth,
          selectionPointOutlineColor: DRAW_STYLE.vertexOutline,
          selectionPointOutlineWidth: DRAW_STYLE.vertexOutlineWidth,
          midPointColor: DRAW_STYLE.outline,
          midPointWidth: DRAW_STYLE.midpointWidth,
          midPointOutlineColor: DRAW_STYLE.vertex,
          midPointOutlineWidth: DRAW_STYLE.midpointOutlineWidth,
        },
      }),
    ],
  })

  let current: MapDrawValue | null = null
  // Ids are generated by Terra Draw; there is at most one drawn feature.
  let currentId: string | number | null = null
  // Store changes made by render() itself are not user edits.
  let isRendering = false

  function readFeature(id: string | number): MapDrawValue | null {
    const feature = draw.getSnapshotFeature(id)
    return feature ? fromGeometry(feature.geometry) : null
  }

  /** Polygon: editing (select) once drawn, drawing otherwise. Point: always placing. */
  function enterIdleMode() {
    if (mode === 'polygon' && currentId !== null) {
      draw.setMode(SELECT_MODE)
      draw.selectFeature(currentId)
      return
    }
    draw.setMode(mode)
  }

  function render(value: MapDrawValue | null) {
    isRendering = true
    draw.setMode(mode)
    draw.clear()
    currentId = null
    current = value
    if (value) {
      const [result] = draw.addFeatures([toFeature(value, mode)])
      if (result?.valid) currentId = result.id ?? null
      else if (import.meta.dev) console.warn('[mapDraw] invalid geometry', result?.reason)
    }
    enterIdleMode()
    isRendering = false
  }

  draw.on('finish', (id) => {
    if (mode === 'point') {
      // A new click replaces the previous point.
      const stale = draw.getSnapshot().filter((feature) => feature.id !== id).map((feature) => feature.id)
      if (stale.length) draw.removeFeatures(stale.filter((featureId): featureId is string | number => featureId !== undefined))
    }
    currentId = id
    const value = readFeature(id)
    if (sameValue(value, current)) return
    current = value
    onChange(value)
    if (mode === 'polygon') enterIdleMode()
  })

  // Deleting vertices below a valid polygon removes the feature: report it as cleared.
  draw.on('change', (_ids, type) => {
    if (isRendering || type !== 'delete' || currentId === null || draw.getSnapshotFeature(currentId)) return
    currentId = null
    current = null
    onChange(null)
    enterIdleMode()
  })

  // A click outside deselects in Terra Draw; the drawn polygon stays editable instead.
  draw.on('deselect', () => {
    if (isRendering || mode !== 'polygon' || currentId === null) return
    const id = currentId
    queueMicrotask(() => {
      if (currentId === id && draw.getSnapshotFeature(id)) draw.selectFeature(id)
    })
  })

  await new Promise<void>((resolve) => {
    draw.on('ready', () => resolve())
    draw.start()
  })
  render(initialValue)

  return {
    setValue(value) {
      if (sameValue(value, current)) return
      render(value)
    },
    destroy() {
      draw.stop()
    },
  }
}
