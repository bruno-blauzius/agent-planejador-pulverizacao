/** Owner table row (Figma 39:633), ready to render. Presentation only. */
import type { FlashMessage } from '~/types/flash'
import type { LinkRole, OwnerFarmLink, OwnerSummary } from '~/types/owner'
import { formatPercent, initials, maskDocument } from './format'
import { PERSON_TYPE_SHORT_LABELS } from './labels'

export interface OwnerFarmChipView {
  readonly id: string
  /** "Santa Rita" (without "Fazenda"/"Sítio"). */
  readonly label: string
  /** "60%", "Arrend.", "Resp. legal" or "—". */
  readonly shareLabel: string
  /** "Santa Rita · 60%" */
  readonly text: string
}

export type OwnerShareTone = 'default' | 'role' | 'muted'

export interface OwnerRowView {
  readonly id: string
  readonly name: string
  readonly initials: string
  /** "PF · ***.278.916-**" | "PJ · 12.345.678/0001-90" */
  readonly documentLabel: string
  /** "Docs pendentes" badge. */
  readonly hasPendingDocuments: boolean
  readonly farms: readonly OwnerFarmChipView[]
  /** "Part." column: the share when the same in every farm, otherwise "Varia". */
  readonly shareLabel: string
  /** Tone of the "Part." column: a percentage, a role ("Arrend."), or muted ("Varia", "—"). */
  readonly shareTone: OwnerShareTone
  /** Row just saved (from the flash message). */
  readonly isHighlighted: boolean
}

const FARM_KIND_PREFIX = /^(fazenda|s[ií]tio|ch[aá]cara|est[aâ]ncia|granja|rancho)\s+/i
const MIXED_SHARE = 'Varia'
const EMPTY = '—'

/** "Fazenda Santa Rita" → "Santa Rita"; keeps the name when only the prefix exists. */
export function shortFarmName(name: string): string {
  const trimmed = name.trim()
  const short = trimmed.replace(FARM_KIND_PREFIX, '')
  return short || trimmed
}

const ROLE_SHARE_LABELS: Readonly<Record<Exclude<LinkRole, 'owner'>, string>> = {
  tenant: 'Arrend.',
  legal_representative: 'Resp. legal',
}

export function linkShareLabel(link: Pick<OwnerFarmLink, 'role' | 'sharePercent'>): string {
  if (link.role !== 'owner') return ROLE_SHARE_LABELS[link.role]
  return formatPercent(link.sharePercent)
}

export function toOwnerRowView(owner: OwnerSummary, highlightId: string | null = null): OwnerRowView {
  const farms = owner.links.map((link) => {
    const label = shortFarmName(link.farmName)
    const shareLabel = linkShareLabel(link)
    return { id: link.farmId, label, shareLabel, text: `${label} · ${shareLabel}` }
  })
  const distinctShares = new Set(farms.map(farm => farm.shareLabel))
  const roles = new Set(owner.links.map(link => (link.role === 'owner' ? 'share' : link.role)))
  const shareLabel = distinctShares.size === 0 ? EMPTY : distinctShares.size === 1 ? [...distinctShares][0]! : MIXED_SHARE
  const isSingleRole = distinctShares.size === 1 && roles.size === 1 && !roles.has('share')
  const shareTone: OwnerShareTone = shareLabel === EMPTY || shareLabel === MIXED_SHARE ? 'muted' : isSingleRole ? 'role' : 'default'
  return {
    id: owner.id,
    name: owner.name,
    initials: initials(owner.name),
    documentLabel: `${PERSON_TYPE_SHORT_LABELS[owner.personType]} · ${maskDocument(owner.document)}`,
    hasPendingDocuments: owner.hasPendingDocuments,
    farms,
    shareLabel,
    shareTone,
    isHighlighted: highlightId === owner.id,
  }
}

function describeLink(link: OwnerFarmLink): string {
  if (link.role === 'owner') return `${link.farmName} (${formatPercent(link.sharePercent)} de participação)`
  return `${link.farmName} (${link.role === 'tenant' ? 'arrendatário' : 'responsável legal'})`
}

/**
 * Alert shown on the owner list after the modal saves (Figma 39:1651). Texts
 * avoid gendered words ("cadastrada/o"), since names do not tell the gender.
 */
export function ownerSavedFlash(
  owner: Pick<OwnerSummary, 'id' | 'name'>,
  links: readonly OwnerFarmLink[],
  mode: 'created' | 'updated',
): FlashMessage {
  const title = mode === 'created' ? `Cadastro de ${owner.name} salvo` : `Dados de ${owner.name} atualizados`
  const description = links.length === 0
    ? 'Sem vínculos com fazendas.'
    : `${links.length === 1 ? 'Vínculo' : 'Vínculos'}: ${links.map(describeLink).join(', ')}.`
  return { kind: 'success', title, description, highlightId: owner.id }
}
