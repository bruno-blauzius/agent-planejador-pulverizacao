<script setup lang="ts">
import type { FieldFormStep } from '~/utils/fieldForm'

/**
 * Steps of the field form (Figma 16:429): "Desenho" › "2 · Detalhes".
 * A done step is a button that goes back to it; the active one is marked
 * with aria-current.
 */
export interface FieldFormStepView {
  readonly value: FieldFormStep
  readonly label: string
  readonly done: boolean
  readonly active: boolean
}

defineProps<{
  steps: readonly FieldFormStepView[]
}>()

const emit = defineEmits<{
  select: [step: FieldFormStep]
}>()
</script>

<template>
  <ol class="field-steps" aria-label="Etapas do cadastro">
    <li v-for="(step, index) in steps" :key="step.value" class="field-steps__item">
      <AppIcon v-if="index > 0" name="chevron-right" :size="14" class="field-steps__separator" />
      <button
        v-if="step.done"
        type="button"
        class="field-steps__chip field-steps__chip--done text-label-badge"
        @click="emit('select', step.value)"
      >
        <AppIcon name="check" :size="14" />
        {{ step.label }}
        <span class="visually-hidden">(concluída, voltar)</span>
      </button>
      <span
        v-else
        class="field-steps__chip text-label-badge"
        :class="{ 'field-steps__chip--active': step.active }"
        :aria-current="step.active ? 'step' : undefined"
      >
        {{ step.label }}
      </span>
    </li>
  </ol>
</template>

<style scoped>
.field-steps {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.field-steps__item {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
}

.field-steps__separator {
  color: var(--color-text-tertiary);
}

.field-steps__chip {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-6) var(--space-10);
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.field-steps__chip--done {
  background: var(--color-green-100);
  color: var(--color-green-700);
  cursor: pointer;
}

.field-steps__chip--done:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-steps__chip--active {
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}
</style>
