<script setup lang="ts" generic="T extends string">
/**
 * Select ("Campo de seleção", library 24:189; instances 52:828, 54:623).
 * Facade over v-select (ADR 0011). Same box as TextField: label above, 48 px,
 * 1.5 px border, Figma chevron; the menu is a floating card with the tokens.
 * For long lists, a searchable autocomplete is the better fit.
 */
export interface SelectItem<V extends string = string> {
  value: V
  label: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    label: string
    items: readonly SelectItem<T>[]
    placeholder?: string
    hint?: string
    error?: string
    disabled?: boolean
    required?: boolean
    name?: string
    id?: string
  }>(),
  {
    placeholder: 'Selecione',
    hint: undefined,
    error: undefined,
    disabled: false,
    required: false,
    name: undefined,
    id: undefined,
  },
)

const model = defineModel<T | null>({ default: null })

const emit = defineEmits<{
  blur: [event: FocusEvent]
}>()

const generatedId = useId()
const inputId = computed(() => props.id ?? generatedId)
const hasError = computed(() => Boolean(props.error))
const errorMessages = computed(() => (props.error ? [props.error] : []))
const selectItems = computed(() =>
  props.items.map((item) => ({ value: item.value, title: item.label, props: { disabled: item.disabled } })),
)
</script>

<template>
  <div class="base-select" :class="{ 'base-select--error': hasError, 'base-select--disabled': disabled }">
    <!-- Labels the input (`for`) and the options list: v-select points the menu list's aria-labelledby to `${id}-label`. -->
    <label :id="`${inputId}-label`" :for="inputId" class="base-select__label text-label-field">
      {{ label }}
      <span v-if="required" class="base-select__required" aria-hidden="true">*</span>
    </label>
    <v-select
      :id="inputId"
      v-model="model"
      class="base-select__control"
      :items="selectItems"
      item-title="title"
      item-value="value"
      :placeholder="placeholder"
      :name="name"
      :disabled="disabled"
      :hint="hasError ? undefined : hint"
      :persistent-hint="Boolean(hint)"
      :error-messages="errorMessages"
      :aria-invalid="hasError || undefined"
      :menu-props="{ contentClass: 'base-select-menu' }"
      variant="outlined"
      hide-details="auto"
      @blur="emit('blur', $event)"
    />
  </div>
</template>

<style scoped>
.base-select {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.base-select__label {
  color: var(--color-text-primary);
}

.base-select__required {
  color: var(--color-status-unsuitable);
}

/* Same box as TextField (Figma 24:170). */
.base-select__control :deep(.v-field) {
  --border-extra: calc(var(--border-width-strong) - var(--border-width-control));

  height: var(--size-control-md);
  padding: 0 var(--space-16);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-primary);
  cursor: pointer;
  transition:
    border-color var(--duration-fast) var(--easing-standard),
    box-shadow var(--duration-fast) var(--easing-standard);
}

.base-select__control :deep(.v-field__outline),
.base-select__control :deep(.v-field__overlay),
.base-select__control :deep(.v-field__loader) {
  display: none;
}

.base-select__control :deep(.v-field--focused) {
  border-color: var(--color-green-700);
  box-shadow:
    inset 0 0 0 var(--border-extra) var(--color-green-700),
    var(--focus-ring);
}

.base-select__control :deep(.v-field__field),
.base-select__control :deep(.v-field__append-inner) {
  align-items: center;
  height: 100%;
  padding: 0;
}

.base-select__control :deep(.v-field__input) {
  min-height: 0;
  height: 100%;
  padding: 0;
  font: var(--font-body-lg);
  letter-spacing: 0;
  color: var(--color-text-primary);
  opacity: 1;
  outline: none;
}

/* v-select keeps an absolutely positioned input for the placeholder: center it in the box. */
.base-select__control :deep(.v-field__input input) {
  top: 50%;
  transform: translateY(-50%);
}

.base-select__control :deep(.v-field__input input::placeholder),
.base-select__control :deep(.v-select__selection-text) {
  font: var(--font-body-lg);
}

.base-select__control :deep(.v-field__input input::placeholder) {
  color: var(--color-text-tertiary);
  opacity: 1;
}

.base-select__control :deep(.v-select__menu-icon) {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--size-icon-select);
  opacity: 1;
}

.base-select__control :deep(.v-input__details) {
  min-height: 0;
  padding: var(--space-8) 0 0;
}

.base-select__control :deep(.v-messages),
.base-select__control :deep(.v-messages__message) {
  font: var(--font-body-sm);
  letter-spacing: 0;
  color: var(--color-text-tertiary);
  opacity: 1;
}

.base-select--error .base-select__control :deep(.v-field) {
  border-color: var(--color-status-unsuitable);
  box-shadow: inset 0 0 0 var(--border-extra) var(--color-status-unsuitable);
}

.base-select--error .base-select__control :deep(.v-messages),
.base-select--error .base-select__control :deep(.v-messages__message) {
  color: var(--color-status-unsuitable);
}

.base-select--disabled .base-select__label,
.base-select--disabled .base-select__control :deep(.v-field__input) {
  color: var(--color-text-tertiary);
}

.base-select--disabled .base-select__control :deep(.v-field) {
  background: var(--color-sand);
  cursor: not-allowed;
}

.base-select--disabled .base-select__control :deep(.v-field--disabled) {
  opacity: 1;
}
</style>

<style>
/* The menu is teleported by Vuetify, outside the scoped styles: floating card with the tokens. */
.base-select-menu > .v-list {
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
}

.base-select-menu .v-list-item {
  min-height: var(--size-control-sm);
  border-radius: var(--radius-sm);
  color: var(--color-text-primary);
}

.base-select-menu .v-list-item-title {
  font: var(--font-body-md);
  letter-spacing: 0;
}

.base-select-menu .v-list-item--active {
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.base-select-menu .v-list-item--active > .v-list-item__overlay {
  display: none;
}
</style>
