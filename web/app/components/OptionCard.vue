<script setup lang="ts" generic="T extends string">
import type { IconName } from './app/AppIcon.vue'

/**
 * Option card ("Cartão de opção", library 25:160; 52:790 role picker, 14:201
 * perimeter method): one native radio with icon tile, title and description.
 * Selected = green-100 with a 2 px green-700 border. With `unavailableHint`
 * the option exists but cannot be chosen yet ("Em breve").
 * `variant="compact"` is the denser wizard version (13 px title, no radio dot).
 */
const props = withDefaults(
  defineProps<{
    value: T
    title: string
    description: string
    icon: IconName
    name: string
    unavailableHint?: string
    variant?: 'default' | 'compact'
  }>(),
  { unavailableHint: undefined, variant: 'default' },
)

const model = defineModel<T | null>({ default: null })

const isSelected = computed(() => model.value === props.value && !props.unavailableHint)
</script>

<template>
  <label
    class="option-card"
    :class="[`option-card--${variant}`, {
      'option-card--selected': isSelected,
      'option-card--unavailable': unavailableHint,
    }]"
  >
    <input
      v-model="model"
      type="radio"
      class="visually-hidden"
      :name="name"
      :value="value"
      :disabled="Boolean(unavailableHint)"
    >
    <span class="option-card__icon" aria-hidden="true">
      <AppIcon :name="icon" :size="18" />
    </span>
    <span class="option-card__text">
      <span
        class="option-card__title"
        :class="variant === 'compact' ? 'text-label-meta-strong' : 'text-label-strong'"
      >{{ title }}</span>
      <span
        class="option-card__description"
        :class="variant === 'compact' ? 'text-caption' : 'text-body-sm'"
      >{{ description }}</span>
    </span>
    <span v-if="unavailableHint" class="option-card__hint text-label-tag">{{ unavailableHint }}</span>
    <span v-else-if="variant === 'default'" class="option-card__radio" aria-hidden="true" />
  </label>
</template>

<style scoped>
.option-card {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
  background: var(--color-white);
  cursor: pointer;
  transition:
    background-color var(--duration-fast) var(--easing-standard),
    border-color var(--duration-fast) var(--easing-standard);
}

.option-card:hover:not(.option-card--selected, .option-card--unavailable) {
  border-color: var(--color-green-700);
}

.option-card:has(input:focus-visible) {
  box-shadow: var(--focus-ring);
}

/* Figma: 2 px border; the extra width is an inset shadow to avoid layout shift. */
.option-card--selected {
  border-color: var(--color-green-700);
  box-shadow: inset 0 0 0 var(--border-width-hairline) var(--color-green-700);
  background: var(--color-green-100);
}

.option-card--selected:has(input:focus-visible) {
  box-shadow:
    inset 0 0 0 var(--border-width-hairline) var(--color-green-700),
    var(--focus-ring);
}

.option-card--unavailable {
  cursor: not-allowed;
}

.option-card__icon {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.option-card--selected .option-card__icon {
  background: var(--color-green-800);
  color: var(--color-sprout-400);
}

.option-card__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.option-card__title {
  color: var(--color-text-primary);
}

.option-card__description {
  color: var(--color-text-tertiary);
}

.option-card--unavailable .option-card__title {
  color: var(--color-text-tertiary);
}

.option-card__hint {
  flex-shrink: 0;
  padding: var(--space-2) var(--space-8);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

/* Radio indicator of the library card (52:790). */
.option-card__radio {
  width: var(--size-checkbox);
  height: var(--size-checkbox);
  flex-shrink: 0;
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-white);
}

.option-card--selected .option-card__radio {
  border: var(--radio-dot-ring) solid var(--color-green-800);
}
</style>
