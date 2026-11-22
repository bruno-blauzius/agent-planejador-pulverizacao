/**
 * Spray condition computed by the backend (spray-planner rules for Delta T,
 * wind and rain). The front only displays it; it never derives it.
 */
export type SprayCondition = 'suitable' | 'attention' | 'unsuitable' | 'rain'

export interface SprayOutlook {
  readonly condition: SprayCondition
  /** Ready-to-display sentence, e.g. "2 janelas aptas hoje". */
  readonly headline: string
}
