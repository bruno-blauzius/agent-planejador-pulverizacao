import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import type { GeoJsonPolygon } from '~/types/geo'
import { createEmptyDraft } from '~/utils/wizard'
import {
  buildImportSummary,
  formatCoordinateInput,
  importErrorMessage,
  isHeadquartersOutside,
  LOCATION_MESSAGES,
  parseCoordinate,
  parseCoordinatesQuery,
  perimeterAreaLabel,
  PERIMETER_IMPORT_MAX_BYTES,
  pointFromInputs,
  precheckPerimeterFile,
  validateCoordinateInputs,
} from '~/utils/wizardLocation'

const SQUARE: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.29], [-46.72, -21.31]]],
}

describe('coordinates typed by hand', () => {
  it('parses decimal comma or point', () => {
    expect(parseCoordinate('-21,3045')).toBe(-21.3045)
    expect(parseCoordinate(' -46.7128 ')).toBe(-46.7128)
    expect(parseCoordinate('21°S')).toBeNull()
    expect(formatCoordinateInput(-21.3045)).toBe('-21,3045')
  })

  it('builds a point only with both values in range', () => {
    expect(pointFromInputs('-21,3', '-46,71')).toEqual({ lat: -21.3, lng: -46.71 })
    expect(pointFromInputs('-91', '-46,71')).toBeNull()
    expect(pointFromInputs('-21,3', '')).toBeNull()
  })

  it('reads a pasted pair, latitude first', () => {
    expect(parseCoordinatesQuery('-21,30, -46,71')).toEqual({ lat: -21.3, lng: -46.71 })
    expect(parseCoordinatesQuery('-21.30 -46.71')).toEqual({ lat: -21.3, lng: -46.71 })
    expect(parseCoordinatesQuery('-21,30; -46,71')).toEqual({ lat: -21.3, lng: -46.71 })
    expect(parseCoordinatesQuery('Guaxupé, MG')).toBeNull()
  })

  it('accepts empty inputs (optional) but not half of a pair', () => {
    expect(validateCoordinateInputs('', '')).toEqual({})
    expect(validateCoordinateInputs('-21,3', '')).toEqual({ longitude: LOCATION_MESSAGES.coordinatesIncomplete })
    expect(validateCoordinateInputs('abc', '-200')).toEqual({
      latitude: LOCATION_MESSAGES.latitudeInvalid,
      longitude: LOCATION_MESSAGES.longitudeInvalid,
    })
  })
})

describe('perimeter import', () => {
  it('rejects unknown formats and large files before uploading', () => {
    expect(precheckPerimeterFile({ name: 'fazenda.pdf', size: 10 })).toBe(LOCATION_MESSAGES.importFormat)
    expect(precheckPerimeterFile({ name: 'fazenda.KML', size: PERIMETER_IMPORT_MAX_BYTES + 1 }))
      .toBe(LOCATION_MESSAGES.importTooLarge)
    expect(precheckPerimeterFile({ name: 'car.zip', size: 2048 })).toBeNull()
  })

  it('turns API failures into honest messages', () => {
    const error = (code: ApiError['code'], details: { field: string, message: string }[] = []) =>
      new ApiError({ code, message: 'Sem conexão.', status: 400, details })
    expect(importErrorMessage(error('unsupported_media_type'))).toBe(LOCATION_MESSAGES.importFormat)
    expect(importErrorMessage(error('payload_too_large'))).toBe(LOCATION_MESSAGES.importTooLarge)
    expect(importErrorMessage(error('validation_failed'))).toBe(LOCATION_MESSAGES.importNoPolygon)
    expect(importErrorMessage(error('validation_failed', [{ field: 'file', message: 'o arquivo não tem polígonos' }])))
      .toBe('o arquivo não tem polígonos')
    expect(importErrorMessage(error('network_error'))).toBe('Sem conexão.')
  })

  it('summarizes the imported file with the official area', () => {
    const location = {
      ...createEmptyDraft().location,
      perimeter: SQUARE,
      perimeterAreaHa: 486.2,
      forecastPoint: { lat: -21.3, lng: -46.71 },
      importedFileName: 'fazenda-santa-rita.kml',
      importedFileSize: 48 * 1024,
      importedSourceFormat: 'kml' as const,
      importedCity: 'Guaxupé',
      importedState: 'MG',
      importedDatum: 'SIRGAS 2000',
      importedFieldCount: 14,
    }
    expect(buildImportSummary(location)).toEqual({
      fileName: 'fazenda-santa-rita.kml',
      detailLabel: '48 KB · KML',
      areaLabel: '486,2 ha',
      cityLabel: 'Guaxupé · MG',
      forecastPointLabel: '-21,30 · -46,71',
      datumLabel: 'SIRGAS 2000',
      fieldCount: 14,
    })
    expect(perimeterAreaLabel(location)).toBe('486,2 ha')
  })

  it('warns (without blocking) when the headquarters is outside the perimeter', () => {
    expect(isHeadquartersOutside({ perimeter: SQUARE, forecastPoint: { lat: -21.3, lng: -46.71 } })).toBe(false)
    expect(isHeadquartersOutside({ perimeter: SQUARE, forecastPoint: { lat: -22, lng: -47 } })).toBe(true)
    expect(isHeadquartersOutside({ perimeter: null, forecastPoint: { lat: -22, lng: -47 } })).toBe(false)
  })

  it('previews a drawn perimeter as approximate and hides it while unfinished', () => {
    expect(perimeterAreaLabel({ perimeter: SQUARE, perimeterAreaHa: null })).toMatch(/^≈ .+ ha · aproximada$/)
    expect(perimeterAreaLabel({
      perimeter: { type: 'Polygon', coordinates: [[[-46.7, -21.3], [-46.6, -21.3]]] },
      perimeterAreaHa: null,
    })).toBeNull()
    expect(perimeterAreaLabel({ perimeter: null, perimeterAreaHa: null })).toBeNull()
  })
})
