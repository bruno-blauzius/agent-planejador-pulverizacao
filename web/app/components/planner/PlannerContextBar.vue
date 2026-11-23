<script setup lang="ts">
/**
 * Context bar of the planner (Figma 18:75): Fazenda, Talhões and Produto with
 * the current choice. Each pill only announces the click; the page opens the
 * picker (menu or dialog) and owns the selection.
 */
export interface ContextBarItem {
  key: string
  /** e.g. "Fazenda". */
  label: string
  /** Current choice, e.g. "Santa Rita". */
  value: string
}

defineProps<{
  items: readonly ContextBarItem[]
  disabled?: boolean
}>()

const emit = defineEmits<{
  select: [key: string]
}>()
</script>

<template>
  <ul class="context-bar" aria-label="Contexto do planejamento">
    <li v-for="item in items" :key="item.key">
      <button
        class="context-bar__pill"
        type="button"
        aria-haspopup="dialog"
        :disabled="disabled"
        @click="emit('select', item.key)"
      >
        <span class="context-bar__label text-caption">{{ item.label }}</span>
        <span class="context-bar__value text-label-meta-strong">{{ item.value }}</span>
        <AppIcon name="chevron-down" :size="14" class="context-bar__chevron" />
      </button>
    </li>
  </ul>
</template>

<style scoped>
.context-bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.context-bar__pill {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  padding: var(--space-10) var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--easing-standard);
}

.context-bar__pill:hover:not(:disabled) {
  background: var(--color-sand);
}

.context-bar__pill:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.context-bar__pill:disabled {
  cursor: not-allowed;
}

.context-bar__label {
  color: var(--color-text-tertiary);
}

.context-bar__value {
  color: var(--color-text-primary);
}

.context-bar__chevron {
  color: var(--color-text-secondary);
}
</style>
