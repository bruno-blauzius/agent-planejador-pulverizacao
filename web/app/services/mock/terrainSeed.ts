/**
 * Relief of the mock world (`NUXT_PUBLIC_API_MOCK=true`), coherent with the
 * farms of `seed.ts` and with the Figma frames of screen 21:
 * - Santa Rita is calculated; Talhão 4 is "forte ondulado" with 62 % of the area
 *   above 20 % (tractor restricted, self-propelled unsuitable), Talhão 2 has a
 *   manual adjustment and Talhão 9 failed to calculate;
 * - Boa Esperança was never calculated (frame 04e);
 * - the lowland field of Ouro Verde is flat and in a "baixada".
 */
import type { MockTerrainFieldSeed } from './terrainMock'

export const MOCK_TERRAIN_CALCULATED_FARM_IDS: readonly string[] = ['farm-santa-rita', 'farm-ouro-verde', 'farm-recanto']

export const MOCK_TERRAIN_FIELD_SEEDS: Readonly<Record<string, MockTerrainFieldSeed>> = {
  'field-str-4': {
    slopeMeanPct: 24.3,
    slopeMaxPct: 41.8,
    position: 'slope',
    aspect: 'northwest',
    elevationMinM: 872,
    elevationMaxM: 941,
    classShares: [0, 8, 30, 52, 10],
  },
  'field-str-2': {
    adjustment: {
      origin: 'topographic_survey',
      slopeMeanPct: 14.2,
      slopeMaxPct: 21.5,
      position: 'slope',
      justification: 'Levantamento topográfico de 2026: a declividade real é maior que a do modelo de 30 m.',
      adjustedBy: { id: 'user-marina', name: 'Marina Rezende' },
    },
  },
  'field-str-9': { failureReason: 'Bloco do modelo de elevação indisponível para este talhão.' },
  'field-fov-3': {
    slopeMeanPct: 1.8,
    slopeMaxPct: 4.1,
    position: 'lowland',
    aspect: 'flat',
    elevationMinM: 640,
    elevationMaxM: 652,
    classShares: [92, 8, 0, 0, 0],
  },
}
