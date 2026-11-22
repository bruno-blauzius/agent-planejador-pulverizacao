import { describe, expect, it } from 'vitest'
import type { OwnerSummary } from '~/types/owner'
import type { WizardOwnerShare } from '~/types/wizard'
import {
  newOwnerSuggestionLabel,
  parseShareInput,
  shareTotalNotice,
  toOwnerSuggestionView,
  toWizardOwnerRowView,
} from '~/utils/wizardOwners'

function share(ownerId: string, name: string, sharePercent: number | null, role: WizardOwnerShare['role'] = 'owner'): WizardOwnerShare {
  return { ownerId, name, personType: 'individual', documentLabel: '***.278.916-**', role, sharePercent, isNewlyRegistered: false }
}

describe('owner search suggestions', () => {
  it('describes person type, masked document and farms', () => {
    const owner: OwnerSummary = {
      id: 'owner-helena',
      personType: 'individual',
      name: 'Helena Prado',
      document: '71533088420',
      hasPendingDocuments: false,
      links: [{ farmId: 'f1', farmName: 'Sítio Boa Esperança', role: 'owner', sharePercent: 100 }],
    }
    expect(toOwnerSuggestionView(owner)).toMatchObject({
      initials: 'HP',
      detail: 'PF · ***.330.884-** · Sítio Boa Esperança',
    })
    expect(toOwnerSuggestionView({ ...owner, personType: 'company', document: '12345678000190', links: [] }).detail)
      .toBe('PJ · 12.345.678/0001-90 · sem fazendas')
    expect(newOwnerSuggestionLabel(' Hel ')).toBe('Não achou? Cadastrar “Hel…” como novo proprietário')
  })
})

describe('linked owner rows', () => {
  it('marks a newly registered and highlighted owner', () => {
    const row = toWizardOwnerRowView({ ...share('h', 'Helena Prado', 20.5), isNewlyRegistered: true }, 'h', null)
    expect(row).toEqual({
      ownerId: 'h',
      name: 'Helena Prado',
      initials: 'HP',
      detail: 'PF · ***.278.916-** · nova',
      shareInput: '20,5',
      isHighlighted: true,
      error: null,
    })
  })

  it('parses the share input', () => {
    expect(parseShareInput('60')).toBe(60)
    expect(parseShareInput('60,5%')).toBe(60.5)
    expect(parseShareInput('')).toBeNull()
    expect(parseShareInput('abc')).toBeNull()
  })
})

describe('shareTotalNotice', () => {
  it('asks to adjust only when the total is not 100%', () => {
    expect(shareTotalNotice(100)).toBeNull()
    expect(shareTotalNotice(99.99)).toBeNull()
    expect(shareTotalNotice(120)).toBe('Participação total: 120%. Ajuste para fechar 100%.')
  })
})
