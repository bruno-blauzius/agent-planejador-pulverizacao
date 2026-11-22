<script setup lang="ts">
import type { IconName } from '../app/AppIcon.vue'

/**
 * Action button (Figma "Botão", 2:186).
 * primary: main action of the screen (one per screen) · accent: agent/AI actions ·
 * secondary / ghost: support · danger: destructive actions.
 * Sizes: lg 56 (mobile/field), md 48 (default), sm 36 (tables).
 * lg-compact: 56 px tall with the md radius and padding, as the full-width
 * form buttons of the login (instances 6:104 and 34:355).
 * `unavailableHint` ("Em breve"): the action exists but cannot be used yet. The
 * button stays focusable with `aria-disabled` and the reason is part of its
 * accessible name (visually hidden) and of the tooltip.
 */
export type ButtonVariant = 'primary' | 'accent' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'lg' | 'lg-compact' | 'md' | 'sm'

const props = withDefaults(
  defineProps<{
    variant?: ButtonVariant
    size?: ButtonSize
    icon?: IconName
    type?: 'button' | 'submit' | 'reset'
    disabled?: boolean
    loading?: boolean
    block?: boolean
    /** Renders a NuxtLink instead of a <button>. */
    to?: string
    /** Why the action is unavailable; renders the button disabled but focusable. */
    unavailableHint?: string
  }>(),
  {
    variant: 'primary',
    size: 'md',
    icon: undefined,
    type: 'button',
    disabled: false,
    loading: false,
    block: false,
    to: undefined,
    unavailableHint: undefined,
  },
)

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

defineSlots<{
  default?: () => unknown
}>()

const ICON_SIZE: Record<ButtonSize, number> = { 'lg': 20, 'lg-compact': 20, 'md': 18, 'sm': 16 }

const isUnavailable = computed(() => Boolean(props.unavailableHint))
const isInactive = computed(() => props.disabled || props.loading || isUnavailable.value)
const iconSize = computed(() => ICON_SIZE[props.size])
const classes = computed(() => [
  'base-button',
  `base-button--${props.variant}`,
  `base-button--${props.size}`,
  { 'base-button--block': props.block, 'base-button--inactive': isInactive.value },
])

function onClick(event: MouseEvent) {
  if (isInactive.value) {
    event.preventDefault()
    return
  }
  emit('click', event)
}
</script>

<template>
  <!-- Facade over v-btn (ADR 0011): the visual comes from the scoped styles below. -->
  <v-btn
    :class="classes"
    :to="to && !isInactive ? to : undefined"
    :type="to ? undefined : type"
    :disabled="isInactive && !isUnavailable"
    :aria-disabled="isUnavailable || undefined"
    :title="unavailableHint"
    :aria-busy="loading || undefined"
    :ripple="false"
    variant="flat"
    @click="onClick"
  >
    <template v-if="loading || icon" #prepend>
      <span v-if="loading" class="base-button__spinner" aria-hidden="true" />
      <AppIcon v-else-if="icon" :name="icon" :size="iconSize" />
    </template>
    <span class="base-button__label"><slot /></span>
    <span v-if="unavailableHint" class="visually-hidden">({{ unavailableHint }})</span>
  </v-btn>
</template>

<style scoped>
.base-button {
  --button-gap: var(--space-8);

  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  min-width: 0;
  border: var(--border-width-control) solid transparent;
  letter-spacing: 0;
  white-space: nowrap;
  text-decoration: none;
  cursor: pointer;
  transition:
    background-color var(--duration-fast) var(--easing-standard),
    border-color var(--duration-fast) var(--easing-standard);
}

.base-button:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

/* Vuetify's Material state layer and icon offsets are replaced by the Figma states below. */
.base-button :deep(.v-btn__overlay),
.base-button :deep(.v-btn__underlay) {
  display: none;
}

.base-button :deep(.v-btn__prepend) {
  margin-inline: 0 var(--button-gap);
}

.base-button.v-btn--disabled {
  opacity: 1;
}

/* Fills a column alone, or shares a row equally with sibling block buttons. */
.base-button--block {
  /* Basis auto (not 0): a 0 basis would collapse the height inside column flex containers. */
  flex: 1 1 auto;
  width: 100%;
  min-width: 0;
}

/* Sizes */
.base-button--lg {
  --button-gap: var(--space-10);

  height: var(--size-control-lg);
  padding: 0 var(--space-28);
  border-radius: var(--radius-lg);
  font: var(--font-label-button);
}

.base-button--lg-compact {
  height: var(--size-control-lg);
  padding: 0 var(--space-22);
  border-radius: var(--radius-md);
  font: var(--font-label-button);
}

.base-button--md {
  height: var(--size-control-md);
  padding: 0 var(--space-22);
  border-radius: var(--radius-md);
  font: var(--font-label-button);
}

.base-button--sm {
  --button-gap: var(--space-6);

  height: var(--size-control-sm);
  padding: 0 var(--space-14);
  border-radius: var(--radius-sm);
  font: var(--font-label-button-sm);
}

/* Variants */
.base-button--primary {
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.base-button--primary:hover {
  background: var(--color-green-900);
}

.base-button--accent {
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.base-button--accent:hover {
  background: var(--color-sprout-500);
}

.base-button--secondary {
  border-color: var(--color-border);
  background: var(--color-white);
  color: var(--color-green-800);
}

.base-button--secondary:hover {
  background: var(--color-sand);
}

.base-button--ghost {
  background: transparent;
  color: var(--color-green-700);
}

.base-button--ghost:hover {
  background: var(--color-green-100);
}

.base-button--danger {
  background: var(--color-status-unsuitable);
  color: var(--color-text-inverse);
}

.base-button--danger:hover {
  background: var(--color-status-unsuitable-hover);
}

/* Disabled (Figma "Estado=Desabilitado") */
.base-button--inactive,
.base-button--inactive:hover {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.base-button--primary.base-button--inactive,
.base-button--accent.base-button--inactive,
.base-button--danger.base-button--inactive {
  background: var(--color-border);
}

.base-button--secondary.base-button--inactive {
  background: var(--color-white);
}

.base-button--ghost.base-button--inactive {
  background: transparent;
}

.base-button__spinner {
  width: var(--size-spinner);
  height: var(--size-spinner);
  border: var(--border-width-strong) solid currentColor;
  border-right-color: transparent;
  border-radius: var(--radius-pill);
  animation: base-button-spin var(--duration-spinner) linear infinite;
}

@keyframes base-button-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .base-button__spinner {
    animation-duration: 2s;
  }
}
</style>
