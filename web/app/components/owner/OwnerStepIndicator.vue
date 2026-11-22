<script setup lang="ts">
import type { OwnerFormStep } from '~/composables/useOwnerForm'

/**
 * Steps of the owner modal (Figma 39:1059 / 39:1374): current step dark,
 * upcoming step sand, completed step green-100 with a check.
 */
const props = defineProps<{
  step: OwnerFormStep
}>()

const STEPS: readonly { value: OwnerFormStep, label: string }[] = [
  { value: 'data', label: 'Dados' },
  { value: 'links', label: 'Vínculos' },
]

const currentIndex = computed(() => STEPS.findIndex((item) => item.value === props.step))

function stateOf(index: number): 'done' | 'current' | 'upcoming' {
  if (index < currentIndex.value) return 'done'
  return index === currentIndex.value ? 'current' : 'upcoming'
}
</script>

<template>
  <ol class="owner-steps" aria-label="Etapas">
    <li v-for="(item, index) in STEPS" :key="item.value" class="owner-steps__item">
      <AppIcon v-if="index > 0" name="chevron-right" :size="14" class="owner-steps__separator" />
      <span
        class="owner-steps__pill text-label-badge"
        :class="`owner-steps__pill--${stateOf(index)}`"
        :aria-current="stateOf(index) === 'current' ? 'step' : undefined"
      >
        <AppIcon v-if="stateOf(index) === 'done'" name="check" :size="14" />
        <template v-if="stateOf(index) === 'done'">{{ item.label }}</template>
        <template v-else>{{ index + 1 }} · {{ item.label }}</template>
        <span v-if="stateOf(index) === 'done'" class="visually-hidden">(concluída)</span>
      </span>
    </li>
  </ol>
</template>

<style scoped>
.owner-steps {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.owner-steps__item {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
}

.owner-steps__separator {
  color: var(--color-text-tertiary);
}

.owner-steps__pill {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-6) var(--space-10);
  border-radius: var(--radius-pill);
}

.owner-steps__pill--current {
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.owner-steps__pill--upcoming {
  background: var(--color-sand);
  color: var(--color-text-tertiary);
}

.owner-steps__pill--done {
  background: var(--color-green-100);
  color: var(--color-green-700);
}
</style>
