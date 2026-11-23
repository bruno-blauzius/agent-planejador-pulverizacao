<script setup lang="ts">
/** Follow-up suggestions below the agent answer (Figma 18:157). */
withDefaults(
  defineProps<{
    suggestions: readonly string[]
    disabled?: boolean
    label?: string
  }>(),
  { disabled: false, label: 'Sugestões de pergunta' },
)

const emit = defineEmits<{
  select: [suggestion: string]
}>()
</script>

<template>
  <ul class="suggestion-chips" :aria-label="label">
    <li v-for="suggestion in suggestions" :key="suggestion">
      <button
        class="suggestion-chips__chip text-label-chip"
        type="button"
        :disabled="disabled"
        @click="emit('select', suggestion)"
      >
        {{ suggestion }}
      </button>
    </li>
  </ul>
</template>

<style scoped>
.suggestion-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.suggestion-chips__chip {
  padding: var(--space-8) var(--space-12);
  border: var(--border-width-hairline) solid var(--color-planner-chip-border);
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--color-green-700);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--easing-standard);
}

.suggestion-chips__chip:hover:not(:disabled) {
  background: var(--color-green-100);
}

.suggestion-chips__chip:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.suggestion-chips__chip:disabled {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}
</style>
