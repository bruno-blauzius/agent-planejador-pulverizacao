import { computed, getCurrentScope, onScopeDispose, reactive, ref, shallowRef, watch } from 'vue'
import type { FarmWizardDraft, LocationMode, WizardErrors, WizardFieldErrors, WizardStepKey } from '~/types/wizard'
import { farmCreatedFlash } from '~/utils/farmCard'
import {
  buildPreview,
  buildReview,
  buildStepViews,
  createEmptyDraft,
  emptyWizardErrors,
  formatDraftSavedLabel,
  isStepReachable,
  isStepValid,
  mapApiErrorsToSteps,
  parseWizardStep,
  stepIndex,
  toFarmInput,
  validateStep,
  WIZARD_MESSAGES,
  WIZARD_STEPS,
} from '~/utils/wizard'
import { browserDraftStorage, parseDraft, serializeDraft, type WizardDraftStorage } from '~/utils/wizardDraft'
import { useFarmActions } from './useFarmActions'
import { useFarmWizardLocation, type UseFarmWizardLocationOptions } from './useFarmWizardLocation'
import { useFarmWizardOwners, type UseFarmWizardOwnersOptions } from './useFarmWizardOwners'
import { forbiddenMessageFor } from '~/utils/permissions'
import { useFlash } from './useFlash'
import { registerLogoutCleanup } from './useLogoutCleanup'
import { useSession } from './useSession'

export interface UseFarmWizardOptions extends UseFarmWizardLocationOptions, UseFarmWizardOwnersOptions {
  /** Default: `localStorage` in the browser, none on the server. Tests inject a fake. */
  storage?: WizardDraftStorage | null
  now?: () => Date
}

const FARMS_PATH = '/farms'
const DRAFT_STATE_KEY = 'farm-wizard:draft'
const ENTRY_MODES: readonly LocationMode[] = ['address', 'pin', 'file']

const CLOCK_TICK_MS = 30_000

/** Everything the user edits; save metadata stays out so saving does not retrigger itself. */
function draftContent(draft: FarmWizardDraft): string {
  const { savedAt: _savedAt, userId: _userId, ...content } = draft
  return JSON.stringify(content)
}

/**
 * New-farm wizard (screens 5–8): draft of the 4 steps, step navigation through
 * `?step=`, presentation checks, live preview, review and the final save.
 *
 * The draft lives in `useState` (survives step changes and client navigation)
 * and, in the browser, in local storage (survives reloads and "Salvar e sair").
 * The stored draft is restored on mount and is never overwritten before that;
 * a draft of another user is discarded, and logout clears it.
 * Call it once per page and pass slices to the components as props.
 */
export function useFarmWizard(options: UseFarmWizardOptions = {}) {
  const route = useRoute()
  const flash = useFlash()
  const actions = useFarmActions()
  const session = useSession()
  const now = options.now ?? (() => new Date())
  const storage = options.storage !== undefined ? options.storage : browserDraftStorage()

  const draft = useState<FarmWizardDraft>(DRAFT_STATE_KEY, createEmptyDraft)
  const touched = reactive(new Set<string>())
  const attempted = reactive(new Set<WizardStepKey>())
  const apiErrors = shallowRef<WizardErrors>(emptyWizardErrors())
  const generalError = ref<string | null>(null)
  const clock = ref(now())
  let lastSavedContent = draftContent(draft.value)
  /** Nothing is written to storage before the stored draft was read (it would be lost). */
  let restored = false

  // --- steps -----------------------------------------------------------------

  /** `?source=` alone (e.g. /farms/new?source=file) means step 2. */
  const requestedStep = computed(() =>
    parseWizardStep(route.query.step) ?? (route.query.source ? 'location' : 'identification'))

  /** The requested step, or the first one the user must fill before reaching it. */
  const currentStep = computed<WizardStepKey>(() => {
    const requested = requestedStep.value
    if (isStepReachable(draft.value, requested)) return requested
    return WIZARD_STEPS.find(step => !isStepValid(draft.value, step)) ?? requested
  })

  const steps = computed(() => buildStepViews(draft.value, currentStep.value))
  const stepNumber = computed(() => stepIndex(currentStep.value) + 1)

  function reach(step: WizardStepKey) {
    if (stepIndex(step) > stepIndex(draft.value.furthestStep)) draft.value.furthestStep = step
  }

  async function navigateToStep(step: WizardStepKey, replace = false) {
    await navigateTo({ query: { ...route.query, step, source: undefined } }, replace ? { replace } : undefined)
  }

  async function goTo(step: WizardStepKey): Promise<boolean> {
    if (!isStepReachable(draft.value, step)) return false
    await navigateToStep(step)
    return true
  }

  async function back() {
    const index = stepIndex(currentStep.value)
    if (index === 0) return
    await navigateToStep(WIZARD_STEPS[index - 1] ?? 'identification')
  }

  // --- validation ------------------------------------------------------------

  function visibleErrors(step: WizardStepKey): WizardFieldErrors {
    const local = validateStep(draft.value, step)
    const visibleLocal: WizardFieldErrors = {}
    for (const [field, message] of Object.entries(local)) {
      if (attempted.has(step) || touched.has(`${step}:${field}`)) visibleLocal[field] = message
    }
    return { ...apiErrors.value[step], ...visibleLocal }
  }

  const fieldErrors = computed(() => visibleErrors(currentStep.value))

  function touch(field: string) {
    touched.add(`${currentStep.value}:${field}`)
  }

  /** Goes to the next step when the current one is valid; on the review step, saves. */
  async function next(): Promise<boolean> {
    const step = currentStep.value
    attempted.add(step)
    if (!isStepValid(draft.value, step)) return false
    if (step === 'review') return save()
    const target = WIZARD_STEPS[stepIndex(step) + 1] ?? 'review'
    reach(target)
    await navigateToStep(target)
    return true
  }

  // --- step 2: location (useFarmWizardLocation) ------------------------------

  const locationStep = useFarmWizardLocation(draft, { geolocation: options.geolocation })

  // --- step 3: owners (useFarmWizardOwners) -----------------------------------

  const ownersStep = useFarmWizardOwners(draft, computed(() => visibleErrors('owners')), {
    searchDebounceMs: options.searchDebounceMs,
  })
  // The "cadastrada e vinculada" toast belongs to that visit of step 3.
  watch(currentStep, (step, previous) => {
    if (previous === 'owners' && step !== 'owners') ownersStep.dismissOwnerNotice()
  })

  // --- draft persistence -----------------------------------------------------

  function persist() {
    if (!storage || !restored) return
    draft.value.savedAt = now().toISOString()
    draft.value.userId = session.user.value?.id ?? null
    lastSavedContent = draftContent(draft.value)
    storage.save(serializeDraft(draft.value))
  }

  function discardDraft() {
    storage?.clear()
    draft.value = createEmptyDraft()
    lastSavedContent = draftContent(draft.value)
    apiErrors.value = emptyWizardErrors()
  }

  /**
   * `?source=address|pin|file` opens step 2 in that mode, even with step 1
   * empty ("Importar KML" on the list uses `source=file`).
   */
  function applyEntryQuery() {
    const mode = ENTRY_MODES.find(item => item === route.query.source)
    if (!mode) return
    draft.value.location.mode = mode
    reach('location')
  }

  function belongsToAnotherUser(stored: FarmWizardDraft): boolean {
    const currentUserId = session.user.value?.id ?? null
    return stored.userId !== null && currentUserId !== null && stored.userId !== currentUserId
  }

  function restoreDraft() {
    if (storage && draft.value.savedAt === null) {
      const stored = parseDraft(storage.load())
      if (stored && belongsToAnotherUser(stored)) storage.clear()
      else if (stored) {
        draft.value = stored
        lastSavedContent = draftContent(stored)
      }
    }
    restored = true
    applyEntryQuery()
    if (draftContent(draft.value) !== lastSavedContent) persist()
    if (requestedStep.value !== currentStep.value) void navigateToStep(currentStep.value, true)
  }

  watch(() => draftContent(draft.value), (content) => {
    // Any edit hides the API errors of the last save attempt.
    apiErrors.value = emptyWizardErrors()
    generalError.value = null
    if (content !== lastSavedContent) persist()
  })

  applyEntryQuery()
  // On mount (client only, after hydration): server and client render the same
  // first frame, and unlike onNuxtReady it does not wait for an idle callback.
  onMounted(restoreDraft)
  registerLogoutCleanup('farm-wizard', discardDraft)

  if (import.meta.client && getCurrentScope()) {
    const timer = setInterval(() => (clock.value = now()), CLOCK_TICK_MS)
    onScopeDispose(() => clearInterval(timer))
  }

  const draftSavedLabel = computed(() => formatDraftSavedLabel(draft.value.savedAt, clock.value))
  /** Epoch ms of the last local save; changes on every new save (live announcement). */
  const draftSavedAt = computed<number | null>(() => {
    const time = draft.value.savedAt ? Date.parse(draft.value.savedAt) : Number.NaN
    return Number.isNaN(time) ? null : time
  })

  // --- save ------------------------------------------------------------------

  async function handleSaveFailure() {
    const error = actions.saveError.value
    if (!error) return
    if (error.code !== 'validation_failed' || error.details.length === 0) {
      generalError.value = forbiddenMessageFor(error) ?? (error.message || WIZARD_MESSAGES.saveFailed)
      return
    }
    const { errors, unmapped } = mapApiErrorsToSteps(error.details, draft.value.owners)
    apiErrors.value = errors
    generalError.value = unmapped.length > 0 ? unmapped.map(item => item.message).join(' ') : WIZARD_MESSAGES.saveInvalid
    const stepWithError = WIZARD_STEPS.find(step => Object.keys(errors[step]).length > 0)
    if (stepWithError) await navigateToStep(stepWithError)
  }

  /** Validates all steps, creates the farm, then returns to /farms with a toast. */
  async function save(): Promise<boolean> {
    try {
      return await saveDraft()
    }
    catch (error) {
      // Anything unexpected (not an API error, which useAction already handles)
      // is reported to the user instead of escaping as an unhandled rejection.
      console.error('[farm-wizard] save failed', error)
      generalError.value = WIZARD_MESSAGES.saveFailed
      return false
    }
  }

  async function saveDraft(): Promise<boolean> {
    if (actions.saving.value) return false
    WIZARD_STEPS.forEach(step => attempted.add(step))
    const invalid = WIZARD_STEPS.find(step => !isStepValid(draft.value, step))
    // Plain deep copy through serialization: the payload must not alias the
    // editable draft, and nested reactive proxies cannot go through structuredClone.
    const plain = parseDraft(serializeDraft(draft.value))
    const input = plain ? toFarmInput(plain) : null
    if (invalid || !input) {
      await navigateToStep(invalid ?? 'identification')
      return false
    }

    generalError.value = null
    const farm = await actions.createFarm(input)
    if (!farm) {
      await handleSaveFailure()
      return false
    }
    flash.show(farmCreatedFlash(farm))
    discardDraft()
    await navigateTo(FARMS_PATH)
    return true
  }

  /** Keeps the local draft (resumed at /farms/new) and returns to the list. */
  async function saveAndExit() {
    persist()
    const name = draft.value.identification.name.trim()
    flash.show({
      kind: 'success',
      title: WIZARD_MESSAGES.draftSaved,
      description: name ? `Continue o cadastro de ${name} em "Nova fazenda".` : 'Continue o cadastro em "Nova fazenda".',
    })
    await navigateTo(FARMS_PATH)
  }

  /** Discards the draft and returns to the list. */
  async function cancel() {
    discardDraft()
    await navigateTo(FARMS_PATH)
  }

  return {
    identification: computed(() => draft.value.identification),
    location: computed(() => draft.value.location),
    owners: computed(() => draft.value.owners),
    confirmed: computed({
      get: () => draft.value.confirmed,
      set: (value: boolean) => (draft.value.confirmed = value),
    }),

    currentStep,
    stepNumber,
    steps,
    isFirstStep: computed(() => stepIndex(currentStep.value) === 0),
    isLastStep: computed(() => currentStep.value === 'review'),
    next,
    back,
    goTo,
    cancel,

    fieldErrors,
    touch,
    isStepValid: (step: WizardStepKey) => isStepValid(draft.value, step),

    ...locationStep,

    ...ownersStep,

    preview: computed(() => buildPreview(draft.value)),
    review: computed(() => buildReview(draft.value)),
    draftSavedLabel,
    draftSavedAt,

    save,
    saveAndExit,
    saving: actions.saving,
    saveError: computed(() => generalError.value),
  }
}
