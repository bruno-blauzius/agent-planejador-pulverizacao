<script setup lang="ts">
/**
 * Message list of the planner chat. It is a live region (`role="log"`), so
 * screen readers announce each new message. While the agent is answering it
 * shows a status line, and a failed answer is announced as an alert; the
 * parent decides when each applies.
 */
withDefaults(
  defineProps<{
    /** The agent is still answering. */
    pending?: boolean
    pendingLabel?: string
    /** Message of a failed answer (never an invented reply). */
    error?: string
  }>(),
  { pending: false, pendingLabel: 'O agente está respondendo…', error: undefined },
)

defineSlots<{
  default?: () => unknown
}>()
</script>

<template>
  <div class="chat-log">
    <div class="chat-log__messages" role="log" aria-live="polite" aria-relevant="additions" aria-label="Conversa com o agente">
      <slot />
    </div>
    <p v-if="pending" class="chat-log__pending text-body-sm" role="status">
      <span class="chat-log__spinner" aria-hidden="true" />
      {{ pendingLabel }}
    </p>
    <p v-if="error" class="chat-log__error text-body-sm" role="alert">
      <AppIcon name="alert" :size="16" />
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.chat-log {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.chat-log__messages {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  min-width: 0;
}

.chat-log__pending {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  color: var(--color-text-tertiary);
}

.chat-log__spinner {
  width: var(--size-spinner);
  height: var(--size-spinner);
  border: var(--border-width-strong) solid currentColor;
  border-right-color: transparent;
  border-radius: var(--radius-pill);
  animation: chat-log-spin var(--duration-spinner) linear infinite;
}

.chat-log__error {
  display: flex;
  gap: var(--space-8);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-10);
  border-radius: var(--radius-md);
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}

@keyframes chat-log-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .chat-log__spinner {
    animation-duration: 2s;
  }
}
</style>
