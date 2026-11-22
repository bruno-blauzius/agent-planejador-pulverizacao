<script setup lang="ts">
import type { FieldPopoverView } from '~/utils/farmDetail'

/**
 * Popover of the selected field on the farm map (Figma 15:334): area,
 * planting and last application, the day's spray window and the actions.
 * The window comes from the spray-planner (not modeled yet): the strip says
 * so instead of showing a time. "Editar" opens the field form (screen 10);
 * "Planejar aplicação" stays disabled with its hint until the planner exists.
 */
const props = withDefaults(
  defineProps<{
    field: FieldPopoverView
    /** Shows "Editar" (registry.write, ADR 0013); off by default so a missing check fails closed. */
    editable?: boolean
  }>(),
  { editable: false },
)

const emit = defineEmits<{
  close: []
}>()

const windowText = computed(() => props.field.todayWindowLabel ?? 'Janela de hoje disponível com o planejador')
</script>

<template>
  <MapPopover :title="field.name" :subtitle="field.subtitle" @close="emit('close')">
    <dl class="field-popover__metrics">
      <FieldMetricTile size="sm" label="Área" :value="field.areaLabel" />
      <FieldMetricTile size="sm" label="Plantio" :value="field.plantingLabel" />
      <FieldMetricTile size="sm" label="Últ. aplic." :value="field.lastApplicationLabel" />
    </dl>

    <p class="field-popover__window text-label-chip" :class="{ 'field-popover__window--muted': !field.todayWindowLabel }">
      <AppIcon name="sparkles" :size="16" class="field-popover__window-icon" />
      <span class="field-popover__window-text">{{ windowText }}</span>
    </p>

    <template #actions>
      <BaseButton v-if="editable" variant="secondary" :to="field.editFieldTo">Editar</BaseButton>
      <BaseButton
        variant="accent"
        block
        :unavailable-hint="field.planApplication.disabled ? field.planApplication.hint : undefined"
      >
        Planejar aplicação
      </BaseButton>
    </template>
  </MapPopover>
</template>

<style scoped>
.field-popover__metrics {
  display: flex;
  gap: var(--space-8);
  margin: 0;
}

.field-popover__window {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-green-900);
  color: var(--color-text-inverse);
}

.field-popover__window--muted {
  color: var(--color-text-inverse-strong);
}

.field-popover__window-icon {
  color: var(--color-sprout-400);
}

.field-popover__window-text {
  flex: 1;
  min-width: 0;
}
</style>
