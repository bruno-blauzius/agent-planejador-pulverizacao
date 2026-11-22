/**
 * Rough planar geometry used ONLY by the in-memory adapter to return plausible
 * areas. The real computation belongs to services/api (ADR 0010); never import
 * this outside `services/mock/`.
 */
import type { GeoJsonPosition, GeoPoint, PerimeterGeometry } from '~/types/geo'

const METERS_PER_DEGREE = 111_320
const SQUARE_METERS_PER_HECTARE = 10_000

function ringAreaSquareMeters(ring: readonly GeoJsonPosition[]): number {
  if (ring.length < 4) return 0
  const meanLat = ring.reduce((sum, [, lat]) => sum + lat, 0) / ring.length
  const lngScale = METERS_PER_DEGREE * Math.cos((meanLat * Math.PI) / 180)
  let doubled = 0
  for (let index = 0; index < ring.length - 1; index++) {
    const [lng1, lat1] = ring[index]!
    const [lng2, lat2] = ring[index + 1]!
    doubled += lng1 * lngScale * (lat2 * METERS_PER_DEGREE) - lng2 * lngScale * (lat1 * METERS_PER_DEGREE)
  }
  return Math.abs(doubled) / 2
}

function polygonsOf(geometry: PerimeterGeometry): readonly (readonly (readonly GeoJsonPosition[])[])[] {
  return geometry.type === 'Polygon' ? [geometry.coordinates] : geometry.coordinates
}

/** Outer ring minus holes, rounded to 0.1 ha. */
export function approximateAreaHa(geometry: PerimeterGeometry): number {
  const squareMeters = polygonsOf(geometry).reduce((total, [outer = [], ...holes]) => {
    const holesArea = holes.reduce((sum, hole) => sum + ringAreaSquareMeters(hole), 0)
    return total + ringAreaSquareMeters(outer) - holesArea
  }, 0)
  return Math.round((squareMeters / SQUARE_METERS_PER_HECTARE) * 10) / 10
}

/** Mean of the outer ring vertices (good enough for a mock forecast point). */
export function approximateCentroid(geometry: PerimeterGeometry): GeoPoint {
  const outer = polygonsOf(geometry)[0]?.[0] ?? []
  const vertices = outer.slice(0, -1)
  if (!vertices.length) return { lat: 0, lng: 0 }
  const lng = vertices.reduce((sum, [x]) => sum + x, 0) / vertices.length
  const lat = vertices.reduce((sum, [, y]) => sum + y, 0) / vertices.length
  return { lat: Math.round(lat * 100) / 100, lng: Math.round(lng * 100) / 100 }
}
