<script setup lang="ts">
/**
 * Message box "Pergunte ao agente…" (Figma 18:164). Emits `submit` with the
 * trimmed text; the parent decides what to do with it and clears the model.
 */
const model = defineModel<string>({ default: '' })

const props = withDefaults(
  defineProps<{
    placeholder?: string
    /** Accessible name of the field (the Figma frame has only the placeholder). */
    label?: string
    disabled?: boolean
  }>(),
  { placeholder: 'Pergunte ao agente…', label: 'Mensagem para o agente', disabled: false },
)

const emit = defineEmits<{
  submit: [text: string]
}>()

const inputId = useId()
const canSend = computed(() => !props.disabled && model.value.trim().length > 0)

function onSubmit() {
  if (!canSend.value) return
  emit('submit', model.value.trim())
}
</script>

<template>
  <form class="chat-input" method="post" novalidate @submit.prevent="onSubmit">
    <label :for="inputId" class="visually-hidden">{{ label }}</label>
    <input
      :id="inputId"
      v-model="model"
      class="chat-input__field text-body-md"
      type="text"
      autocomplete="off"
      :placeholder="placeholder"
      :disabled="disabled"
    >
    <button
      class="chat-input__send"
      type="submit"
      aria-label="Enviar mensagem"
      :disabled="disabled"
    >
      <AppIcon name="send" :size="20" />
    </button>
  </form>
</template>

<style scoped>
.chat-input {
  display: flex;
  gap: var(--space-10);
  align-items: center;
}

.chat-input__field {
  flex: 1 1 auto;
  min-width: 0;
  height: var(--size-control-md);
  padding: 0 var(--space-14);
  border: var(--border-width-control) solid transparent;
  border-radius: var(--radius-tile-lg);
  background: var(--color-sand);
  color: var(--color-text-primary);
}

.chat-input__field::placeholder {
  color: var(--color-text-tertiary);
}

.chat-input__field:focus-visible {
  outline: none;
  border-color: var(--color-sprout-400);
  box-shadow: var(--focus-ring);
}

.chat-input__field:disabled {
  cursor: not-allowed;
}

.chat-input__send {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-control-md);
  height: var(--size-control-md);
  border: 0;
  border-radius: var(--radius-tile-lg);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--easing-standard);
}

.chat-input__send:hover:not(:disabled) {
  background: var(--color-sprout-500);
}

.chat-input__send:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.chat-input__send:disabled {
  background: var(--color-border);
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}
</style>
