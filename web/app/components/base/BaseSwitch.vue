<script setup lang="ts">
/**
 * Switch (Figma "Interruptor" 25:136: 44×26 pill, green-800 when on, white thumb).
 * Facade over v-switch (ADR 0011): the native checkbox keeps keyboard and
 * label semantics; the track and thumb are drawn with the tokens.
 * The label is visually hidden when the surrounding card already shows it.
 */
withDefaults(
  defineProps<{
    label: string
    hideLabel?: boolean
    disabled?: boolean
    /** id of the element that describes the switch (e.g. the card text). */
    describedBy?: string
  }>(),
  { hideLabel: false, disabled: false, describedBy: undefined },
)

const model = defineModel<boolean>({ default: false })
</script>

<template>
  <v-switch
    v-model="model"
    class="base-switch"
    :class="{ 'base-switch--on': model }"
    :disabled="disabled"
    :ripple="false"
    :aria-describedby="describedBy"
    role="switch"
    :aria-checked="model"
    inset
    hide-details
    density="compact"
  >
    <template #label>
      <span :class="hideLabel ? 'visually-hidden' : 'base-switch__label text-label-strong'">{{ label }}</span>
    </template>
  </v-switch>
</template>

<style scoped>
.base-switch {
  flex: 0 0 auto;
}

.base-switch :deep(.v-selection-control) {
  min-height: 0;
}

.base-switch :deep(.v-switch__track) {
  width: var(--size-switch-width);
  min-width: var(--size-switch-width);
  height: var(--size-switch-height);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  opacity: 1;
}

.base-switch--on :deep(.v-switch__track) {
  border-color: var(--color-green-800);
  background: var(--color-green-800);
}

.base-switch :deep(.v-selection-control__wrapper) {
  width: var(--size-switch-width);
  height: var(--size-switch-height);
}

.base-switch :deep(.v-switch__thumb) {
  width: var(--size-switch-thumb);
  height: var(--size-switch-thumb);
  background: var(--color-white);
  box-shadow: var(--shadow-segment);
  color: var(--color-white);
  transform: none;
}

.base-switch :deep(.v-selection-control__input::before) {
  display: none;
}

.base-switch :deep(.v-selection-control--focus-visible .v-switch__track) {
  box-shadow: var(--focus-ring);
}

.base-switch :deep(.v-label) {
  opacity: 1;
}

.base-switch__label {
  color: var(--color-text-primary);
}
</style>
