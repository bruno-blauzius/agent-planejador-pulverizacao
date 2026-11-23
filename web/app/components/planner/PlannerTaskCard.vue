<script setup lang="ts">
/**
 * "Tarefa de hoje" card of the mobile operator view (Figma 20:123). Texts come
 * ready from the API. The action only announces the click: the application
 * was already confirmed by a human on the planner, so this only starts it.
 */
withDefaults(
  defineProps<{
    /** Field code in the tile, e.g. "T3". */
    fieldCode: string
    /** e.g. "Tarefa de hoje". */
    eyebrow?: string
    /** e.g. "Fungicida · 38,6 ha". */
    title: string
    /** e.g. "0,5 L/ha · 4 tanques de 2.000 L". */
    detail: string
    actionLabel?: string
    actionDisabled?: boolean
    /** Shows the spinner and blocks the action while it runs. */
    actionLoading?: boolean
    /** Why the action is unavailable (kept focusable, reason announced). */
    actionHint?: string
  }>(),
  { eyebrow: 'Tarefa de hoje', actionLabel: 'Iniciar aplicação', actionDisabled: false, actionLoading: false, actionHint: undefined },
)

const emit = defineEmits<{
  action: []
}>()
</script>

<template>
  <section class="task-card" :aria-label="eyebrow">
    <div class="task-card__summary">
      <span class="task-card__code text-h4" aria-hidden="true">{{ fieldCode }}</span>
      <div class="task-card__text">
        <p class="task-card__eyebrow text-label-micro">{{ eyebrow }}</p>
        <h2 class="task-card__title text-label-lg">
          <span class="visually-hidden">Talhão {{ fieldCode }}: </span>{{ title }}
        </h2>
        <p class="task-card__detail text-body-sm">{{ detail }}</p>
      </div>
    </div>
    <BaseButton
      size="lg"
      icon="play"
      block
      :disabled="actionDisabled"
      :loading="actionLoading"
      :unavailable-hint="actionHint"
      @click="emit('action')"
    >
      {{ actionLabel }}
    </BaseButton>
  </section>
</template>

<style scoped>
.task-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding: var(--space-18);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-xl);
  background: var(--color-white);
}

.task-card__summary {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.task-card__code {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-icon-tile-lg);
  height: var(--size-icon-tile-lg);
  border-radius: var(--radius-tile-lg);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.task-card__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.task-card__eyebrow {
  margin: 0;
  color: var(--color-coffee-600);
  letter-spacing: var(--tracking-overline);
  text-transform: uppercase;
}

.task-card__title {
  margin: 0;
  color: var(--color-text-primary);
}

.task-card__detail {
  margin: 0;
  color: var(--color-text-secondary);
}
</style>
