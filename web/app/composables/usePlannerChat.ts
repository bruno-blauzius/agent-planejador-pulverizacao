import { computed, ref, shallowRef, watch } from 'vue'
import type { ApiError } from '~/services/errors'
import type { PlannerMessageInput } from '~/types/planner'
import { buildPlanView, toAgentEntry, toUserEntry } from '~/utils/planner'
import { emptyChatState, useChatState } from './internal/plannerState'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'
import { registerLogoutCleanup } from './useLogoutCleanup'
import { usePermissions } from './usePermissions'
import { usePlannerContext } from './usePlannerContext'
import { useSession } from './useSession'

export interface UsePlannerChatOptions {
  now?: () => Date
  /** Defaults to the shared selection (`usePlannerContext()`). */
  context?: ReturnType<typeof usePlannerContext>
}

/**
 * Conversation with the planner agent (screen 16). One request per message, no
 * streaming (ADR 0016). The history lives in memory for the session and belongs
 * to ONE farm and ONE user: changing the farm or the user, or logging out,
 * clears it (plan, conversation id, suggestions), so a plan is never offered for
 * registration in a farm it was not made for. Failures (agent or weather down,
 * 403) are exposed, never answered with invented text.
 */
export function usePlannerChat(options: UsePlannerChatOptions = {}) {
  const { planner } = useApiServices()
  const { can } = usePermissions()
  const { user } = useSession()
  const context = options.context ?? usePlannerContext()
  const now = options.now ?? (() => new Date())
  const state = useChatState()
  const draft = ref('')
  /** Error already shown for a previous context: hidden after a reset. */
  const seenError = shallowRef<ApiError | null>(null)

  const delivery = useAction(async (input: PlannerMessageInput) => planner.sendMessage(input))

  function reset() {
    seenError.value = delivery.error.value
    state.value = emptyChatState(state.value.generation + 1)
    draft.value = ''
  }

  registerLogoutCleanup('planner-chat', reset)
  watch(() => context.farmId.value, (farmId, previous) => {
    if (farmId !== previous) reset()
  })
  watch(() => user.value?.id, (id, previous) => {
    if (id !== previous) reset()
  })

  async function deliver(input: PlannerMessageInput): Promise<boolean> {
    const generation = state.value.generation
    const reply = await delivery.execute(input)
    // The farm/user changed while the agent was thinking: the answer is for another context.
    if (state.value.generation !== generation) return false
    if (!reply) {
      state.value = { ...state.value, failedInput: input }
      return false
    }
    state.value = {
      ...state.value,
      conversationId: reply.conversationId,
      entries: [...state.value.entries, toAgentEntry(reply)],
      suggestions: [...reply.suggestions],
      failedInput: null,
    }
    return true
  }

  /** Sends `text` (or the draft). Resolves `true` when the agent answered. */
  async function send(text?: string): Promise<boolean> {
    const content = (text ?? draft.value).trim()
    const requestContext = context.requestContext.value
    if (!content || !requestContext || delivery.pending.value || !can('planner.use')) return false
    draft.value = ''
    state.value = {
      ...state.value,
      entries: [...state.value.entries, toUserEntry(`user-${state.value.entries.length + 1}`, content, now())],
      suggestions: [],
    }
    return deliver({ conversationId: state.value.conversationId, text: content, context: requestContext })
  }

  /** Repeats the request that failed. */
  async function retry(): Promise<boolean> {
    const input = state.value.failedInput
    if (!input || delivery.pending.value) return false
    return deliver(input)
  }

  const latestPlan = computed(() => [...state.value.entries].reverse().find(entry => entry.plan)?.plan ?? null)
  const error = computed(() => (delivery.error.value === seenError.value ? null : delivery.error.value))
  const errorCode = computed(() => error.value?.code)

  return {
    entries: computed(() => state.value.entries),
    conversationId: computed(() => state.value.conversationId),
    draft,
    suggestions: computed(() => state.value.suggestions),
    isEmpty: computed(() => state.value.entries.length === 0),

    /** Plan of the most recent agent reply that had one ("Plano sugerido pelo agente"). */
    latestPlan,
    planView: computed(() => (latestPlan.value ? buildPlanView(latestPlan.value) : null)),

    canUse: computed(() => can('planner.use')),
    /** Farm and at least one field are needed before asking. */
    contextMissing: computed(() => !context.isReady.value),
    pending: delivery.pending,
    error,
    canRetry: computed(() => state.value.failedInput !== null),
    isForbidden: computed(() => errorCode.value === 'forbidden' || errorCode.value === 'out_of_scope'),
    isAgentUnavailable: computed(() => errorCode.value === 'agent_unavailable'),
    isWeatherUnavailable: computed(() => errorCode.value === 'weather_unavailable'),

    send,
    retry,
    reset,
  }
}
