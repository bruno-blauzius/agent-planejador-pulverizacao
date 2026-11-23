<script setup lang="ts">
import type { SprayCondition } from '~/types/spray'

/**
 * "Pode pulverizar" card of the mobile operator view (Figma 20:35): the verdict
 * for now, until when and how much of the window has elapsed. Verdict, texts
 * and `progress` come ready from the API.
 */
const props = withDefaults(
  defineProps<{
    /** e.g. "Agora · 06:48". */
    eyebrow: string
    /** Big verdict; a "\n" breaks the line ("Pode\npulverizar"). */
    headline: string
    /** e.g. "até 10:00". */
    until?: string
    /** e.g. "· faltam 3h12" (the separator comes with the text). */
    remaining?: string
    /** Elapsed share of the window, 0 to 1. */
    progress?: number
    startLabel?: string
    endLabel?: string
    condition?: SprayCondition
  }>(),
  {
    until: undefined,
    remaining: undefined,
    progress: 0,
    startLabel: undefined,
    endLabel: undefined,
    condition: 'suitable',
  },
)

const percent = computed(() => Math.round(Math.min(1, Math.max(0, props.progress)) * 100))
</script>

<template>
  <section class="now-card" aria-label="Condição agora">
    <p class="now-card__eyebrow text-overline">
      <span class="now-card__dot" :class="`now-card__dot--${condition}`" aria-hidden="true" />
      {{ eyebrow }}
    </p>
    <h2 class="now-card__headline">{{ headline }}</h2>
    <p v-if="until || remaining" class="now-card__until">
      <span v-if="until" class="now-card__until-time text-label-lg">{{ until }}</span>
      <span v-if="remaining" class="now-card__remaining text-body-md">{{ remaining }}</span>
    </p>
    <div
      class="now-card__track"
      role="progressbar"
      aria-label="Andamento da janela"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="percent"
    >
      <span class="now-card__fill" :style="{ width: `${percent}%` }" />
    </div>
    <p v-if="startLabel || endLabel" class="now-card__ends text-caption-xs">
      <span>{{ startLabel }}</span>
      <span>{{ endLabel }}</span>
    </p>
  </section>
</template>

<style scoped>
.now-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  padding: var(--space-18) var(--space-22);
  overflow: hidden;
  border-radius: var(--radius-2xl);
  background: var(--color-green-900);
  color: var(--color-text-inverse);
}

.now-card__eyebrow {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  color: var(--color-sprout-400);
}

.now-card__dot {
  width: var(--size-planner-progress);
  height: var(--size-planner-progress);
  border-radius: var(--radius-pill);
  background: var(--color-sprout-400);
}

.now-card__dot--attention {
  background: var(--color-status-attention);
}

.now-card__dot--unsuitable {
  background: var(--color-status-unsuitable);
}

.now-card__dot--rain {
  background: var(--color-info-rain);
}

.now-card__headline {
  margin: 0;
  font: var(--font-planner-now);
  letter-spacing: var(--tracking-planner-now);
  max-width: var(--planner-now-headline-max-width);
  white-space: pre-line;
}

.now-card__until {
  display: flex;
  gap: var(--space-6);
  align-items: baseline;
  margin: 0;
}

.now-card__until-time {
  color: var(--color-sprout-400);
}

.now-card__remaining {
  color: var(--color-text-inverse-body);
}

.now-card__track {
  height: var(--size-planner-progress);
  border-radius: var(--radius-pill);
  background: var(--color-planner-track);
}

.now-card__fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--color-sprout-400);
}

.now-card__ends {
  display: flex;
  justify-content: space-between;
  margin: 0;
  color: var(--color-text-inverse-subtle);
}
</style>
