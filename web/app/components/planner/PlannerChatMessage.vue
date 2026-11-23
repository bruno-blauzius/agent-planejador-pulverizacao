<script setup lang="ts">
/**
 * Chat bubble of the spray planner (Figma 18:109 user, 18:151 agent). The text
 * comes in the default slot (paragraphs and <strong> are styled); `note` is the
 * highlighted warning the agent attaches to its answer (Figma 18:153).
 */
export type ChatRole = 'user' | 'agent'

defineProps<{
  role: ChatRole
  /** Warning shown inside the agent bubble. */
  note?: string
}>()

defineSlots<{
  default?: () => unknown
}>()
</script>

<template>
  <div class="chat-message" :class="`chat-message--${role}`">
    <span class="visually-hidden">{{ role === 'user' ? 'Você:' : 'Agente:' }}</span>
    <div class="chat-message__text text-body-md">
      <slot />
    </div>
    <p v-if="note && role === 'agent'" class="chat-message__note text-body-sm">
      <AppIcon name="drop" :size="16" class="chat-message__note-icon" />
      <span>{{ note }}</span>
    </p>
  </div>
</template>

<style scoped>
.chat-message {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  min-width: 0;
}

.chat-message--user {
  align-self: flex-end;
  max-width: min(var(--planner-bubble-user-max-width), 100%);
  padding: var(--space-12) var(--space-16);
  border-radius: var(--radius-bubble) var(--radius-bubble-tail) var(--radius-bubble) var(--radius-bubble);
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.chat-message--agent {
  align-self: stretch;
  padding: var(--space-14) var(--space-16);
  border-radius: var(--radius-bubble-tail) var(--radius-bubble) var(--radius-bubble) var(--radius-bubble);
  background: var(--color-green-100);
  color: var(--color-text-primary);
}

/* Agent answers use 22 px line height (Figma 18:152), taller than the 21 px body style. */
.chat-message--agent .chat-message__text {
  line-height: var(--planner-answer-line-height);
}

.chat-message__text :deep(p) {
  margin: 0;
}

.chat-message__text :deep(p + p) {
  margin-top: var(--space-22);
}

.chat-message__text :deep(strong) {
  font-weight: 700;
}

.chat-message__note {
  display: flex;
  gap: var(--space-8);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-10);
  border-radius: var(--radius-md);
  background: var(--color-status-attention-bg);
  color: var(--color-text-primary);
}

.chat-message__note-icon {
  color: var(--color-status-attention);
}
</style>
