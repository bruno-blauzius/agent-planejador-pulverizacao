/** Front-only model of the new-farm wizard (screens 5–8). Never sent as is: see `toFarmInput`. */
import type { PerimeterSourceFormat, ProductionType } from './farm'
import type { GeoPoint, PerimeterGeometry } from './geo'
import type { LinkRole, PersonType } from './owner'

export type WizardStepKey = 'identification' | 'location' | 'owners' | 'review'
export type WizardStepStatus = 'complete' | 'current' | 'pending'
/** "Como localizar a fazenda?" (step 2). */
export type LocationMode = 'address' | 'pin' | 'file'
/** "Como definir o perímetro?" (step 2). `car` stays disabled until the API has an endpoint. */
export type PerimeterMethod = 'draw' | 'file' | 'car'

export interface IdentificationDraft {
  name: string
  /** "Apelido / código"; empty → null on submit. */
  code: string
  /** Empty → null on submit. */
  cooperative: string
  productionType: ProductionType | null
}

export interface LocationDraft {
  mode: LocationMode
  perimeterMethod: PerimeterMethod
  /** As typed; only digits are sent. */
  postalCode: string
  city: string
  /** UF. */
  state: string
  carNumber: string
  perimeter: PerimeterGeometry | null
  /** Only when the API computed it (perimeter import). Never computed in the front. */
  perimeterAreaHa: number | null
  /** Headquarters pinned on the map, typed, or the centroid returned by the import. */
  forecastPoint: GeoPoint | null
  /** Latitude/longitude inputs (pt-BR text, "-21,3045"); kept in sync with `forecastPoint`. */
  latitude: string
  longitude: string
  importedFileName: string | null
  importedFileSize: number | null
  importedSourceFormat: PerimeterSourceFormat | null
  /** Optional metadata returned by the import (shown on the result card). */
  importedCity: string | null
  importedState: string | null
  importedDatum: string | null
  importedFieldCount: number | null
}

export interface WizardOwnerShare {
  ownerId: string
  name: string
  personType: PersonType
  /** Already masked (`maskDocument`): the local draft never keeps a full CPF. */
  documentLabel: string
  role: LinkRole
  sharePercent: number | null
  /** Registered through the wizard ("· nova" on the card). */
  isNewlyRegistered: boolean
}

export interface FarmWizardDraft {
  version: 1
  identification: IdentificationDraft
  location: LocationDraft
  owners: WizardOwnerShare[]
  /** Step 4 "Confirmo que os dados estão corretos". */
  confirmed: boolean
  /** Furthest step reached; earlier steps are clickable. */
  furthestStep: WizardStepKey
  /** ISO timestamp of the last local save. */
  savedAt: string | null
  /** User who saved the draft; a draft of another user is discarded on restore. */
  userId: string | null
}

/** Field → message, for one step. */
export type WizardFieldErrors = Partial<Record<string, string>>
export type WizardErrors = Record<WizardStepKey, WizardFieldErrors>

/** One item of the steps component (Figma 14:125). */
export interface WizardStepView {
  readonly key: WizardStepKey
  readonly number: number
  readonly title: string
  readonly description: string
  readonly status: WizardStepStatus
  readonly isReachable: boolean
}

export interface WizardPreviewRow {
  readonly key: 'area' | 'city' | 'crop' | 'forecastPoint'
  readonly label: string
  readonly value: string
}

/** Live preview card (Figma 14:288). */
export interface FarmWizardPreview {
  /** "Nova fazenda" while empty. */
  readonly name: string
  readonly rows: readonly WizardPreviewRow[]
  readonly perimeter: PerimeterGeometry | null
  readonly forecastPoint: GeoPoint | null
  readonly hint: string
}

export interface WizardReviewRow {
  readonly label: string
  /** Final text; "Não informado" when the value is missing. */
  readonly value: string
  /** Missing value: shown in the muted style. */
  readonly isEmpty: boolean
}

/** One block of the review step (Figma 35:1048). */
export interface WizardReviewSection {
  readonly step: Exclude<WizardStepKey, 'review'>
  /** "1 · Identificação". */
  readonly title: string
  readonly rows: readonly WizardReviewRow[]
  /** Shown instead of the rows when there are none ("Nenhum proprietário vinculado."). */
  readonly emptyMessage: string | null
}
