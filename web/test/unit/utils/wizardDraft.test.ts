import { describe, expect, it } from 'vitest'
import { createEmptyDraft } from '~/utils/wizard'
import { parseDraft, serializeDraft } from '~/utils/wizardDraft'

describe('wizard draft storage format', () => {
  it('round-trips a draft', () => {
    const draft = createEmptyDraft()
    draft.identification.name = 'Fazenda Santa Rita'
    draft.identification.productionType = 'grains'
    draft.location.perimeter = { type: 'Polygon', coordinates: [[[-46.7, -21.3], [-46.6, -21.3], [-46.6, -21.2], [-46.7, -21.3]]] }
    draft.owners = [{
      ownerId: 'owner-1',
      name: 'Bruno',
      personType: 'individual',
      documentLabel: '***.278.916-**',
      role: 'owner',
      sharePercent: 100,
      isNewlyRegistered: true,
    }]
    draft.furthestStep = 'owners'
    draft.savedAt = '2026-11-21T12:00:00.000Z'

    expect(parseDraft(serializeDraft(draft))).toEqual(draft)
  })

  it('ignores missing, corrupt and other-version drafts', () => {
    expect(parseDraft(null)).toBeNull()
    expect(parseDraft('{not json')).toBeNull()
    expect(parseDraft(JSON.stringify({ version: 2 }))).toBeNull()
  })

  it('drops invalid pieces instead of trusting them', () => {
    const parsed = parseDraft(JSON.stringify({
      version: 1,
      identification: { name: 42, productionType: 'rice' },
      location: { perimeter: { type: 'Polygon', coordinates: [[['x', 1]]] }, forecastPoint: { lat: 'a' } },
      owners: [{ ownerId: 'owner-1' }, null],
      furthestStep: 'payment',
    }))
    expect(parsed?.identification).toEqual({ name: '', code: '', cooperative: '', productionType: null })
    expect(parsed?.location.perimeter).toBeNull()
    expect(parsed?.location.forecastPoint).toBeNull()
    expect(parsed?.owners).toEqual([])
    expect(parsed?.furthestStep).toBe('identification')
  })
})
