import { computed, getCurrentScope, onScopeDispose, readonly, ref, shallowRef, watch, type Ref } from 'vue'
import type { Owner, OwnerSummary } from '~/types/owner'
import type { FarmWizardDraft, WizardFieldErrors } from '~/types/wizard'
import { formatPercent, maskDocument } from '~/utils/format'
import { isShareTotalComplete, ownerErrorKey, wizardShareTotal } from '~/utils/wizard'
import {
  newOwnerSuggestionLabel,
  OWNER_SEARCH_MIN_LENGTH,
  OWNER_SEARCH_PAGE_SIZE,
  parseShareInput,
  shareTotalNotice,
  toOwnerSuggestionView,
  toWizardOwnerRowView,
} from '~/utils/wizardOwners'
import { useOwners } from './useOwners'

export interface UseFarmWizardOwnersOptions {
  /** Delay before searching while the user types. Tests pass 0. */
  searchDebounceMs?: number
}

/** In-page toast after "Salvar e vincular" (Figma 41:1176). */
export interface WizardOwnerNotice {
  readonly title: string
  readonly description: string | null
}

/** What `useOwnerForm({ mode: 'wizard' })` returns on submit. */
export interface WizardOwnerFormResult {
  readonly owner: Owner
  readonly sharePercent: number
}

const DEFAULT_DEBOUNCE_MS = 300
const SHARE_TOTAL = 100

/**
 * Step 3 of the new-farm wizard (Figma 35:696, 41:681, 41:914, 41:1176):
 * owner search with debounce, link/unlink, share per owner with the total,
 * and the owner registered through the wizard-mode owner form (trilha A).
 * Works on the wizard draft owned by `useFarmWizard`.
 */
export function useFarmWizardOwners(
  draft: Ref<FarmWizardDraft>,
  errors: Readonly<Ref<WizardFieldErrors>>,
  options: UseFarmWizardOwnersOptions = {},
) {
  const debounceMs = options.searchDebounceMs ?? DEFAULT_DEBOUNCE_MS
  const search = ref('')
  const debouncedSearch = ref('')
  const highlightId = ref<string | null>(null)
  const notice = shallowRef<WizardOwnerNotice | null>(null)
  const alertsBannerVisible = ref(true)

  let timer: ReturnType<typeof setTimeout> | undefined
  watch(search, (value) => {
    clearTimeout(timer)
    if (debounceMs <= 0) debouncedSearch.value = value.trim()
    else timer = setTimeout(() => (debouncedSearch.value = value.trim()), debounceMs)
  })
  if (getCurrentScope()) onScopeDispose(() => clearTimeout(timer))

  const isSearching = computed(() => debouncedSearch.value.length >= OWNER_SEARCH_MIN_LENGTH)
  const results = useOwners(() => ({
    search: isSearching.value ? debouncedSearch.value : undefined,
    pageSize: OWNER_SEARCH_PAGE_SIZE,
  }))

  const linkedIds = computed(() => new Set(draft.value.owners.map(owner => owner.ownerId)))

  const suggestions = computed(() => {
    if (!isSearching.value) return []
    return results.owners.value.filter(owner => !linkedIds.value.has(owner.id)).map(toOwnerSuggestionView)
  })

  const totalShare = computed(() => wizardShareTotal(draft.value.owners))

  function push(owner: OwnerSummary | Owner, sharePercent: number | null, isNewlyRegistered: boolean) {
    draft.value.owners.push({
      ownerId: owner.id,
      name: owner.name,
      personType: owner.personType,
      documentLabel: maskDocument(owner.document),
      role: 'owner',
      sharePercent,
      isNewlyRegistered,
    })
  }

  /** "Vincular" an existing owner; suggests the remaining share (100% for the first). */
  function addOwner(owner: OwnerSummary | Owner, sharePercent: number | null = null) {
    if (linkedIds.value.has(owner.id)) return
    const remaining = SHARE_TOTAL - totalShare.value
    push(owner, sharePercent ?? (remaining > 0 ? remaining : null), false)
    search.value = ''
    debouncedSearch.value = ''
  }

  /**
   * "Salvar e vincular" in the wizard-mode owner form: links the new owner with
   * the typed share. The others keep theirs (user decision): a total other than
   * 100% shows as an error that blocks "Continuar" until the user adjusts it.
   */
  function linkRegisteredOwner(result: WizardOwnerFormResult) {
    if (linkedIds.value.has(result.owner.id)) return
    push(result.owner, result.sharePercent, true)
    highlightId.value = result.owner.id
    search.value = ''
    debouncedSearch.value = ''
    notice.value = {
      title: `${result.owner.name} cadastrada e vinculada`,
      description: shareTotalNotice(totalShare.value),
    }
  }

  function removeOwner(ownerId: string) {
    // splice in place: `filter` would store a new array of reactive proxies in the draft.
    const index = draft.value.owners.findIndex(owner => owner.ownerId === ownerId)
    if (index >= 0) draft.value.owners.splice(index, 1)
    if (highlightId.value === ownerId) highlightId.value = null
    // The toast described the list before the removal: it no longer applies.
    notice.value = null
  }

  /** Accepts the input text ("60", "60,5") or a number; invalid text clears the share. */
  function setOwnerShare(ownerId: string, share: string | number | null) {
    const owner = draft.value.owners.find(item => item.ownerId === ownerId)
    if (!owner) return
    owner.sharePercent = typeof share === 'string' ? parseShareInput(share) : share
  }

  return {
    ownerSearch: search,
    /** Dropdown open: enough characters typed. */
    isOwnerSearchActive: isSearching,
    ownerSuggestions: suggestions,
    ownerSearchPending: results.pending,
    ownerSearchError: results.error,
    /** "Não achou? Cadastrar "Hel…" como novo proprietário"; null while not searching. */
    newOwnerSuggestion: computed(() => (isSearching.value ? newOwnerSuggestionLabel(search.value) : null)),
    /** Preset of `useOwnerForm({ mode: 'wizard', presetFarm })`: farm name and current shares. */
    ownerFormPreset: computed(() => ({
      name: draft.value.identification.name.trim() || 'Nova fazenda',
      currentShares: draft.value.owners
        .filter(owner => owner.role === 'owner')
        .map(owner => ({ name: owner.name, sharePercent: owner.sharePercent ?? 0 })),
    })),
    /** Name typed in the search, to prefill the new-owner form. */
    newOwnerPrefillName: computed(() => search.value.trim()),
    ownerRows: computed(() => draft.value.owners.map(owner =>
      toWizardOwnerRowView(owner, highlightId.value, errors.value[ownerErrorKey(owner.ownerId)] ?? null))),
    addOwner,
    linkRegisteredOwner,
    removeOwner,
    setOwnerShare,
    totalShare,
    shareTotalLabel: computed(() => formatPercent(totalShare.value)),
    /** Bar width, 0–100. */
    shareProgress: computed(() => Math.min(SHARE_TOTAL, Math.max(0, totalShare.value))),
    isShareComplete: computed(() => isShareTotalComplete(totalShare.value)),
    ownerNotice: readonly(notice),
    dismissOwnerNotice: () => (notice.value = null),
    /** "Avisos de janela para os proprietários" banner (dismissible, this visit only). */
    alertsBannerVisible: readonly(alertsBannerVisible),
    dismissAlertsBanner: () => (alertsBannerVisible.value = false),
  }
}
