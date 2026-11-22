<script setup lang="ts">
import type { OwnerSummary } from '~/types/owner'
import type { OwnerSuggestionView } from '~/utils/wizardOwners'

/**
 * Owner search of wizard step 3 (Figma 35:978 idle, 41:817 + 41:893 typing).
 * ARIA 1.2 combobox: the input owns a listbox of suggestions plus the
 * "Cadastrar novo" option; arrows move the active option, Enter picks it and
 * Esc closes the list. A polite status announces the result count.
 */
const props = withDefaults(
  defineProps<{
    suggestions: readonly OwnerSuggestionView[]
    /** Enough characters typed: the list is shown. */
    isActive: boolean
    pending?: boolean
    errorMessage?: string | null
    /** "Não achou? Cadastrar “Hel…” como novo proprietário"; null hides the option. */
    newOwnerLabel?: string | null
    /** Blocked "Continuar" with no owner linked lands focus here. */
    invalid?: boolean
    describedBy?: string
  }>(),
  { pending: false, errorMessage: null, newOwnerLabel: null, invalid: false, describedBy: undefined },
)

const search = defineModel<string>({ required: true })

const emit = defineEmits<{
  link: [owner: OwnerSummary]
  createNew: []
}>()

const inputId = useId()
const listboxId = useId()
const statusId = useId()
const input = ref<HTMLInputElement | null>(null)
const activeIndex = ref(-1)
const isDismissed = ref(false)

const isOpen = computed(() => props.isActive && !isDismissed.value)
const optionCount = computed(() => props.suggestions.length + (props.newOwnerLabel ? 1 : 0))
const newOwnerIndex = computed(() => (props.newOwnerLabel ? props.suggestions.length : -1))

function optionId(index: number): string {
  return `${listboxId}-option-${index}`
}

const activeDescendant = computed(() =>
  isOpen.value && activeIndex.value >= 0 ? optionId(activeIndex.value) : undefined,
)

const statusMessage = computed(() => {
  if (!isOpen.value) return ''
  if (props.pending) return 'Buscando proprietários…'
  if (props.errorMessage) return props.errorMessage
  const count = props.suggestions.length
  if (count === 0) return 'Nenhum proprietário encontrado.'
  return count === 1 ? '1 proprietário encontrado.' : `${count} proprietários encontrados.`
})

const hasNoResults = computed(() => !props.pending && !props.errorMessage && props.suggestions.length === 0)

function select(index: number) {
  if (index === newOwnerIndex.value) {
    emit('createNew')
    return
  }
  const suggestion = props.suggestions[index]
  if (suggestion) emit('link', suggestion.owner)
}

function move(step: 1 | -1) {
  if (!props.isActive || optionCount.value === 0) return
  isDismissed.value = false
  const start = activeIndex.value < 0 && step === -1 ? optionCount.value : activeIndex.value
  activeIndex.value = (start + step + optionCount.value) % optionCount.value
}

function onKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      move(1)
      break
    case 'ArrowUp':
      event.preventDefault()
      move(-1)
      break
    case 'Enter':
      // Enter in the search never submits the wizard step.
      event.preventDefault()
      if (isOpen.value && activeIndex.value >= 0) select(activeIndex.value)
      break
    case 'Escape':
      if (!isOpen.value) return
      event.preventDefault()
      event.stopPropagation()
      isDismissed.value = true
      activeIndex.value = -1
      break
  }
}

function focus() {
  input.value?.focus()
}

defineExpose({ focus })

// New text or new results: the list opens again with no active option.
watch(search, () => {
  isDismissed.value = false
  activeIndex.value = -1
})
watch(() => props.suggestions, () => (activeIndex.value = -1))
</script>

<template>
  <div class="owners-search">
    <div class="owners-search__field" :class="{ 'owners-search__field--invalid': invalid }">
      <AppIcon name="search" :size="18" class="owners-search__icon" />
      <label :for="inputId" class="visually-hidden">Buscar proprietário por nome ou CPF/CNPJ</label>
      <input
        :id="inputId"
        ref="input"
        v-model="search"
        class="owners-search__input text-body-md"
        type="search"
        role="combobox"
        autocomplete="off"
        placeholder="Buscar por nome ou CPF/CNPJ já cadastrado…"
        aria-autocomplete="list"
        :aria-expanded="isOpen"
        :aria-controls="listboxId"
        :aria-activedescendant="activeDescendant"
        :aria-describedby="[statusId, describedBy].filter(Boolean).join(' ')"
        :aria-invalid="invalid || undefined"
        :data-focus-on-error="invalid || undefined"
        @keydown="onKeydown"
      >
      <slot name="action" />
    </div>

    <p :id="statusId" class="visually-hidden" role="status" aria-live="polite">{{ statusMessage }}</p>

    <div v-show="isOpen" class="owners-search__panel">
      <p v-if="pending && suggestions.length === 0" class="owners-search__message text-body-sm">Buscando proprietários…</p>
      <p v-else-if="errorMessage" class="owners-search__message owners-search__message--error text-body-sm">{{ errorMessage }}</p>
      <p v-else-if="hasNoResults" class="owners-search__message text-body-sm">Nenhum proprietário encontrado com esse nome ou documento.</p>

      <ul :id="listboxId" class="owners-search__list" role="listbox" aria-label="Proprietários encontrados">
        <li
          v-for="(suggestion, index) in suggestions"
          :id="optionId(index)"
          :key="suggestion.id"
          class="owners-search__option"
          :class="{ 'owners-search__option--active': index === activeIndex }"
          role="option"
          :aria-selected="index === activeIndex"
          @mousedown.prevent
          @mousemove="activeIndex = index"
          @click="select(index)"
        >
          <BaseAvatar :seed="suggestion.id" :initials="suggestion.initials" size="sm" />
          <span class="owners-search__text">
            <span class="owners-search__name text-label-strong">{{ suggestion.name }}</span>
            <span class="owners-search__detail text-body-sm">{{ suggestion.detail }}</span>
          </span>
          <span class="owners-search__link text-label-field" aria-hidden="true">Vincular</span>
        </li>
        <li
          v-if="newOwnerLabel"
          :id="optionId(newOwnerIndex)"
          class="owners-search__option owners-search__option--new"
          :class="{ 'owners-search__option--active': newOwnerIndex === activeIndex }"
          role="option"
          :aria-selected="newOwnerIndex === activeIndex"
          @mousedown.prevent
          @mousemove="activeIndex = newOwnerIndex"
          @click="select(newOwnerIndex)"
        >
          <span class="owners-search__plus" aria-hidden="true"><AppIcon name="plus" :size="16" /></span>
          <span class="owners-search__new-label text-label-field">{{ newOwnerLabel }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.owners-search {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.owners-search__field {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  min-height: var(--size-control-md);
  padding: 0 var(--space-6) 0 var(--space-16);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
}

/* Figma 41:817: the active search has a 2 px green border (keeps the same box size). */
.owners-search__field:focus-within {
  border-color: var(--color-green-700);
  box-shadow: inset 0 0 0 var(--border-width-hairline) var(--color-green-700);
}

.owners-search__field--invalid {
  border-color: var(--color-status-unsuitable);
}

.owners-search__icon {
  flex-shrink: 0;
  color: var(--color-text-tertiary);
}

.owners-search__input {
  flex: 1;
  min-width: 0;
  height: var(--size-control-md);
  border: 0;
  outline: none;
  background: transparent;
  color: var(--color-text-primary);
}

.owners-search__input::placeholder {
  color: var(--color-text-tertiary);
}

.owners-search__input::-webkit-search-cancel-button {
  cursor: pointer;
}

.owners-search__panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-6);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
}

.owners-search__message {
  margin: 0;
  padding: var(--space-10) var(--space-12);
  color: var(--color-text-tertiary);
}

.owners-search__message--error {
  color: var(--color-status-unsuitable);
}

.owners-search__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.owners-search__option {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-tile);
  cursor: pointer;
}

.owners-search__option--active {
  background: var(--color-green-100);
}

.owners-search__option--new {
  gap: var(--space-10);
  border-top: var(--border-width-hairline) solid var(--color-border);
}

.owners-search__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.owners-search__name {
  color: var(--color-text-primary);
}

.owners-search__detail {
  overflow: hidden;
  color: var(--color-text-tertiary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.owners-search__link {
  flex-shrink: 0;
  color: var(--color-green-700);
}

.owners-search__plus {
  display: grid;
  place-items: center;
  width: var(--size-avatar-sm);
  height: var(--size-avatar-sm);
  flex-shrink: 0;
  border-radius: var(--radius-pill);
  background: var(--color-green-800);
  color: var(--color-sprout-400);
}

.owners-search__new-label {
  flex: 1;
  color: var(--color-green-800);
}
</style>
