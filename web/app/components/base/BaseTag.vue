<script setup lang="ts">
/**
 * Short metadata tag (library "Etiqueta" 26:125: area, crop, count, role).
 * Tones: neutral (sand), green, coffee. Static text, not interactive.
 */
export type TagTone = 'neutral' | 'green' | 'coffee'

withDefaults(
  defineProps<{
    label: string
    tone?: TagTone
  }>(),
  { tone: 'neutral' },
)
</script>

<template>
  <span class="base-tag text-label-badge" :class="`base-tag--${tone}`">{{ label }}</span>
</template>

<style scoped>
.base-tag {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  padding: var(--padding-badge-y) var(--space-10);
  overflow: hidden;
  border-radius: var(--radius-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.base-tag--neutral {
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.base-tag--green {
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.base-tag--coffee {
  background: var(--color-coffee-100);
  color: var(--color-coffee-700);
}
</style>
