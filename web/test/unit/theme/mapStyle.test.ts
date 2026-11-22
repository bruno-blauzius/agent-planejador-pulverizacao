import { describe, expect, it } from 'vitest'
import {
  boundsOf,
  createMapStyle,
  featureFillColor,
  MAP_FIELD_COLORS,
  SOURCE_BASEMAP,
  SOURCE_SATELLITE,
} from '~/theme/map-style'
import { palette } from '~/theme/palette'
import type { GeoJsonMultiPolygon, GeoJsonPolygon } from '~/types/geo'

/**
 * Tiny evaluator for the few expression operators used by `featureFillColor`
 * (`match`, `get`, `coalesce`, `%`, `at`, `literal`, `to-color`), so the tests
 * check the color picked for a feature instead of the expression's shape.
 * maplibre's own evaluator is not a direct dependency of the project.
 */
function evaluate(expression: unknown, properties: Record<string, unknown>): unknown {
  if (!Array.isArray(expression)) return expression
  const [operator, ...args] = expression as [string, ...unknown[]]
  switch (operator) {
    case 'literal':
      return args[0]
    case 'get':
      return properties[String(args[0])] ?? null
    case 'coalesce':
      return args.map(arg => evaluate(arg, properties)).find(value => value !== null && value !== undefined) ?? null
    case '%':
      return Number(evaluate(args[0], properties)) % Number(evaluate(args[1], properties))
    case 'at': {
      const list = evaluate(args[1], properties) as unknown[]
      const index = Number(evaluate(args[0], properties))
      if (!Number.isInteger(index) || index < 0 || index >= list.length) throw new Error(`at: index ${index} out of bounds`)
      return list[index]
    }
    case 'to-color':
      return evaluate(args[0], properties)
    case 'match': {
      const input = evaluate(args[0], properties)
      const fallback = args[args.length - 1]
      for (let index = 1; index < args.length - 1; index += 2) {
        if (args[index] === input) return evaluate(args[index + 1], properties)
      }
      return evaluate(fallback, properties)
    }
    default:
      throw new Error(`operator not supported by the test evaluator: ${operator}`)
  }
}

const square: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.29], [-46.72, -21.31]]],
}

describe('boundsOf', () => {
  it('returns null without geometries or positions', () => {
    expect(boundsOf([])).toBeNull()
    expect(boundsOf([{ type: 'Polygon', coordinates: [] }])).toBeNull()
    expect(boundsOf([{ type: 'Polygon', coordinates: [[]] }])).toBeNull()
  })

  it('computes [west, south, east, north] of a polygon', () => {
    expect(boundsOf([square])).toEqual([-46.72, -21.31, -46.70, -21.29])
  })

  it('collapses to a point for a single-position geometry', () => {
    expect(boundsOf([{ type: 'Polygon', coordinates: [[[-46.71, -21.3]]] }])).toEqual([-46.71, -21.3, -46.71, -21.3])
  })

  it('covers every polygon of a MultiPolygon, holes included, and mixed lists', () => {
    const multi: GeoJsonMultiPolygon = {
      type: 'MultiPolygon',
      coordinates: [
        [[[-47, -21], [-46.9, -21], [-46.9, -20.9], [-47, -21]]],
        [[[-45.9, -21.5], [-45.8, -21.5], [-45.8, -21.4], [-45.9, -21.5]], [[-45.85, -21.45], [-45.84, -21.45], [-45.84, -21.44], [-45.85, -21.45]]],
      ],
    }
    expect(boundsOf([multi])).toEqual([-47, -21.5, -45.8, -20.9])
    expect(boundsOf([square, multi])).toEqual([-47, -21.5, -45.8, -20.9])
  })
})

describe('featureFillColor', () => {
  it.each([
    ['suitable', palette['status-suitable']],
    ['attention', palette['status-attention']],
    ['unsuitable', palette['status-unsuitable']],
    ['rain', palette['info-rain']],
  ])('colors status %s with the palette', (status, color) => {
    expect(evaluate(featureFillColor('status'), { status })).toBe(color)
  })

  it('falls back to the first field color without a known status', () => {
    expect(evaluate(featureFillColor('status'), {})).toBe(palette['map-field-1'])
    expect(evaluate(featureFillColor('status'), { status: 'unknown' })).toBe(palette['map-field-1'])
  })

  it('cycles the six palette field colors by crop index', () => {
    const expression = featureFillColor('crop')
    expect(MAP_FIELD_COLORS).toEqual([
      palette['map-field-1'], palette['map-field-2'], palette['map-field-3'],
      palette['map-field-4'], palette['map-field-5'], palette['map-field-6'],
    ])
    expect(evaluate(expression, { cropIndex: 0 })).toBe(palette['map-field-1'])
    expect(evaluate(expression, { cropIndex: 5 })).toBe(palette['map-field-6'])
    expect(evaluate(expression, { cropIndex: 7 })).toBe(palette['map-field-2'])
    expect(evaluate(expression, {})).toBe(palette['map-field-1'])
  })
})

describe('createMapStyle', () => {
  const urls = { tilesUrl: 'https://tiles.test/style.json', satelliteUrl: 'https://sat.test/{z}/{x}/{y}.jpg' }

  it('builds the vector basemap with palette colors and no labels', () => {
    const style = createMapStyle({ ...urls, basemap: 'vector' })
    expect(style.version).toBe(8)
    expect(style.sources[SOURCE_BASEMAP]).toEqual({ type: 'vector', url: urls.tilesUrl })
    expect(style.layers.map(layer => layer.id)).toEqual(['land', 'vegetation', 'water', 'roads'])
    const paintOf = (id: string) => style.layers.find(layer => layer.id === id)?.paint as Record<string, unknown>
    expect(paintOf('land')['background-color']).toBe(palette['map-land'])
    expect(paintOf('vegetation')['fill-color']).toBe(palette['map-land-shade'])
    expect(paintOf('water')['fill-color']).toBe(palette['map-land-shade'])
    expect(paintOf('roads')['line-color']).toBe(palette['map-road'])
    expect(style.layers.some(layer => layer.type === 'symbol')).toBe(false)
  })

  it('adds the satellite raster on top only when selected and configured', () => {
    const satellite = createMapStyle({ ...urls, basemap: 'satellite' })
    expect(satellite.sources[SOURCE_SATELLITE]).toEqual({ type: 'raster', tiles: [urls.satelliteUrl], tileSize: 256 })
    expect(satellite.layers.at(-1)).toEqual({ id: 'satellite', type: 'raster', source: SOURCE_SATELLITE })

    const withoutUrl = createMapStyle({ ...urls, satelliteUrl: '', basemap: 'satellite' })
    expect(withoutUrl.sources[SOURCE_SATELLITE]).toBeUndefined()
    expect(withoutUrl.layers.some(layer => layer.id === 'satellite')).toBe(false)
  })
})
