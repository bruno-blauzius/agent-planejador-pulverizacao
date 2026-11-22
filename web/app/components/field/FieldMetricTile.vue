<script setup lang="ts">
/**
 * Sand tile with a small label over a value: summary of the field panel
 * (Figma 15:185, `md`) and the metrics of the field popover (15:343, `sm`).
 */
withDefaults(
  defineProps<{
    label: string
    value: string
    size?: 'md' | 'sm'
  }>(),
  { size: 'md' },
)
</script>

<template>
  <div class="field-metric" :class="`field-metric--${size}`">
    <dt class="field-metric__label" :class="size === 'md' ? 'text-label-tag' : 'text-label-micro'">{{ label }}</dt>
    <dd class="field-metric__value" :class="size === 'md' ? 'text-h4' : 'text-label-meta-strong'">{{ value }}</dd>
  </div>
</template>

<style scoped>
.field-metric {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  background: var(--color-sand);
}

.field-metric--md {
  padding: var(--space-12);
  border-radius: var(--radius-md);
}

/* Tiles share the 244 px popover row by content so "há 66 dias" fits on one line. */
.field-metric--sm {
  flex: 1 1 auto;
  padding: var(--space-10) var(--space-8);
  border-radius: var(--radius-tile);
}

.field-metric__label {
  color: var(--color-text-tertiary);
}

.field-metric__value {
  margin: 0;
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
