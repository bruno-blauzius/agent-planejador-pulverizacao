<script setup lang="ts">
/**
 * Status of an application with a colored dot (Figma 63:237 "Planejada",
 * "Concluída", "Cancelada"). The label and tone come ready from the logic;
 * `muted` (cancelled) is drawn in the "inapta" red, as in the Figma row.
 */
export type ApplicationTone = 'suitable' | 'attention' | 'info' | 'muted'

defineProps<{
  tone: ApplicationTone
  label: string
}>()
</script>

<template>
  <span class="status-pill text-label-badge" :class="`status-pill--${tone}`">
    <span class="status-pill__dot" aria-hidden="true" />
    {{ label }}
  </span>
</template>

<style scoped>
.status-pill {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  padding: var(--padding-badge-y) var(--space-12) var(--padding-badge-y) var(--space-10);
  border-radius: var(--radius-pill);
  white-space: nowrap;
}

.status-pill__dot {
  width: var(--space-8);
  height: var(--space-8);
  border-radius: var(--radius-pill);
  background: currentcolor;
}

.status-pill--suitable {
  background: var(--color-status-suitable-bg);
  color: var(--color-status-suitable);
}

.status-pill--attention {
  background: var(--color-status-attention-bg);
  color: var(--color-status-attention);
}

.status-pill--info {
  background: var(--color-info-rain-bg);
  color: var(--color-info-rain);
}

.status-pill--muted {
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}
</style>
