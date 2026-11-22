<script setup lang="ts">
import type { ChoiceChipOption } from '../wizard/ChoiceChips.vue'
import type { CropChipView } from '~/utils/fieldForm'

/**
 * Crop of the field (Figma 16:466–16:479): the featured crops as selection
 * chips plus "+ Outra", which lists the other crops of the catalog. A chosen
 * crop outside the featured ones already comes as a selected chip in
 * `featured` (useFieldForm), so the chips always show the current value.
 */
const props = defineProps<{
  featured: readonly CropChipView[]
  others: readonly CropChipView[]
  selectedCode: string | null
  error?: string
}>()

const emit = defineEmits<{
  select: [code: string]
}>()

const options = computed<readonly ChoiceChipOption[]>(() =>
  props.featured.map((chip) => ({ value: chip.code, label: chip.label, emoji: chip.icon })),
)

function onUpdate(code: string | null) {
  if (code) emit('select', code)
}
</script>

<template>
  <div class="field-crop">
    <WizardChoiceChips
      :model-value="selectedCode"
      :options="options"
      label="Cultura"
      :error="error"
      required
      @update:model-value="onUpdate"
    />
    <v-menu v-if="others.length" location="bottom start">
      <template #activator="{ props: menuProps }">
        <button type="button" class="field-crop__other text-label-field" v-bind="menuProps">+ Outra</button>
      </template>
      <ul class="field-crop__menu" aria-label="Outras culturas">
        <li v-for="crop in others" :key="crop.code">
          <button type="button" class="field-crop__option text-label-field" @click="emit('select', crop.code)">
            <span aria-hidden="true">{{ crop.icon }}</span>
            {{ crop.label }}
          </button>
        </li>
      </ul>
    </v-menu>
  </div>
</template>

<style scoped>
.field-crop {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  align-items: flex-start;
}

.field-crop__other {
  padding: var(--space-10) var(--space-14);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.field-crop__other:hover {
  color: var(--color-text-primary);
}

.field-crop__other:focus-visible,
.field-crop__option:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-crop__menu {
  min-width: var(--menu-min-width);
  margin: 0;
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
  list-style: none;
}

.field-crop__option {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  width: 100%;
  padding: var(--space-8) var(--space-10);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.field-crop__option:hover {
  background: var(--color-sand);
}
</style>
