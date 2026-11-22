<script setup lang="ts" generic="T extends string">
import type { IconName } from './app/AppIcon.vue'

/**
 * Group of option cards: a fieldset with legend and one choice (native radios,
 * arrow keys handled by the browser). See OptionCard.
 */
export interface OptionCardItem<V extends string = string> {
  value: V
  title: string
  description: string
  icon: IconName
  unavailableHint?: string
}

withDefaults(
  defineProps<{
    options: readonly OptionCardItem<T>[]
    label: string
    name?: string
    variant?: 'default' | 'compact'
    /** Hides the legend visually (kept for assistive tech) when a heading already names the group. */
    hideLabel?: boolean
  }>(),
  { name: undefined, variant: 'default', hideLabel: false },
)

const model = defineModel<T | null>({ default: null })

const generatedName = useId()
</script>

<template>
  <fieldset class="option-group">
    <legend class="option-group__legend text-label-field" :class="{ 'visually-hidden': hideLabel }">{{ label }}</legend>
    <div class="option-group__list">
      <OptionCard
        v-for="option in options"
        :key="option.value"
        v-model="model"
        :value="option.value"
        :title="option.title"
        :description="option.description"
        :icon="option.icon"
        :unavailable-hint="option.unavailableHint"
        :name="name ?? generatedName"
        :variant="variant"
      />
    </div>
  </fieldset>
</template>

<style scoped>
.option-group {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

/* Floated so the margin applies (legends ignore flex gap). */
.option-group__legend {
  float: left;
  width: 100%;
  margin-bottom: var(--space-24);
  padding: 0;
  color: var(--color-text-primary);
}

.option-group__list {
  display: flex;
  clear: both;
  flex-direction: column;
  gap: var(--space-10);
}
</style>
