<script setup lang="ts">
/**
 * "Plano sugerido pelo agente" (Figma 18:172): dark panel that groups the
 * field plan cards, an optional warning and the two actions. The primary
 * action (register) only asks for confirmation; nothing is saved here.
 */
const props = withDefaults(
  defineProps<{
    eyebrow?: string
    title: string
    /** Warning below the cards, e.g. a window shorter than the work. */
    note?: string
    primaryLabel?: string
    secondaryLabel?: string
    /** Disables both actions (e.g. while the agent is still answering). */
    actionsDisabled?: boolean
    /** Why an action cannot be used yet (e.g. "Em breve"); it stays focusable with the reason in its name. */
    primaryUnavailableHint?: string
    secondaryUnavailableHint?: string
  }>(),
  {
    eyebrow: 'Plano sugerido pelo agente',
    note: undefined,
    primaryLabel: 'Revisar e registrar plano',
    secondaryLabel: 'Ajustar horários',
    actionsDisabled: false,
    primaryUnavailableHint: undefined,
    secondaryUnavailableHint: undefined,
  },
)

const emit = defineEmits<{
  primary: []
  secondary: []
}>()

function onSecondary() {
  if (props.secondaryUnavailableHint) return
  emit('secondary')
}

defineSlots<{
  /** The field plan cards. */
  default?: () => unknown
}>()

const titleId = useId()
</script>

<template>
  <section class="plan-panel" :aria-labelledby="titleId">
    <header class="plan-panel__header">
      <p class="plan-panel__eyebrow text-overline">{{ eyebrow }}</p>
      <h2 :id="titleId" class="plan-panel__title">{{ title }}</h2>
    </header>

    <div class="plan-panel__cards">
      <slot />
    </div>

    <p v-if="note" class="plan-panel__note text-body-sm">
      <AppIcon name="alert" :size="14" class="plan-panel__note-icon" />
      {{ note }}
    </p>

    <div class="plan-panel__actions">
      <BaseButton
        variant="accent"
        :disabled="actionsDisabled"
        :unavailable-hint="primaryUnavailableHint"
        @click="emit('primary')"
      >
        {{ primaryLabel }}
      </BaseButton>
      <button
        class="plan-panel__secondary text-label-strong"
        type="button"
        :disabled="actionsDisabled"
        :aria-disabled="secondaryUnavailableHint ? true : undefined"
        :title="secondaryUnavailableHint"
        @click="onSecondary"
      >
        {{ secondaryLabel }}
        <span v-if="secondaryUnavailableHint" class="visually-hidden">({{ secondaryUnavailableHint }})</span>
      </button>
    </div>
  </section>
</template>

<style scoped>
.plan-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  min-width: 0;
  padding: var(--space-24);
  border-radius: var(--radius-card-xl);
  background: var(--color-green-900);
  color: var(--color-text-inverse);
}

.plan-panel__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.plan-panel__eyebrow {
  margin: 0;
  color: var(--color-sprout-400);
}

.plan-panel__title {
  margin: 0;
  font: var(--font-planner-plan-title);
  letter-spacing: var(--tracking-planner-plan-title);
}

.plan-panel__cards {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
}

.plan-panel__note {
  display: flex;
  gap: var(--space-6);
  align-items: flex-start;
  margin: 0;
  color: var(--color-text-inverse-body);
}

.plan-panel__note-icon {
  flex-shrink: 0;
  margin-top: var(--space-2);
  color: var(--color-status-attention);
}

.plan-panel__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
}

.plan-panel__secondary {
  height: var(--size-control-md);
  padding: 0 var(--space-18);
  border: var(--border-width-hairline) solid var(--color-planner-outline-inverse);
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-text-inverse);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--easing-standard);
}

.plan-panel__secondary:hover:not(:disabled):not([aria-disabled='true']) {
  background: var(--color-surface-inverse-hover);
}

.plan-panel__secondary:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.plan-panel__secondary[aria-disabled='true'],
.plan-panel__secondary:disabled {
  color: var(--color-text-inverse-faint);
  cursor: not-allowed;
}
</style>
