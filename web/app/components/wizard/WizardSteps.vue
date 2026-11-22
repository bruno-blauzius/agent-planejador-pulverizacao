<script setup lang="ts" generic="K extends string">
/**
 * Wizard steps (Figma "Etapas" 35:450 / 14:125): numbered vertical list with
 * connectors and the draft status box (cloud icon). Completed steps show a dark
 * check; reachable steps (other than the current one) are buttons.
 * Structurally compatible with `WizardStepView` from the wizard contract.
 */
export interface WizardStepItem<V extends string = string> {
  key: V
  number: number
  title: string
  description: string
  status: 'complete' | 'current' | 'pending'
  isReachable: boolean
}

const props = withDefaults(
  defineProps<{
    steps: readonly WizardStepItem<K>[]
    /** Draft status text, e.g. "Rascunho salvo · há 2 min". Hidden when empty. */
    draftStatus?: string | null
    /**
     * Timestamp of the last draft save. Each new value is announced once; without
     * it only the first appearance of the status is announced.
     */
    draftSavedAt?: string | number | null
    /** Text announced to screen readers on a new save. */
    draftAnnouncement?: string
  }>(),
  { draftStatus: null, draftSavedAt: null, draftAnnouncement: 'Rascunho salvo' },
)

const emit = defineEmits<{
  select: [key: K]
}>()

// Debounced: the draft is saved on every change and typing must not flood the reader.
const ANNOUNCE_DELAY_MS = 1500
const announcement = ref('')
let announceTimer: ReturnType<typeof setTimeout> | undefined

// The relative label ("há 2 min") changes every minute, so it stays out of the live region.
const saveSignal = computed(() => props.draftSavedAt ?? (props.draftStatus ? 'saved' : null))

watch(saveSignal, (signal, previous) => {
  if (!signal || signal === previous) return
  clearTimeout(announceTimer)
  announcement.value = ''
  announceTimer = setTimeout(() => {
    announcement.value = props.draftAnnouncement
  }, ANNOUNCE_DELAY_MS)
})

onBeforeUnmount(() => clearTimeout(announceTimer))

function isLink(step: WizardStepItem<K>): boolean {
  return step.isReachable && step.status !== 'current'
}
</script>

<template>
  <aside class="wizard-steps">
    <nav aria-label="Etapas do cadastro">
      <ol class="wizard-steps__list">
        <li
          v-for="(step, index) in steps"
          :key="step.key"
          class="wizard-steps__item"
          :class="`wizard-steps__item--${step.status}`"
          :aria-current="step.status === 'current' ? 'step' : undefined"
        >
          <span class="wizard-steps__track" aria-hidden="true">
            <span class="wizard-steps__marker text-label-strong">
              <AppIcon v-if="step.status === 'complete'" name="check" :size="16" />
              <template v-else>{{ step.number }}</template>
            </span>
            <span v-if="index < steps.length - 1" class="wizard-steps__connector" />
          </span>

          <button
            v-if="isLink(step)"
            type="button"
            class="wizard-steps__text wizard-steps__text--link"
            @click="emit('select', step.key)"
          >
            <span class="wizard-steps__title">{{ step.title }}</span>
            <span class="wizard-steps__description text-body-sm">{{ step.description }}</span>
            <span v-if="step.status === 'complete'" class="visually-hidden">(concluída)</span>
          </button>
          <span v-else class="wizard-steps__text">
            <span class="wizard-steps__title">{{ step.title }}</span>
            <span class="wizard-steps__description text-body-sm">{{ step.description }}</span>
          </span>
        </li>
      </ol>
    </nav>

    <p class="visually-hidden" role="status">{{ announcement }}</p>
    <p v-if="draftStatus" class="wizard-steps__draft text-label-caption">
      <AppIcon name="cloud" :size="16" />
      {{ draftStatus }}
    </p>
  </aside>
</template>

<style scoped>
.wizard-steps {
  display: flex;
  flex-direction: column;
  padding: var(--space-20);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.wizard-steps__list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.wizard-steps__item {
  display: flex;
  gap: var(--space-14);
  align-items: flex-start;
  padding: var(--space-10) 0;
}

.wizard-steps__track {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  align-items: center;
}

.wizard-steps__marker {
  display: grid;
  place-items: center;
  width: var(--size-step-marker);
  height: var(--size-step-marker);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  color: var(--color-text-tertiary);
}

.wizard-steps__connector {
  width: var(--border-width-strong);
  height: var(--wizard-step-connector-height);
  background: var(--color-border);
}

.wizard-steps__item--current .wizard-steps__marker {
  border-color: var(--color-sprout-400);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.wizard-steps__item--complete .wizard-steps__marker {
  border-color: var(--color-green-800);
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.wizard-steps__item--complete .wizard-steps__connector {
  background: var(--color-green-800);
}

.wizard-steps__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding-top: var(--space-6);
  color: var(--color-text-tertiary);
  text-align: left;
}

.wizard-steps__text--link {
  padding-right: 0;
  padding-bottom: 0;
  padding-left: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: none;
  cursor: pointer;
}

.wizard-steps__text--link:hover .wizard-steps__title {
  text-decoration: underline;
}

.wizard-steps__text--link:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.wizard-steps__title {
  font: var(--font-label-field);
}

.wizard-steps__item--current .wizard-steps__title,
.wizard-steps__item--complete .wizard-steps__title {
  font: var(--font-label-strong);
  color: var(--color-text-primary);
}

.wizard-steps__draft {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

/* Narrow screens: horizontal row of markers, titles only. */
@media (max-width: 959px) {
  .wizard-steps__list {
    display: flex;
    gap: var(--space-16);
    overflow-x: auto;
    scrollbar-width: thin;
  }

  .wizard-steps__item {
    flex-shrink: 0;
    align-items: center;
  }

  .wizard-steps__connector,
  .wizard-steps__description {
    display: none;
  }

  .wizard-steps__text {
    padding-top: 0;
  }

  .wizard-steps__draft {
    margin-top: var(--space-8);
  }
}
</style>
