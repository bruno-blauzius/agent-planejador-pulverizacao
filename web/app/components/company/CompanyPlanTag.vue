<script setup lang="ts">
import type { CompanyPlan } from '~/types/admin'

/**
 * Plan tag (library "Etiqueta" 26:125): Pro green, Básico neutral,
 * Avaliação coffee (Figma 52:982). The label comes ready from the logic.
 */
defineProps<{
  plan: CompanyPlan
  label: string
}>()
</script>

<template>
  <span class="company-plan text-label-badge" :class="`company-plan--${plan}`">{{ label }}</span>
</template>

<style scoped>
.company-plan {
  display: inline-flex;
  padding: var(--padding-badge-y) var(--space-10);
  border-radius: var(--radius-sm);
  white-space: nowrap;
}

.company-plan--pro {
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.company-plan--basic {
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.company-plan--trial {
  background: var(--color-coffee-100);
  color: var(--color-coffee-700);
}
</style>
