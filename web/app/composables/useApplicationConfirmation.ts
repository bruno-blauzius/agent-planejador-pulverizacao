import { computed, ref, shallowRef, watch } from 'vue'
import type { ApplicationReview, RegisteredApplication } from '~/types/planner'
import type { ApiError } from '~/services/errors'
import { buildRegisteredNotice, buildReviewView, entryKey } from '~/utils/planner'
import { useChatState } from './internal/plannerState'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'
import { usePermissions } from './usePermissions'
import { usePlannerContext } from './usePlannerContext'

export interface UseApplicationConfirmationOptions {
  /** Only read on open and before each submit (a computed cannot watch the wall clock). */
  now?: () => Date
  /** Defaults to the shared selection (`usePlannerContext()`). */
  context?: ReturnType<typeof usePlannerContext>
}

interface Target {
  readonly planId: string
  readonly fieldId: string
}

/**
 * Human confirmation before registering an application (screen 17). Opening the
 * modal asks the API for a fresh review (checks recomputed now); nothing is
 * written until `submit()`, which needs the explicit "Revisei as condições e
 * confirmo a aplicação" checkbox. The agent never registers on its own.
 *
 * Only a plan of the current conversation, made for the farm now selected, can be
 * reviewed (`isPlanOutOfContext` otherwise, with no request); the API enforces the
 * same with 404/403. Expiry (`isExpired`) is evaluated on open and on submit, not
 * continuously: a review that expires while the modal is idle is caught on submit.
 */
export function useApplicationConfirmation(options: UseApplicationConfirmationOptions = {}) {
  const { applications } = useApiServices()
  const { can } = usePermissions()
  const now = options.now ?? (() => new Date())
  const context = options.context ?? usePlannerContext()
  const chat = useChatState()
  const planOutOfContext = ref(false)
  const alreadyRegisteredLocally = ref(false)

  /** Refreshed on open and before each submit (a computed cannot watch the wall clock). */
  const clock = shallowRef(now())
  const isOpen = ref(false)
  const acknowledged = ref(false)
  const target = shallowRef<Target | null>(null)
  const review = shallowRef<ApplicationReview | null>(null)
  const registered = shallowRef<RegisteredApplication | null>(null)

  const reviewAction = useAction((planId: string, fieldId: string) => applications.review(planId, fieldId))
  const registerAction = useAction((review: ApplicationReview) => applications.register({
    planId: review.planId,
    fieldId: review.fieldId,
    reviewId: review.id,
    // Warnings shown in the modal are acknowledged by the checkbox.
    confirmation: { confirmed: true, acknowledgedWarnings: buildReviewView(review).warningCodes },
  }))

  const seenSubmitError = shallowRef<ApiError | null>(null)
  const seenLoadError = shallowRef<ApiError | null>(null)
  const submitError = computed(() => (registerAction.error.value === seenSubmitError.value ? null : registerAction.error.value))
  const loadError = computed(() => (reviewAction.error.value === seenLoadError.value ? null : reviewAction.error.value))

  const view = computed(() => (review.value ? buildReviewView(review.value) : null))
  const canRegister = computed(() => can('applications.register'))
  const isExpired = computed(() => review.value !== null && clock.value > review.value.expiresAt)
  const canSubmit = computed(() =>
    canRegister.value
    && view.value?.canConfirm === true
    && acknowledged.value
    && !isExpired.value
    && !registerAction.pending.value
    && registered.value === null)

  /** The plan must come from this conversation, for the selected farm, and include the field. */
  function belongsToContext(planId: string, fieldId: string): boolean {
    const plan = chat.value.entries.find(entry => entry.plan?.id === planId)?.plan
    return plan !== undefined && plan !== null && plan.farmId === context.farmId.value
      && plan.entries.some(entry => entry.fieldId === fieldId)
  }

  /** Opens the modal and loads the review. Resolves `true` when there is something to confirm. */
  async function open(planId: string, fieldId: string): Promise<boolean> {
    target.value = { planId, fieldId }
    clock.value = now()
    planOutOfContext.value = !belongsToContext(planId, fieldId)
    alreadyRegisteredLocally.value = chat.value.registeredEntries.includes(entryKey(planId, fieldId))
    isOpen.value = true
    acknowledged.value = false
    registered.value = null
    review.value = null
    // Errors of the previous attempt belong to the previous review.
    seenSubmitError.value = registerAction.error.value
    seenLoadError.value = reviewAction.error.value
    if (!canRegister.value || planOutOfContext.value || alreadyRegisteredLocally.value) return false
    review.value = await reviewAction.execute(planId, fieldId)
    return review.value !== null
  }

  /** Reloads the review of the same entry (after `conditions_changed` or expiry). */
  async function reload(): Promise<boolean> {
    return target.value ? open(target.value.planId, target.value.fieldId) : false
  }

  function close() {
    isOpen.value = false
    planOutOfContext.value = false
    alreadyRegisteredLocally.value = false
    acknowledged.value = false
    review.value = null
    target.value = null
  }

  /** The API already has it: the entry stays registered for the rest of the conversation. */
  function markRegistered(planId: string, fieldId: string) {
    const key = entryKey(planId, fieldId)
    if (!chat.value.registeredEntries.includes(key)) {
      chat.value = { ...chat.value, registeredEntries: [...chat.value.registeredEntries, key] }
    }
  }

  /** Registers the application. Resolves with it, or `null` (see `submitError`). */
  async function submit(): Promise<RegisteredApplication | null> {
    clock.value = now()
    const current = review.value
    if (!canSubmit.value || !current) return null
    const result = await registerAction.execute(current)
    if (result) registered.value = result
    if (result || registerAction.error.value?.code === 'conflict') markRegistered(current.planId, current.fieldId)
    return result
  }

  // Another farm: whatever was open belongs to the previous one.
  watch(() => context.farmId.value, (farmId, previous) => {
    if (farmId !== previous) close()
  })

  const reviewCode = computed(() => loadError.value?.code)
  const submitCode = computed(() => submitError.value?.code)

  return {
    isOpen,
    review: view,
    /** Checkbox "Revisei as condições e confirmo a aplicação". */
    acknowledged,
    registered,
    /** Toast content for the last registration (title and message ready to show). */
    notice: computed(() => (registered.value ? buildRegisteredNotice(registered.value) : null)),
    /** Fields of the latest plan already registered: the UI disables "Revisar e registrar" there. */
    registeredFieldIds: computed<readonly string[]>(() => {
      const planId = [...chat.value.entries].reverse().find(entry => entry.plan)?.plan?.id
      if (!planId) return []
      const prefix = `${planId}:`
      return chat.value.registeredEntries.filter(key => key.startsWith(prefix)).map(key => key.slice(prefix.length))
    }),
    /** True when this entry of this plan was already registered in the conversation. */
    isFieldRegistered: (planId: string, fieldId: string) => chat.value.registeredEntries.includes(entryKey(planId, fieldId)),

    canRegister,
    canSubmit,
    isExpired,
    loading: reviewAction.pending,
    submitting: registerAction.pending,
    loadError,
    submitError,
    /** No capability, or the API answered 403/out_of_scope for the review or the registration. */
    isForbidden: computed(() => !canRegister.value
      || [reviewCode.value, submitCode.value].some(code => code === 'forbidden' || code === 'out_of_scope')),
    /** The forecast moved (or the review expired) after it was shown: reload and confirm again. */
    isStale: computed(() => isExpired.value || submitCode.value === 'conditions_changed'),
    /** 409 `conflict` from the API, or the entry was already registered in this conversation. */
    isAlreadyRegistered: computed(() => submitCode.value === 'conflict' || alreadyRegisteredLocally.value),
    isWeatherUnavailable: computed(() => reviewCode.value === 'weather_unavailable'),
    /** The plan is not from this conversation or farm: say to generate a new plan; nothing was requested. */
    isPlanOutOfContext: computed(() => planOutOfContext.value),
    /** A check is `blocked`: the confirm button stays disabled and `review.blockedTitles` says why. */
    isBlocked: computed(() => view.value !== null && !view.value.canConfirm),

    open,
    reload,
    close,
    submit,
  }
}
