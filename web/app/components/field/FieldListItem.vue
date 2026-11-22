<script setup lang="ts">
import { fieldColor } from '~/theme/map-style'
import type { FieldRowView } from '~/utils/farmDetail'

/**
 * Field row of the "Áreas e talhões" list (Figma 15:200; selected 15:216):
 * crop color bar (same slot as the map), name, cultivar · area and the spray
 * status from the API. Selecting it highlights the field on the map.
 */
const props = defineProps<{
  field: FieldRowView
}>()

const emit = defineEmits<{
  select: [id: string]
}>()

const swatchStyle = computed(() => ({ backgroundColor: fieldColor(props.field.colorIndex) }))
</script>

<template>
  <button
    type="button"
    class="field-item"
    :class="{ 'field-item--selected': field.selected }"
    :aria-pressed="field.selected"
    :data-field-id="field.id"
    @click="emit('select', field.id)"
  >
    <!-- Data-driven color: the crop slot is only known at runtime. -->
    <span class="field-item__swatch" :style="swatchStyle" aria-hidden="true" />
    <span class="field-item__text">
      <span class="field-item__name text-label-meta-strong">{{ field.name }}</span>
      <span class="field-item__subtitle text-caption">{{ field.subtitle }}</span>
    </span>
    <StatusBadge v-if="field.condition" :tone="field.condition.value" :label="field.condition.label" />
  </button>
</template>

<style scoped>
.field-item {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  width: 100%;
  padding: var(--space-10);
  border: var(--border-width-control) solid transparent;
  border-radius: var(--radius-md);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.field-item:hover:not(.field-item--selected) {
  background: var(--color-sand);
}

.field-item:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-item--selected {
  border-color: var(--color-green-700);
  background: var(--color-green-100);
}

.field-item__swatch {
  flex-shrink: 0;
  width: var(--size-field-swatch-width);
  height: var(--size-field-swatch-height);
  border-radius: var(--radius-field-swatch);
}

.field-item__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.field-item__name,
.field-item__subtitle {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.field-item__name {
  color: var(--color-text-primary);
}

.field-item__subtitle {
  color: var(--color-text-secondary);
}
</style>
