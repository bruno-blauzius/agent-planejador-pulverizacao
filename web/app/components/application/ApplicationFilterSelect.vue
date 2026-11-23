<script setup lang="ts">
/**
 * Compact filter of the applications toolbar (Figma 63:237: "Período  Últimos
 * 30 dias ⌄"). A native select keeps keyboard and screen-reader behavior; the
 * option labels come ready from the logic.
 */
export interface FilterSelectOption {
  value: string | number | null
  label: string
}

const props = defineProps<{
  label: string
  options: readonly FilterSelectOption[]
  modelValue: string | number | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string | number | null]
}>()

const selectId = useId()
const selectedIndex = computed(() => Math.max(0, props.options.findIndex(option => option.value === props.modelValue)))

function onChange(event: Event) {
  const index = Number((event.target as HTMLSelectElement).value)
  const option = props.options[index]
  if (option) emit('update:modelValue', option.value)
}
</script>

<template>
  <div class="filter-select">
    <label :for="selectId" class="filter-select__label text-caption">{{ label }}</label>
    <select
      :id="selectId"
      class="filter-select__control text-label-field"
      @change="onChange"
    >
      <option
        v-for="(option, index) in options"
        :key="String(option.value)"
        :value="index"
        :selected="index === selectedIndex"
      >
        {{ option.label }}
      </option>
    </select>
    <AppIcon name="chevron-down" :size="14" class="filter-select__chevron" />
  </div>
</template>

<style scoped>
.filter-select {
  position: relative;
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  height: var(--application-filter-select-height);
  padding: 0 var(--space-12);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
}

.filter-select:focus-within {
  box-shadow: var(--focus-ring);
}

.filter-select__label {
  color: var(--color-text-tertiary);
}

.filter-select__control {
  appearance: none;
  padding: 0 var(--space-20) 0 0;
  border: 0;
  background: transparent;
  color: var(--color-text-primary);
  cursor: pointer;
}

.filter-select__control:focus {
  outline: none;
}

.filter-select__chevron {
  position: absolute;
  right: var(--space-12);
  color: var(--color-text-secondary);
  pointer-events: none;
}
</style>
