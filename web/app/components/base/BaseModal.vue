<script setup lang="ts">
import type { IconName } from '../app/AppIcon.vue'

export type ModalSize = 'sm' | 'md'

/**
 * Modal dialog over an overlay (Figma 39:1044 overlay + 39:1045 modal).
 * Accessible: role="dialog", aria-modal, labelled by the title, Esc closes,
 * focus moves into the dialog, Tab is trapped and focus returns on close.
 */
const props = withDefaults(
  defineProps<{
    title: string
    eyebrow?: string
    description?: string
    icon?: IconName
    /** Named width: md (640, Figma 39:1045) or sm (short notices). */
    size?: ModalSize
    /** Explicit width in px; overrides `size`. */
    width?: number
    closeLabel?: string
    closeOnOverlay?: boolean
  }>(),
  {
    eyebrow: undefined,
    description: undefined,
    icon: undefined,
    size: 'md',
    width: undefined,
    closeLabel: 'Fechar',
    closeOnOverlay: true,
  },
)

const open = defineModel<boolean>('open', { default: false })

const emit = defineEmits<{
  close: []
}>()

defineSlots<{
  default?: () => unknown
  actions?: () => unknown
}>()

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

// Widths come from the tokens (--modal-width-*); v-dialog accepts any CSS length.
const maxWidth = computed(() => (props.width ? `${props.width}px` : `var(--modal-width-${props.size})`))

const titleId = useId()
const descriptionId = useId()
const cardRef = ref<{ $el: HTMLElement } | null>(null)
const titleRef = ref<HTMLElement | null>(null)
let previouslyFocused: HTMLElement | null = null

function close() {
  open.value = false
  emit('close')
}

/** v-dialog asks to close on Esc or outside click (outside click is off when `closeOnOverlay` is false). */
function onDialogUpdate(value: boolean) {
  if (!value) close()
}

/** `persistent` also blocks Esc in Vuetify; Esc must still close the modal. */
function onEscape() {
  if (!props.closeOnOverlay) close()
}

// Captured synchronously when opening, before Vuetify moves focus into the dialog.
watch(open, (isOpen) => {
  if (isOpen && import.meta.client) previouslyFocused = document.activeElement as HTMLElement | null
}, { flush: 'sync' })

/**
 * Vuetify traps focus inside the dialog; this moves it to the field marked with
 * `data-autofocus` (it may sit on a wrapper such as the TextField root).
 */
function onAfterEnter() {
  const root = cardRef.value?.$el
  const marked = root?.querySelector<HTMLElement>('[data-autofocus]')
  const target = marked?.matches(FOCUSABLE) ? marked : marked?.querySelector<HTMLElement>(FOCUSABLE)
  // Next task: runs after Vuetify's own initial focus inside the dialog
  // (a timer, not rAF, so it also runs in background tabs).
  if (target) setTimeout(() => target.focus())
}

/*
 * Mounted already open (e.g. a modal rendered with v-if from a route query):
 * v-dialog plays no enter transition, so capture the opener and focus here.
 */
onMounted(() => {
  if (!open.value) return
  previouslyFocused = document.activeElement as HTMLElement | null
  nextTick(() => setTimeout(onAfterEnter))
})

/*
 * Removed by the host (v-if) before or during the leave transition: after-leave
 * never runs, so the opener gets the focus back here. after-leave clears
 * `previouslyFocused`, so focus is never restored twice.
 */
onBeforeUnmount(() => {
  const target = previouslyFocused
  previouslyFocused = null
  if (target?.isConnected) setTimeout(() => target.focus())
})

/** Moves focus to the title, e.g. when the content changes to another step. */
function focusTitle() {
  nextTick(() => titleRef.value?.focus())
}

defineExpose({ focusTitle })

/** Without an activator Vuetify does not restore focus, so the opener gets it back here. */
async function onAfterLeave() {
  const target = previouslyFocused
  previouslyFocused = null
  await nextTick()
  target?.focus()
}
</script>

<template>
  <!-- Facade over v-dialog + v-card (ADR 0011). -->
  <v-dialog
    :model-value="open"
    :max-width="maxWidth"
    :persistent="!closeOnOverlay"
    :aria-labelledby="titleId"
    :aria-describedby="description ? descriptionId : undefined"
    class="base-modal-overlay"
    scrim="green-950"
    @update:model-value="onDialogUpdate"
    @after-enter="onAfterEnter"
    @after-leave="onAfterLeave"
    @keydown.esc="onEscape"
  >
    <v-card ref="cardRef" class="modal" :elevation="0" :border="false">
      <header class="modal__header">
        <span v-if="icon" class="modal__icon" aria-hidden="true">
          <AppIcon :name="icon" :size="24" />
        </span>
        <div class="modal__heading">
          <p v-if="eyebrow" class="modal__eyebrow text-overline">{{ eyebrow }}</p>
          <h2 :id="titleId" ref="titleRef" class="modal__title text-h3" tabindex="-1">{{ title }}</h2>
          <p v-if="description" :id="descriptionId" class="modal__description text-body-md">
            {{ description }}
          </p>
        </div>
        <button type="button" class="modal__close" :aria-label="closeLabel" @click="close">
          <AppIcon name="x" :size="18" />
        </button>
      </header>

      <div class="modal__body">
        <slot />
      </div>

      <footer v-if="$slots.actions" class="modal__actions">
        <slot name="actions" />
      </footer>
    </v-card>
  </v-dialog>
</template>

<style scoped>
.modal {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  width: 100%;
  padding: var(--space-32);
  border: 0;
  border-radius: var(--radius-2xl);
  background: var(--color-white);
  box-shadow: var(--shadow-modal);
}

.modal:focus {
  outline: none;
}

.modal :deep(> .v-card__overlay),
.modal :deep(> .v-card__underlay) {
  display: none;
}

.modal__header {
  display: flex;
  gap: var(--space-14);
  align-items: flex-start;
}

.modal__icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-lg);
  height: var(--size-icon-tile-lg);
  flex-shrink: 0;
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-900);
  color: var(--color-sprout-400);
}

.modal__heading {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.modal__eyebrow {
  color: var(--color-coffee-600);
}

.modal__title:focus {
  outline: none;
}

.modal__title {
  color: var(--color-text-primary);
}

.modal__description {
  color: var(--color-text-secondary);
}

.modal__close {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  flex-shrink: 0;
  border: 0;
  border-radius: var(--radius-md);
  background: var(--color-sand);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.modal__close:hover {
  background: var(--color-border);
}

.modal__close:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.modal__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

.modal__actions {
  display: flex;
  gap: var(--space-10);
}
</style>

<style>
/*
 * The overlay is teleported by Vuetify, outside this component's scoped styles.
 * Figma overlay (39:1044): green-950 at 55%.
 */
.base-modal-overlay > .v-overlay__scrim {
  background: var(--color-overlay);
  opacity: 1;
}
</style>
