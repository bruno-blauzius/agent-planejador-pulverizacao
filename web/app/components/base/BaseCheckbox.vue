<script setup lang="ts">
/**
 * Checkbox (Figma "Caixa de seleção", library block 25:169: 22 px box with a
 * primary label; the login instance 6:99 uses `size="sm"`: 20 px, muted label).
 * Facade over v-checkbox (ADR 0011): Vuetify keeps the native input (keyboard,
 * label association, aria); the box is drawn here with the Figma icon.
 */
withDefaults(
  defineProps<{
    label: string
    disabled?: boolean
    name?: string
    /** sm: login (20 px, secondary label) · md: library default (22 px, primary label). */
    size?: 'sm' | 'md'
  }>(),
  { disabled: false, name: undefined, size: 'sm' },
)

const model = defineModel<boolean>({ default: false })
</script>

<template>
  <v-checkbox
    v-model="model"
    class="base-checkbox"
    :class="`base-checkbox--${size}`"
    :name="name"
    :disabled="disabled"
    :ripple="false"
    hide-details
    density="compact"
  >
    <template #input="{ inputNode, model: checked }">
      <span
        class="base-checkbox__box"
        :class="{ 'base-checkbox__box--checked': checked.value }"
        aria-hidden="true"
      >
        <AppIcon v-if="checked.value" name="check" :size="14" />
      </span>
      <component :is="inputNode" />
    </template>
    <template #label>
      <span class="base-checkbox__label text-body-md">{{ label }}</span>
    </template>
  </v-checkbox>
</template>

<style scoped>
.base-checkbox {
  --checkbox-size: var(--size-checkbox);
  --checkbox-gap: var(--space-8);
  --checkbox-label-color: var(--color-text-secondary);

  flex: 0 0 auto;
}

.base-checkbox--md {
  --checkbox-size: var(--size-checkbox-lg);
  --checkbox-gap: var(--space-10);
  --checkbox-label-color: var(--color-text-primary);
}

.base-checkbox :deep(.v-selection-control) {
  gap: var(--checkbox-gap);
  min-height: 0;
}

.base-checkbox :deep(.v-selection-control__wrapper),
.base-checkbox :deep(.v-selection-control__input) {
  width: var(--checkbox-size);
  height: var(--checkbox-size);
}

/* Native input stays on top of the drawn box: it receives clicks and focus. */
.base-checkbox :deep(.v-selection-control__input input) {
  opacity: 0;
  cursor: pointer;
}

.base-checkbox :deep(.v-selection-control__input::before) {
  display: none;
}

.base-checkbox__box {
  display: grid;
  place-items: center;
  width: var(--checkbox-size);
  height: var(--checkbox-size);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-xs);
  background: var(--color-white);
  color: var(--color-text-inverse);
  transition:
    background-color var(--duration-fast) var(--easing-standard),
    border-color var(--duration-fast) var(--easing-standard);
}

.base-checkbox__box--checked {
  border-color: var(--color-green-800);
  background: var(--color-green-800);
}

.base-checkbox :deep(.v-selection-control--focus-visible) .base-checkbox__box {
  box-shadow: var(--focus-ring);
}

.base-checkbox :deep(.v-label) {
  opacity: 1;
  letter-spacing: 0;
  cursor: pointer;
}

.base-checkbox__label {
  color: var(--checkbox-label-color);
}

.base-checkbox :deep(.v-selection-control--disabled) .base-checkbox__box {
  background: var(--color-sand);
}

.base-checkbox :deep(.v-selection-control--disabled) .base-checkbox__label {
  color: var(--color-text-tertiary);
}
</style>
