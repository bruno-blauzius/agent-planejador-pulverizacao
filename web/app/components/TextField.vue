<script setup lang="ts">
import type { IconName } from './app/AppIcon.vue'

/**
 * Text field (Figma "Campo de texto", 24:137). Label always above the box
 * (never placeholder-only), 48 px tall for gloved/sunlit use.
 * States: default, focus (sprout ring), filled, error, disabled.
 */
const props = withDefaults(
  defineProps<{
    label: string
    placeholder?: string
    hint?: string
    /** Error message. When set, the field is marked invalid and the hint is replaced. */
    error?: string
    icon?: IconName
    type?: 'text' | 'email' | 'password' | 'tel' | 'search' | 'url' | 'number' | 'date'
    inputmode?: 'text' | 'email' | 'tel' | 'numeric' | 'decimal' | 'search' | 'url'
    autocomplete?: string
    name?: string
    id?: string
    disabled?: boolean
    required?: boolean
    readonly?: boolean
    maxlength?: number
  }>(),
  {
    placeholder: undefined,
    hint: undefined,
    error: undefined,
    icon: undefined,
    type: 'text',
    inputmode: undefined,
    autocomplete: undefined,
    name: undefined,
    id: undefined,
    disabled: false,
    required: false,
    readonly: false,
    maxlength: undefined,
  },
)

const model = defineModel<string>({ default: '' })

const emit = defineEmits<{
  blur: [event: FocusEvent]
  focus: [event: FocusEvent]
}>()

defineSlots<{
  /** Trailing content inside the box (e.g. show-password button). */
  trailing?: () => unknown
}>()

const generatedId = useId()
const inputId = computed(() => props.id ?? generatedId)
const hasError = computed(() => Boolean(props.error))
const errorMessages = computed(() => (props.error ? [props.error] : []))
</script>

<template>
  <div
    class="text-field"
    :class="{ 'text-field--error': hasError, 'text-field--disabled': disabled }"
  >
    <!-- Vuetify points the input's aria-labelledby to `${id}-label`: this is that label. -->
    <label :id="`${inputId}-label`" :for="inputId" class="text-field__label text-label-field">
      {{ label }}
      <span v-if="required" class="text-field__required" aria-hidden="true">*</span>
    </label>
    <!--
      Facade over v-text-field (ADR 0011). The label stays above the box (Figma:
      never a floating label); hint and error use Vuetify messages, which wire
      aria-describedby and announce errors.
    -->
    <v-text-field
      :id="inputId"
      v-model="model"
      class="text-field__control"
      :type="type"
      :name="name"
      :placeholder="placeholder"
      :inputmode="inputmode"
      :autocomplete="autocomplete"
      :disabled="disabled"
      :readonly="readonly"
      :required="required"
      :maxlength="maxlength"
      :hint="hasError ? undefined : hint"
      :persistent-hint="Boolean(hint)"
      :error-messages="errorMessages"
      :aria-invalid="hasError || undefined"
      variant="outlined"
      hide-details="auto"
      @blur="emit('blur', $event)"
      @focus="emit('focus', $event)"
    >
      <template v-if="icon" #prepend-inner>
        <AppIcon :name="icon" :size="18" class="text-field__icon" />
      </template>
      <template v-if="$slots.trailing" #append-inner>
        <slot name="trailing" />
      </template>
    </v-text-field>
  </div>
</template>

<style scoped>
.text-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.text-field__label {
  color: var(--color-text-primary);
}

.text-field__required {
  color: var(--color-status-unsuitable);
}

/* The Figma box is drawn on .v-field; Vuetify's notched outline and state layer are not used. */
.text-field__control :deep(.v-field) {
  --border-extra: calc(var(--border-width-strong) - var(--border-width-control));

  /*
   * No grid gap: v-field keeps four columns even when the icon and trailing
   * slots are absent, and each empty column would still eat a gap (narrow
   * fields such as CEP clipped the text). The 10 px spacing goes on the slots.
   */
  gap: 0;
  align-items: center;
  height: var(--size-control-md);
  padding: 0 var(--space-16);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-primary);
  transition:
    border-color var(--duration-fast) var(--easing-standard),
    box-shadow var(--duration-fast) var(--easing-standard);
}

.text-field__control :deep(.v-field__outline),
.text-field__control :deep(.v-field__overlay),
.text-field__control :deep(.v-field__loader) {
  display: none;
}

/* Figma focus/error use the strong border; the extra width is an inset shadow to avoid layout shift. */
.text-field__control :deep(.v-field--focused) {
  border-color: var(--color-green-700);
  box-shadow:
    inset 0 0 0 var(--border-extra) var(--color-green-700),
    var(--focus-ring);
}

.text-field__control :deep(.v-field__field),
.text-field__control :deep(.v-field__prepend-inner),
.text-field__control :deep(.v-field__append-inner) {
  align-items: center;
  height: 100%;
  padding: 0;
}

.text-field__control :deep(.v-field__prepend-inner) {
  padding-inline-end: var(--space-10);
}

.text-field__control :deep(.v-field__append-inner) {
  padding-inline-start: var(--space-10);
}

.text-field__control :deep(.v-field__input) {
  min-height: 0;
  height: 100%;
  padding: 0;
  font: var(--font-body-lg);
  letter-spacing: 0;
  color: var(--color-text-primary);
  opacity: 1;
  /* The focus indicator is the field box (border + ring), not the input outline. */
  outline: none;
}

.text-field__control :deep(.v-field__input::placeholder) {
  color: var(--color-text-tertiary);
  opacity: 1;
}

.text-field__icon {
  color: var(--color-text-secondary);
}

.text-field__control :deep(.v-input__details) {
  min-height: 0;
  padding: var(--space-8) 0 0;
}

/* Without hint or error the details row would still add its 8 px padding (Figma field = label + 8 + 48). */
.text-field__control :deep(.v-input__details--hidden) {
  display: none;
}

.text-field__control :deep(.v-messages),
.text-field__control :deep(.v-messages__message) {
  font: var(--font-body-sm);
  letter-spacing: 0;
  color: var(--color-text-tertiary);
  opacity: 1;
}

.text-field--error .text-field__control :deep(.v-field) {
  border-color: var(--color-status-unsuitable);
  box-shadow: inset 0 0 0 var(--border-extra) var(--color-status-unsuitable);
}

.text-field--error .text-field__control :deep(.v-messages),
.text-field--error .text-field__control :deep(.v-messages__message) {
  color: var(--color-status-unsuitable);
}

.text-field--disabled .text-field__label,
.text-field--disabled .text-field__icon,
.text-field--disabled .text-field__control :deep(.v-field__input) {
  color: var(--color-text-tertiary);
}

.text-field--disabled .text-field__control :deep(.v-field) {
  background: var(--color-sand);
  cursor: not-allowed;
}

.text-field--disabled .text-field__control :deep(.v-field--disabled) {
  opacity: 1;
}
</style>
