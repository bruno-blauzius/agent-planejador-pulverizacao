/** One-shot message shown on the next screen (see `useFlash`). */
export interface FlashMessage {
  readonly kind: 'success' | 'attention' | 'error'
  readonly title: string
  readonly description?: string
  /** Id of the record just saved, highlighted on the next screen (e.g. owner row). */
  readonly highlightId?: string
}
