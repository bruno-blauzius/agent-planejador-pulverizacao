import type { PageRequest } from './pagination'

/** individual = pessoa física (CPF); company = pessoa jurídica (CNPJ). */
export type PersonType = 'individual' | 'company'

/** owner = proprietário com participação; tenant = arrendatário. */
export type LinkRole = 'owner' | 'tenant' | 'legal_representative'

export type OwnerDocumentKind = 'id_card' | 'driver_license' | 'articles_of_incorporation' | 'other'

export interface OwnerDocument {
  readonly id: string
  readonly kind: OwnerDocumentKind
  readonly fileName: string
  readonly uploadedAt: Date
}

export interface OwnerFarmLink {
  readonly farmId: string
  readonly farmName: string
  readonly role: LinkRole
  /** 0–100; null for tenants and legal representatives. */
  readonly sharePercent: number | null
}

export interface OwnerFarmLinkInput {
  readonly farmId: string
  readonly role: LinkRole
  readonly sharePercent: number | null
}

export interface OwnerSummary {
  readonly id: string
  readonly personType: PersonType
  /** Nome completo (PF) ou razão social (PJ). */
  readonly name: string
  /** CPF or CNPJ, digits only. Use `maskDocument`/`formatDocument` to display. */
  readonly document: string
  readonly hasPendingDocuments: boolean
  readonly links: readonly OwnerFarmLink[]
}

export interface Owner extends OwnerSummary {
  /** ISO date `YYYY-MM-DD`, individuals only. */
  readonly birthDate: string | null
  /** WhatsApp, digits with area code. */
  readonly phone: string | null
  readonly email: string | null
  readonly windowAlertsEnabled: boolean
  readonly documents: readonly OwnerDocument[]
  readonly createdAt: Date
  readonly updatedAt: Date
}

export interface OwnerInput {
  readonly personType: PersonType
  readonly name: string
  readonly document: string
  readonly birthDate: string | null
  readonly phone: string | null
  readonly email: string | null
  readonly windowAlertsEnabled: boolean
}

export interface OwnerFilters extends PageRequest {
  readonly search?: string
  readonly personType?: PersonType
}
