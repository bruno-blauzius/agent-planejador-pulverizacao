import { describe, expect, it } from 'vitest'
import type { GeoJsonPolygon } from '~/types/geo'
import { approximateAreaHa, isDrawablePolygon, verticesInside, vertexCount } from '~/utils/geoArea'

function square(lng: number, lat: number, side: number): GeoJsonPolygon {
  return { type: 'Polygon', coordinates: [[[lng, lat], [lng + side, lat], [lng + side, lat + side], [lng, lat + side], [lng, lat]]] }
}

describe('geoArea', () => {
  it('approximates the area in hectares with one decimal', () => {
    // ~0.01° × 0.01° near the equator ≈ 1.11 km × 1.11 km ≈ 123.6 ha
    expect(approximateAreaHa(square(0, 0, 0.01))).toBeCloseTo(123.6, 0)
    const multi = { type: 'MultiPolygon', coordinates: [square(0, 0, 0.01).coordinates, square(1, 0, 0.01).coordinates] } as const
    expect(approximateAreaHa(multi)).toBeCloseTo(247.2, 0)
  })

  it('subtracts holes', () => {
    const outer = square(0, 0, 0.02).coordinates[0]!
    const hole = square(0.005, 0.005, 0.01).coordinates[0]!
    expect(approximateAreaHa({ type: 'Polygon', coordinates: [outer, hole] })).toBeCloseTo(494.5 - 123.6, 0)
  })

  it('counts vertices without the closing point', () => {
    expect(vertexCount(null)).toBe(0)
    expect(vertexCount(square(0, 0, 1))).toBe(4)
    expect(vertexCount({ type: 'Polygon', coordinates: [[[0, 0], [1, 0], [1, 1]]] })).toBe(3)
    expect(isDrawablePolygon({ type: 'Polygon', coordinates: [[[0, 0], [1, 0], [0, 0]]] })).toBe(false)
    expect(isDrawablePolygon(square(0, 0, 1))).toBe(true)
  })

  it('checks vertices against the perimeter, honoring holes and multipolygons', () => {
    const perimeter = square(0, 0, 10)
    expect(verticesInside(square(1, 1, 2), perimeter)).toBe(true)
    expect(verticesInside(square(9, 9, 2), perimeter)).toBe(false)
    const withHole: GeoJsonPolygon = { type: 'Polygon', coordinates: [perimeter.coordinates[0]!, square(4, 4, 2).coordinates[0]!] }
    expect(verticesInside(square(4.5, 4.5, 1), withHole)).toBe(false)
    const multi = { type: 'MultiPolygon', coordinates: [perimeter.coordinates, square(20, 0, 5).coordinates] } as const
    expect(verticesInside(square(21, 1, 1), multi)).toBe(true)
  })
})
