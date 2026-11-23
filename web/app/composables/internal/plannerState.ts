/**
 * Planner state shared by several composables (chat, confirmation). Not
 * auto-imported (nested folder): pages and components use the `useX` composables.
 */
import type { ChatEntry } from '~/utils/planner'
import type { PlannerMessageInput } from '~/types/planner'

export interface ChatState {
  conversationId: string | null
  entries: ChatEntry[]
  suggestions: string[]
  /** Message whose reply failed: kept so "Tentar de novo" does not duplicate the user bubble. */
  failedInput: PlannerMessageInput | null
  /** Bumped on every reset: a reply that arrives after a reset belongs to the old context and is dropped. */
  generation: number
  /** `planId:fieldId` of the plan entries already registered (cleared with the conversation). */
  registeredEntries: string[]
}

export const emptyChatState = (generation = 0): ChatState => ({
  conversationId: null,
  entries: [],
  suggestions: [],
  failedInput: null,
  generation,
  registeredEntries: [],
})

/** The conversation of the signed-in user in the selected farm. */
export function useChatState() {
  return useState<ChatState>('planner:chat', () => emptyChatState())
}
