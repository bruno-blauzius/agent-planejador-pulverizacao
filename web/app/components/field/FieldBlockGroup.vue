<script setup lang="ts">
import type { FieldBlockView } from '~/utils/farmDetail'

/**
 * Collapsible block ("Área Norte") of the field list (Figma 15:194; collapsed
 * 15:254): chevron, name and area (or "area · N talhões" when collapsed).
 */
const props = defineProps<{
  block: FieldBlockView
}>()

const emit = defineEmits<{
  toggle: [key: string]
  select: [id: string]
}>()

const listId = useId()
const meta = computed(() => (props.block.expanded ? props.block.areaLabel : props.block.collapsedLabel))
</script>

<template>
  <section class="field-block" :class="{ 'field-block--expanded': block.expanded }">
    <h3 class="field-block__heading">
      <button
        type="button"
        class="field-block__toggle"
        :aria-expanded="block.expanded"
        :aria-controls="listId"
        @click="emit('toggle', block.key)"
      >
        <AppIcon :name="block.expanded ? 'chevron-down' : 'chevron-right'" :size="16" class="field-block__chevron" />
        <span class="field-block__name text-label-strong">{{ block.name }}</span>
        <span class="field-block__meta text-label-chip">{{ meta }}</span>
      </button>
    </h3>
    <ul v-show="block.expanded" :id="listId" class="field-block__list">
      <li v-for="field in block.fields" :key="field.id">
        <FieldListItem :field="field" @select="emit('select', $event)" />
      </li>
    </ul>
  </section>
</template>

<style scoped>
.field-block {
  display: flex;
  flex-direction: column;
}

.field-block--expanded {
  gap: var(--space-6);
}

.field-block__heading {
  margin: 0;
}

.field-block__toggle {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  width: 100%;
  padding: var(--space-6) var(--space-4);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.field-block__toggle:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-block__chevron {
  color: var(--color-text-secondary);
}

.field-block__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.field-block__meta {
  color: var(--color-text-tertiary);
  white-space: nowrap;
}

.field-block__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  margin: 0;
  padding: 0;
  list-style: none;
}
</style>
