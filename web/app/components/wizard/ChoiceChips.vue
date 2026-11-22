<script setup lang="ts" generic="T extends string">
/**
 * Selection chips (Figma "Chip de seleção" 25:167; instance 35:661).
 * Quick single choice among a few visible options. Native radio inputs inside a
 * fieldset keep keyboard (arrows), form semantics and the group name.
 */
export interface ChoiceChipOption<V extends string = string> {
  value: V
  label: string
  /** Decorative emoji shown before the label (hidden from assistive tech). */
  emoji?: string
  disabled?: boolean
}

withDefaults(
  defineProps<{
    options: readonly ChoiceChipOption<T>[]
    /** Visible group label (fieldset legend). */
    label: string
    /** Radio group name; defaults to a generated id. */
    name?: string
    error?: string
    required?: boolean
  }>(),
  { name: undefined, error: undefined, required: false },
)

const model = defineModel<T | null>({ default: null })

const generatedName = useId()
const errorId = `${generatedName}-error`
</script>

<template>
  <fieldset
    class="choice-chips"
    :aria-describedby="error ? errorId : undefined"
    :aria-invalid="error ? true : undefined"
  >
    <legend class="choice-chips__legend text-label-field">
      {{ label }}
      <span v-if="required" class="choice-chips__required" aria-hidden="true">*</span>
    </legend>
    <div class="choice-chips__list">
      <label
        v-for="option in options"
        :key="option.value"
        class="choice-chips__chip text-label-field"
        :class="{
          'choice-chips__chip--selected': option.value === model,
          'choice-chips__chip--disabled': option.disabled,
        }"
      >
        <input
          v-model="model"
          type="radio"
          class="visually-hidden"
          :name="name ?? generatedName"
          :value="option.value"
          :disabled="option.disabled"
          :required="required"
          :aria-invalid="error ? true : undefined"
        >
        <AppIcon v-if="option.value === model" name="check" :size="14" />
        <span v-if="option.emoji" aria-hidden="true">{{ option.emoji }}</span>
        {{ option.label }}
      </label>
    </div>
    <p v-if="error" :id="errorId" class="choice-chips__error text-body-sm" role="alert">{{ error }}</p>
  </fieldset>
</template>

<style scoped>
/* Legend is floated so the margin below it applies (legends ignore flex gap). */
.choice-chips {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.choice-chips__legend {
  float: left;
  width: 100%;
  margin-bottom: var(--space-24);
  padding: 0;
  color: var(--color-text-primary);
}

.choice-chips__required {
  color: var(--color-status-unsuitable);
}

.choice-chips__list {
  display: flex;
  clear: both;
  flex-wrap: wrap;
  gap: var(--space-8);
}

.choice-chips__chip {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-10) var(--space-14);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-secondary);
  white-space: nowrap;
  cursor: pointer;
  transition:
    background-color var(--duration-fast) var(--easing-standard),
    border-color var(--duration-fast) var(--easing-standard);
}

.choice-chips__chip:hover:not(.choice-chips__chip--selected, .choice-chips__chip--disabled) {
  color: var(--color-text-primary);
}

.choice-chips__chip:has(input:focus-visible) {
  box-shadow: var(--focus-ring);
}

.choice-chips__chip--selected {
  border-color: var(--color-green-800);
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.choice-chips__chip--disabled {
  background: var(--color-sand);
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.choice-chips__error {
  margin: var(--space-8) 0 0;
  color: var(--color-status-unsuitable);
}
</style>
