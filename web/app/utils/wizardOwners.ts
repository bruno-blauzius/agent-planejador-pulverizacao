/**
 * Presentation rules of wizard step 3 (owners): search suggestions, linked
 * owner rows and share input. Shares are never adjusted for the user: a total
 * other than 100% blocks "Continuar" here (UX) and the API rejects it on save.
 */
import type { OwnerSummary } from '~/types/owner'
import type { WizardOwnerShare } from '~/types/wizard'
import { formatPercent, initials, maskDocument } from './format'
import { PERSON_TYPE_SHORT_LABELS } from './labels'
import { isShareTotalComplete } from './wizard'

/** Suggestions only appear from this many characters on (Figma 41:681 shows "Hel"). */
export const OWNER_SEARCH_MIN_LENGTH = 2
export const OWNER_SEARCH_PAGE_SIZE = 8


/** One item of the search dropdown (Figma 41:681). */
export interface OwnerSuggestionView {
  readonly id: string
  readonly name: string
  readonly initials: string
  /** "PF · ***.330.884-** · Sítio Boa Esperança" / "PJ · 12.345.678/0001-90 · sem fazendas". */
  readonly detail: string
  readonly owner: OwnerSummary
}

/** One linked owner card (Figma 35:696 / 41:1176). */
export interface WizardOwnerRowView {
  readonly ownerId: string
  readonly name: string
  readonly initials: string
  /** "PF · ***.278.916-**", plus " · nova" for an owner registered in this wizard. */
  readonly detail: string
  /** Input text, e.g. "60". */
  readonly shareInput: string
  /** Just registered: highlighted card. */
  readonly isHighlighted: boolean
  readonly error: string | null
}

function farmsLabel(owner: OwnerSummary): string {
  if (owner.links.length === 0) return 'sem fazendas'
  return owner.links.map(link => link.farmName).join(', ')
}

export function toOwnerSuggestionView(owner: OwnerSummary): OwnerSuggestionView {
  return {
    id: owner.id,
    name: owner.name,
    initials: initials(owner.name),
    detail: [PERSON_TYPE_SHORT_LABELS[owner.personType], maskDocument(owner.document), farmsLabel(owner)].join(' · '),
    owner,
  }
}

export function toWizardOwnerRowView(
  owner: WizardOwnerShare,
  highlightId: string | null,
  error: string | null,
): WizardOwnerRowView {
  const parts = [PERSON_TYPE_SHORT_LABELS[owner.personType], owner.documentLabel]
  if (owner.isNewlyRegistered) parts.push('nova')
  return {
    ownerId: owner.ownerId,
    name: owner.name,
    initials: initials(owner.name),
    detail: parts.filter(Boolean).join(' · '),
    shareInput: owner.sharePercent === null ? '' : String(owner.sharePercent).replace('.', ','),
    isHighlighted: owner.ownerId === highlightId,
    error,
  }
}

/** "60", "60,5", "60%" → number; empty or invalid → null (the row shows the error). */
export function parseShareInput(text: string): number | null {
  const normalized = text.trim().replace('%', '').replace(',', '.').trim()
  if (!normalized) return null
  const value = Number(normalized)
  return Number.isFinite(value) ? value : null
}

/** "Não achou? Cadastrar "Hel…" como novo proprietário". */
export function newOwnerSuggestionLabel(search: string): string {
  return `Não achou? Cadastrar “${search.trim()}…” como novo proprietário`
}

/** Toast line after linking: null when the total closes 100%. */
export function shareTotalNotice(total: number): string | null {
  if (isShareTotalComplete(total)) return null
  return `Participação total: ${formatPercent(total)}. Ajuste para fechar 100%.`
}
