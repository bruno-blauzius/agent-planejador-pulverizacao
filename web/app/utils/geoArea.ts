/**
 * Geometry helpers for presentation only (ADR 0012). The official area of a
 * field or farm is always the one returned by the API; these values preview
 * what is being drawn and must be labeled as approximate.
 */
import turfArea from '@turf/area'
import type { GeoJsonMultiPolygon, GeoJsonPolygon, GeoJsonPosition, PerimeterGeometry } from '~/types/geo'

const SQUARE_METERS_PER_HECTARE = 10_000

type Ring = readonly GeoJsonPosition[]

function toMutableRings(rings: readonly Ring[]): number[][][] {
  return rings.map(ring => ring.map(([lng, lat]) => [lng, lat]))
}

/** Approximate area in hectares (one decimal), e.g. 41.2. */
export function approximateAreaHa(geometry: GeoJsonPolygon | GeoJsonMultiPolygon): number {
  const squareMeters = geometry.type === 'Polygon'
    ? turfArea({ type: 'Polygon', coordinates: toMutableRings(geometry.coordinates) })
    : turfArea({ type: 'MultiPolygon', coordinates: geometry.coordinates.map(toMutableRings) })
  return Math.round((squareMeters / SQUARE_METERS_PER_HECTARE) * 10) / 10
}

/** Vertices of the outer ring, without the closing point repeated. */
export function vertexCount(polygon: GeoJsonPolygon | null): number {
  const outer = polygon?.coordinates[0] ?? []
  if (outer.length === 0) return 0
  const [first, last] = [outer[0]!, outer[outer.length - 1]!]
  const closed = outer.length > 1 && first[0] === last[0] && first[1] === last[1]
  return closed ? outer.length - 1 : outer.length
}

/** A polygon that can be saved: at least three distinct vertices. */
export function isDrawablePolygon(polygon: GeoJsonPolygon | null): polygon is GeoJsonPolygon {
  return vertexCount(polygon) >= 3
}

/** Ray casting on one ring. Points on the edge may fall either way (fine for a warning). */
function insideRing([x, y]: GeoJsonPosition, ring: Ring): boolean {
  let inside = false
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i]!
    const [xj, yj] = ring[j]!
    if ((yi > y) !== (yj > y) && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) inside = !inside
  }
  return inside
}

function insidePolygon(point: GeoJsonPosition, rings: readonly Ring[]): boolean {
  const [outer, ...holes] = rings
  return !!outer && insideRing(point, outer) && !holes.some(hole => insideRing(point, hole))
}

/**
 * Whether every vertex lies inside the perimeter. A simple check for a UI
 * warning: an edge crossing a concave perimeter is not detected (the API
 * validates the geometry).
 */
export function verticesInside(polygon: GeoJsonPolygon, perimeter: PerimeterGeometry): boolean {
  const polygons = perimeter.type === 'Polygon' ? [perimeter.coordinates] : perimeter.coordinates
  return (polygon.coordinates[0] ?? []).every(point => polygons.some(rings => insidePolygon(point, rings)))
}
