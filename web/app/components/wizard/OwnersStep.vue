<script setup lang="ts">
import type { OwnerFormOptions, OwnerFormResult } from '~/composables/useOwnerForm'
import type { WizardOwnerFormResult, WizardOwnerNotice } from '~/composables/useFarmWizardOwners'
import type { OwnerSummary } from '~/types/owner'
import type { OwnerSuggestionView, WizardOwnerRowView } from '~/utils/wizardOwners'

/**
 * Step 3 — Proprietários (Figma 35:696 list, 41:681 search, 41:914 new owner
 * modal, 41:1176 linked + toast). Presentation only: search results, rows,
 * totals and errors come ready from the wizard composable; actions go up as
 * events. Hosts the owner modal in `wizard` mode and moves focus to the share
 * of the owner just linked.
 */
const props = withDefaults(
  defineProps<{
    rows: readonly WizardOwnerRowView[]
    suggestions: readonly OwnerSuggestionView[]
    searchActive: boolean
    searchPending?: boolean
    searchError?: string | null
    newOwnerSuggestion?: string | null
    newOwnerPrefillName?: string
    ownerFormPreset: NonNullable<OwnerFormOptions['presetFarm']>
    shareTotalLabel: string
    shareProgress: number
    isShareComplete: boolean
    errors?: Partial<Record<string, string>>
    alertsBannerVisible?: boolean
    notice?: WizardOwnerNotice | null
  }>(),
  {
    searchPending: false,
    searchError: null,
    newOwnerSuggestion: null,
    newOwnerPrefillName: '',
    errors: () => ({}),
    alertsBannerVisible: false,
    notice: null,
  },
)

const search = defineModel<string>('search', { required: true })

const emit = defineEmits<{
  link: [owner: OwnerSummary]
  register: [result: WizardOwnerFormResult]
  remove: [ownerId: string]
  updateShare: [ownerId: string, text: string]
  touch: [field: string]
  dismissBanner: []
  dismissNotice: []
}>()

const root = ref<HTMLElement | null>(null)
const searchRef = ref<{ focus: () => void } | null>(null)
const isModalOpen = ref(false)
const modalPrefill = ref('')
const totalErrorId = useId()
const emptyId = useId()

const ownersError = computed(() => props.errors.owners ?? null)
const hasRows = computed(() => props.rows.length > 0)
/** With no owner linked, the error belongs to the search; otherwise to the total. */
const isSearchInvalid = computed(() => Boolean(ownersError.value) && !hasRows.value)
const totalError = computed(() => (hasRows.value ? ownersError.value : null))

function focusShare(ownerId: string) {
  const input = root.value?.querySelector<HTMLInputElement>(`[data-share-input="${ownerId}"]`)
  input?.focus()
  input?.select()
}

async function onLink(owner: OwnerSummary) {
  emit('link', owner)
  await nextTick()
  focusShare(owner.id)
}

/** After removing, focus goes to the next card's remove button, else the search. */
async function onRemove(ownerId: string) {
  const index = props.rows.findIndex(row => row.ownerId === ownerId)
  emit('remove', ownerId)
  await nextTick()
  const next = props.rows[Math.min(index, props.rows.length - 1)]
  const button = next ? root.value?.querySelector<HTMLElement>(`[data-remove-owner="${next.ownerId}"]`) : null
  if (button) button.focus()
  else searchRef.value?.focus()
}

function onShareBlur(ownerId: string) {
  emit('touch', `owner:${ownerId}`)
  emit('touch', 'owners')
}

function openModal() {
  modalPrefill.value = props.newOwnerPrefillName
  isModalOpen.value = true
}

/**
 * The modal returns focus to its opener when it unmounts (a timer); linking
 * schedules the share focus after it, so the new card wins.
 */
async function onRegistered(result: OwnerFormResult) {
  if (result.mode !== 'wizard') return
  isModalOpen.value = false
  emit('register', { owner: result.owner, sharePercent: result.sharePercent })
  await nextTick()
  setTimeout(() => focusShare(result.owner.id))
}

// The toast belongs to this visit of the step; it must not come back on return.
onBeforeUnmount(() => {
  if (props.notice) emit('dismissNotice')
})
</script>

<template>
  <div ref="root" class="owners-step">
    <WizardOwnersSearch
      ref="searchRef"
      v-model="search"
      :suggestions="suggestions"
      :is-active="searchActive"
      :pending="searchPending"
      :error-message="searchError"
      :new-owner-label="newOwnerSuggestion"
      :invalid="isSearchInvalid"
      :described-by="isSearchInvalid ? emptyId : undefined"
      @link="onLink"
      @create-new="openModal"
    >
      <template #action>
        <BaseButton variant="secondary" size="sm" icon="plus" aria-label="Cadastrar novo proprietário" @click="openModal">
          Novo
        </BaseButton>
      </template>
    </WizardOwnersSearch>

    <ul v-if="hasRows" class="owners-step__list" aria-label="Proprietários vinculados">
      <li v-for="row in rows" :key="row.ownerId">
        <WizardOwnersShareCard
          :row="row"
          :described-by="totalError ? totalErrorId : undefined"
          @update-share="emit('updateShare', row.ownerId, $event)"
          @blur-share="onShareBlur(row.ownerId)"
          @remove="onRemove(row.ownerId)"
        />
      </li>
    </ul>
    <p
      v-else
      :id="emptyId"
      class="owners-step__empty text-body-md"
      :class="{ 'owners-step__empty--error': isSearchInvalid }"
    >
      {{ isSearchInvalid ? ownersError : 'Nenhum proprietário vinculado ainda. Busque pelo nome ou CPF/CNPJ, ou cadastre um novo.' }}
    </p>

    <WizardOwnersShareTotal
      v-if="hasRows"
      :label="shareTotalLabel"
      :progress="shareProgress"
      :is-complete="isShareComplete"
      :error="totalError"
      :error-id="totalErrorId"
    />

    <ToastAlert
      v-if="alertsBannerVisible"
      tone="agent"
      title="Avisos de janela para os proprietários"
      message="Quem estiver vinculado recebe as janelas aptas pelo WhatsApp. Dá para ajustar depois, no cadastro de cada um."
      dismiss-label="Dispensar aviso"
      @dismiss="emit('dismissBanner')"
    />

    <ToastAlert
      v-if="notice"
      floating
      tone="success"
      :title="notice.title"
      :message="notice.description ?? undefined"
      @dismiss="emit('dismissNotice')"
    />

    <OwnerModal
      v-if="isModalOpen"
      mode="wizard"
      :preset-farm="ownerFormPreset"
      :initial-name="modalPrefill"
      @close="isModalOpen = false"
      @saved="onRegistered"
    />
  </div>
</template>

<style scoped>
.owners-step {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.owners-step__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
  padding: 0;
  list-style: none;
}

.owners-step__empty {
  margin: 0;
  padding: var(--space-20);
  border: var(--border-width-control) dashed var(--color-border);
  border-radius: var(--radius-tile-lg);
  color: var(--color-text-tertiary);
  text-align: center;
}

.owners-step__empty--error {
  border-color: var(--color-status-unsuitable);
  color: var(--color-status-unsuitable);
}
</style>
