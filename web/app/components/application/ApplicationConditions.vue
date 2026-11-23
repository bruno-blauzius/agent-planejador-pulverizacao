<script setup lang="ts">
import type { ConditionsCellView } from '~/utils/applications'

/**
 * "Condições" cell of the applications table (Figma 63:237): the values
 * recorded at the time (Delta T, wind), the live-forecast hint of a planned
 * application, or the rain that cancelled one. The texts come ready.
 */
defineProps<{
  conditions: ConditionsCellView
}>()
</script>

<template>
  <span v-if="conditions.kind === 'values'" class="conditions conditions--values text-label-field">
    <span class="conditions__part">
      <AppIcon name="thermometer" :size="13" class="conditions__icon" />{{ conditions.parts[0] }}
    </span>
    <span v-if="conditions.parts[1]" class="conditions__part">
      <AppIcon name="wind" :size="13" class="conditions__icon" />{{ conditions.parts[1] }}
    </span>
  </span>
  <span v-else-if="conditions.kind === 'rain'" class="conditions conditions--rain text-label-field">
    <AppIcon name="drop" :size="14" />{{ conditions.label }}
  </span>
  <span v-else class="conditions conditions--muted text-body-sm">{{ conditions.label }}</span>
</template>

<style scoped>
.conditions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-4) var(--space-8);
  align-items: center;
}

.conditions__part {
  display: inline-flex;
  gap: var(--space-4);
  align-items: center;
}

.conditions__icon {
  color: var(--color-sprout-600);
}

.conditions--values {
  color: var(--color-text-primary);
}

.conditions--rain {
  color: var(--color-status-unsuitable);
}

.conditions--muted {
  color: var(--color-text-tertiary);
}
</style>
