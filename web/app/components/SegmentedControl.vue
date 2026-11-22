<script setup lang="ts" generic="T extends string">
import type { IconName } from './app/AppIcon.vue'

/**
 * Segmented control (Figma "Segmento" 25:141, instance 39:1066).
 * Single choice among few options; behaves as a radio group
 * (arrow keys move and select, roving tabindex).
 * `variant="outlined"` + options with `icon` = the icon view switch of the
 * farm list (Figma 7:152: white box, green-100 active segment); the label then
 * becomes the accessible name of each icon.
 * `variant="contrast"` = the layer switch over the map (Figma 15:295: white
 * box, green-800 active segment with inverse text).
 */
export interface SegmentOption<V extends string = string> {
  value: V
  label: string
  icon?: IconName
  disabled?: boolean
  /** Why a disabled option is unavailable (accessible name and tooltip). */
  hint?: string
}

const props = withDefaults(
  defineProps<{
    options: readonly SegmentOption<T>[]
    /** Accessible name of the group. */
    label: string
    /** Stretch segments to fill the width (as in the modal 39:1066). */
    block?: boolean
    variant?: 'filled' | 'outlined' | 'contrast'
  }>(),
  { block: false, variant: 'filled' },
)

const model = defineModel<T>({ required: true })

// Keyed by value: refs collected by v-for have no guaranteed order.
const buttonRefs = new Map<T, HTMLElement>()

const enabledOptions = computed(() => props.options.filter((option) => !option.disabled))

/** Option reachable with Tab: the selected one, or the first enabled as fallback. */
const focusableValue = computed<T | undefined>(() => {
  const selected = enabledOptions.value.find((option) => option.value === model.value)
  return (selected ?? enabledOptions.value[0])?.value
})

/** Function ref on v-btn: receives the component instance, whose `$el` is the button. */
function setButtonRef(value: T, instance: unknown) {
  const element = instance && typeof instance === 'object' && '$el' in instance ? instance.$el : instance
  if (element instanceof HTMLElement) buttonRefs.set(value, element)
  else buttonRefs.delete(value)
}

function select(value: T) {
  model.value = value
}

function move(step: 1 | -1) {
  const options = enabledOptions.value
  if (options.length === 0) return
  const currentIndex = options.findIndex((option) => option.value === focusableValue.value)
  const next = options[(currentIndex + step + options.length) % options.length]
  if (!next) return
  select(next.value)
  buttonRefs.get(next.value)?.focus()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
    event.preventDefault()
    move(1)
  } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
    event.preventDefault()
    move(-1)
  }
}
</script>

<template>
  <!--
    Facade over v-btn-toggle (ADR 0011). Keeps radio-group semantics: one choice,
    arrow keys move and select, roving tabindex.
  -->
  <v-btn-toggle
    :model-value="model"
    class="segmented"
    :class="[`segmented--${variant}`, { 'segmented--block': block }]"
    role="radiogroup"
    :aria-label="label"
    mandatory
    @update:model-value="select"
    @keydown="onKeydown"
  >
    <v-btn
      v-for="option in options"
      :key="option.value"
      :ref="(instance) => setButtonRef(option.value, instance)"
      :value="option.value"
      role="radio"
      class="segmented__option"
      :class="{ 'segmented__option--active': option.value === model, 'segmented__option--icon': option.icon }"
      :aria-label="option.icon ? option.label : undefined"
      :title="option.icon ? option.label : option.hint"
      :aria-checked="option.value === model"
      :aria-pressed="undefined"
      :tabindex="option.value === focusableValue ? 0 : -1"
      :disabled="option.disabled"
      :ripple="false"
      variant="text"
    >
      <AppIcon v-if="option.icon" :name="option.icon" :size="18" />
      <template v-else>{{ option.label }}</template>
      <span v-if="option.disabled && option.hint" class="visually-hidden">({{ option.hint }})</span>
    </v-btn>
  </v-btn-toggle>
</template>

<style scoped>
.segmented {
  display: inline-flex;
  gap: var(--space-4);
  height: auto;
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  overflow: visible;
}

.segmented--block {
  display: flex;
  width: 100%;
}

.segmented__option :deep(.v-btn__overlay),
.segmented__option :deep(.v-btn__underlay) {
  display: none;
}

.segmented__option {
  display: inline-flex;
  min-width: 0;
  height: auto;
  letter-spacing: 0;
  opacity: 1;
  align-items: center;
  justify-content: center;
  min-height: var(--size-segment);
  padding: var(--padding-segment-y) var(--space-14);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-secondary);
  font: var(--font-label-field);
  white-space: nowrap;
  cursor: pointer;
}

.segmented--block .segmented__option {
  flex: 1;
  min-width: 0;
}

.segmented__option:hover:not(.segmented__option--active, :disabled) {
  color: var(--color-text-primary);
}

.segmented__option:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.segmented__option:disabled {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.segmented__option--active {
  background: var(--color-white);
  box-shadow: var(--shadow-segment);
  color: var(--color-green-800);
  font: var(--font-label-strong);
}
.segmented--outlined {
  gap: var(--space-2);
  border: var(--border-width-hairline) solid var(--color-border);
  background: var(--color-white);
}

.segmented__option--icon {
  min-height: 0;
  padding: var(--space-8);
}

.segmented--outlined .segmented__option--active {
  background: var(--color-green-100);
  box-shadow: none;
}
.segmented--contrast {
  gap: var(--space-6);
  padding: var(--padding-segment-contrast);
  background: var(--color-white);
}

.segmented--contrast .segmented__option {
  min-height: 0;
  padding: var(--padding-segment-contrast-option);
  font: var(--font-label-chip);
}

.segmented--contrast .segmented__option--active {
  background: var(--color-green-800);
  box-shadow: none;
  color: var(--color-text-inverse);
  font: var(--font-label-chip);
}
</style>
