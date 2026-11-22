/** WGS84 point. */
export interface GeoPoint {
  readonly lat: number
  readonly lng: number
}

/** GeoJSON position, `[lng, lat]` (RFC 7946). */
export type GeoJsonPosition = readonly [number, number]

export interface GeoJsonPolygon {
  readonly type: 'Polygon'
  readonly coordinates: readonly (readonly GeoJsonPosition[])[]
}

export interface GeoJsonMultiPolygon {
  readonly type: 'MultiPolygon'
  readonly coordinates: readonly (readonly (readonly GeoJsonPosition[])[])[]
}

export type PerimeterGeometry = GeoJsonPolygon | GeoJsonMultiPolygon
