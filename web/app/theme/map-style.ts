import type { ExpressionSpecification, StyleSpecification } from 'maplibre-gl'
import type { GeoJsonPosition, PerimeterGeometry } from '~/types/geo'
import type { SprayCondition } from '~/types/spray'
import type { TerrainAspect } from '~/types/terrain'
import type { TerrainClassKey, TerrainMapLayer } from '~/utils/terrain'
import { palette } from './palette'

/**
 * Map style of the app (ADR 0012), generated from palette.ts so the map uses
 * the same single source of colors as the rest of the UI (ADR 0011).
 * Reproduces the dark "coffee green" background of the Figma (7:174, 15:265):
 * dark land, slightly darker water/vegetation, faint roads, no labels.
 *
 * Only type imports from maplibre-gl here: the library itself is loaded on the
 * client by components/map/.
 */

export type MapBasemap = 'vector' | 'satellite'
export type MapColorBy = 'crop' | 'status' | 'terrain'

export const MAP_ATTRIBUTION = '© OpenFreeMap © OpenMapTiles © colaboradores do OpenStreetMap'

export const SOURCE_BASEMAP = 'basemap'
export const SOURCE_SATELLITE = 'satellite'
export const SOURCE_FEATURES = 'features'
export const LAYER_FEATURES_FILL = 'features-fill'
export const LAYER_FEATURES_OUTLINE = 'features-outline'
export const LAYER_FEATURES_SELECTED = 'features-selected'
export const LAYER_FEATURES_DASHED = 'features-dashed'

/** Field fills of the Figma map, cycled per crop (legend 15:321). */
export const MAP_FIELD_COLORS: readonly string[] = [
  palette['map-field-1'],
  palette['map-field-2'],
  palette['map-field-3'],
  palette['map-field-4'],
  palette['map-field-5'],
  palette['map-field-6'],
]

/** Field fills when coloring by spray status (legend of the "Status" layer). */
export const STATUS_COLORS: Readonly<Record<SprayCondition, string>> = {
  suitable: palette['status-suitable'],
  attention: palette['status-attention'],
  unsuitable: palette['status-unsuitable'],
  rain: palette['info-rain'],
}

/** Neutral grey of a field without relief (dashed outline on the map, "Não calculado"). */
const TERRAIN_NOT_CALCULATED_COLOR = '#b9b7aa'

/** Fill per relief class on the map and in the legend (Figma 68:1067: Embrapa classes, flat to mountainous). */
export const TERRAIN_CLASS_COLORS: Readonly<Record<TerrainClassKey, string>> = {
  flat: '#9fcf55',
  gently_undulating: '#e2c84a',
  undulating: '#e8973a',
  strongly_undulating: '#d0573a',
  mountainous: '#8e2f3e',
  not_calculated: TERRAIN_NOT_CALCULATED_COLOR,
}

/**
 * Chip of the Figma component "Relevo · Selo de classe" (67:2239) and bar of
 * "Relevo · Linha do talhão" (67:2240): darker than the map fill so it reads on white.
 * The Figma does not draw "Montanhoso" nor "Não calculado" in these components; those
 * two follow the same family (darker tone of the map color, neutral for not calculated).
 */
export interface TerrainClassTone {
  readonly bar: string
  readonly badgeBackground: string
  readonly badgeText: string
}

export const TERRAIN_CLASS_TONES: Readonly<Record<TerrainClassKey, TerrainClassTone>> = {
  flat: { bar: '#6f9b2e', badgeBackground: '#eaf4d6', badgeText: '#335020' },
  gently_undulating: { bar: '#c9a227', badgeBackground: '#f8f0d2', badgeText: '#6b5410' },
  undulating: { bar: '#e08a2e', badgeBackground: '#fbe8d3', badgeText: '#7a4310' },
  strongly_undulating: { bar: '#c8492f', badgeBackground: '#f9e0da', badgeText: '#7a2414' },
  mountainous: { bar: '#8e2f3e', badgeBackground: '#f2dade', badgeText: '#5e1a28' },
  not_calculated: { bar: TERRAIN_NOT_CALCULATED_COLOR, badgeBackground: palette.sand, badgeText: palette['text-secondary'] },
}

/** Sequential scale of the "Altitude" layer, index = band (0 = lowest). */
export const TERRAIN_ELEVATION_COLORS: readonly string[] = ['#dfe6b8', '#b9cf8e', '#86b072', '#4f8a64', '#2c5f58']

/** "Exposição" layer: one hue per sector, neutral for flat ground, no aspect and not calculated. */
export const TERRAIN_ASPECT_COLORS: Readonly<Record<TerrainAspect | 'no_aspect' | 'not_calculated', string>> = {
  north: '#4f7fd1',
  northeast: '#4fb3c9',
  east: '#6dbf7a',
  southeast: '#c3d255',
  south: '#e8b83a',
  southwest: '#e8823a',
  west: '#d0573a',
  northwest: '#a768b8',
  flat: '#cfcdc0',
  no_aspect: '#e4e2d8',
  not_calculated: TERRAIN_NOT_CALCULATED_COLOR,
}

/**
 * Color per key of the layer: what MapView (`terrainPalette`) and MapLegend consume.
 * Elevation keys are `elevation-<band>`, plus `not_calculated`.
 */
export function terrainPalette(layer: TerrainMapLayer): Readonly<Record<string, string>> {
  if (layer === 'slope') return TERRAIN_CLASS_COLORS
  if (layer === 'aspect') return TERRAIN_ASPECT_COLORS
  const bands = TERRAIN_ELEVATION_COLORS.map((color, index) => [`elevation-${index}`, color] as const)
  return { ...Object.fromEntries(bands), not_calculated: TERRAIN_NOT_CALCULATED_COLOR }
}

/** Fill of a terrain key that has no entry in the palette. */
export const TERRAIN_FALLBACK_COLOR = TERRAIN_NOT_CALCULATED_COLOR

interface StyleOptions {
  tilesUrl: string
  satelliteUrl: string
  basemap: MapBasemap
}

/** Base style: vector basemap (or satellite raster) without any data layer. */
export function createMapStyle({ tilesUrl, satelliteUrl, basemap }: StyleOptions): StyleSpecification {
  const showSatellite = basemap === 'satellite' && Boolean(satelliteUrl)
  return {
    version: 8,
    sources: {
      [SOURCE_BASEMAP]: { type: 'vector', url: tilesUrl },
      ...(satelliteUrl
        ? { [SOURCE_SATELLITE]: { type: 'raster', tiles: [satelliteUrl], tileSize: 256 } }
        : {}),
    },
    layers: [
      { id: 'land', type: 'background', paint: { 'background-color': palette['map-land'] } },
      {
        id: 'vegetation',
        type: 'fill',
        source: SOURCE_BASEMAP,
        'source-layer': 'landcover',
        filter: ['in', ['get', 'class'], ['literal', ['wood', 'forest', 'grass', 'wetland']]],
        paint: { 'fill-color': palette['map-land-shade'], 'fill-opacity': 0.7 },
      },
      {
        id: 'water',
        type: 'fill',
        source: SOURCE_BASEMAP,
        'source-layer': 'water',
        paint: { 'fill-color': palette['map-land-shade'] },
      },
      {
        id: 'roads',
        type: 'line',
        source: SOURCE_BASEMAP,
        'source-layer': 'transportation',
        filter: ['in', ['get', 'class'], ['literal', ['motorway', 'trunk', 'primary', 'secondary', 'tertiary', 'minor', 'track']]],
        paint: {
          'line-color': palette['map-road'],
          'line-opacity': 0.35,
          'line-width': ['interpolate', ['linear'], ['zoom'], 8, 0.5, 14, 2, 18, 4],
        },
      },
      ...(showSatellite
        ? [{ id: 'satellite', type: 'raster' as const, source: SOURCE_SATELLITE }]
        : []),
    ],
  }
}

/**
 * Fill color per feature: by spray status (from the API), by terrain key through the
 * given palette (`terrainPalette(layer)`), or by crop index.
 */
export function featureFillColor(colorBy: MapColorBy, terrainColors: Readonly<Record<string, string>> = {}): ExpressionSpecification {
  if (colorBy === 'terrain') {
    // One lookup in the palette object (a `match` would need a typed, non-empty label list).
    return ['to-color', ['coalesce', ['get', ['to-string', ['coalesce', ['get', 'terrainKey'], '']], ['literal', { ...terrainColors }]], TERRAIN_FALLBACK_COLOR]]
  }
  if (colorBy === 'status') {
    return [
      'match',
      ['get', 'status'],
      'suitable', STATUS_COLORS.suitable,
      'attention', STATUS_COLORS.attention,
      'unsuitable', STATUS_COLORS.unsuitable,
      'rain', STATUS_COLORS.rain,
      palette['map-field-1'],
    ]
  }
  // `literal` keeps the palette as plain strings; `to-color` turns the picked one into a color.
  return [
    'to-color',
    ['at', ['%', ['coalesce', ['get', 'cropIndex'], 0], MAP_FIELD_COLORS.length], ['literal', [...MAP_FIELD_COLORS]]],
  ]
}

export const FEATURE_OUTLINE_COLOR = palette['green-100']
export const FEATURE_SELECTED_COLOR = palette['sprout-400']

/** Polygon styling of the Figma map (15:265): fills, 1.2 px light outline, 4 px selection. */
export const FEATURE_FILL_OPACITY = 0.9
export const FEATURE_OUTLINE_OPACITY = 0.5
export const FEATURE_OUTLINE_WIDTH = 1.2
export const FEATURE_SELECTED_WIDTH = 4
/** Outline of a field without relief: dashed and a bit stronger than the plain outline so the pattern reads. */
export const FEATURE_DASHED_WIDTH = 2
export const FEATURE_DASHED_PATTERN: [number, number] = [2, 2]
/** Closest zoom when fitting to geometries (avoids zooming into a single small field). */
export const FIT_MAX_ZOOM = 17

/** Fill of a crop color slot, shared by the map, the legend and the field list. */
export function fieldColor(cropIndex: number | undefined): string {
  return MAP_FIELD_COLORS[toCropIndex(cropIndex) % MAP_FIELD_COLORS.length] ?? palette['map-field-1']
}

/** Crop color slot: a non-negative integer, so the palette lookup is always valid. */
export function toCropIndex(value: number | undefined): number {
  return Number.isInteger(value) && (value ?? 0) >= 0 ? (value ?? 0) : 0
}

/** [west, south, east, north] of the given geometries, or null when empty. */
export function boundsOf(geometries: readonly PerimeterGeometry[]): [number, number, number, number] | null {
  const positions: GeoJsonPosition[] = []
  for (const geometry of geometries) {
    const polygons = geometry.type === 'Polygon' ? [geometry.coordinates] : geometry.coordinates
    for (const polygon of polygons) {
      for (const ring of polygon) positions.push(...ring)
    }
  }
  if (positions.length === 0) return null
  let [west, south] = positions[0] ?? [0, 0]
  let [east, north] = [west, south]
  for (const [lng, lat] of positions) {
    west = Math.min(west, lng)
    east = Math.max(east, lng)
    south = Math.min(south, lat)
    north = Math.max(north, lat)
  }
  return [west, south, east, north]
}

/**
 * Point where the label pill of a polygon sits: area-weighted centroid of the
 * largest outer ring (fields are close to convex), falling back to the ring
 * average for degenerate rings. Presentation geometry only.
 */
export function labelAnchorOf(geometry: PerimeterGeometry): GeoJsonPosition | null {
  const rings = geometry.type === 'Polygon' ? [geometry.coordinates[0]] : geometry.coordinates.map((polygon) => polygon[0])
  let best: { area: number, anchor: GeoJsonPosition } | null = null
  for (const ring of rings) {
    if (!ring || ring.length < 3) continue
    const candidate = ringCentroid(ring)
    if (!best || candidate.area > best.area) best = candidate
  }
  return best?.anchor ?? null
}

function ringCentroid(ring: readonly GeoJsonPosition[]): { area: number, anchor: GeoJsonPosition } {
  let doubleArea = 0
  let cx = 0
  let cy = 0
  for (let index = 0; index < ring.length - 1; index++) {
    const [x1, y1] = ring[index] ?? [0, 0]
    const [x2, y2] = ring[index + 1] ?? [0, 0]
    const cross = x1 * y2 - x2 * y1
    doubleArea += cross
    cx += (x1 + x2) * cross
    cy += (y1 + y2) * cross
  }
  if (Math.abs(doubleArea) < Number.EPSILON) {
    const count = ring.length
    const sum = ring.reduce<[number, number]>((acc, [x, y]) => [acc[0] + x, acc[1] + y], [0, 0])
    return { area: 0, anchor: [sum[0] / count, sum[1] / count] }
  }
  return { area: Math.abs(doubleArea / 2), anchor: [cx / (3 * doubleArea), cy / (3 * doubleArea)] }
}
