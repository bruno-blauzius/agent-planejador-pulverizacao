<script setup lang="ts">
/**
 * "Participação total" bar of wizard step 3 (Figma "Barra de progresso" 26:174).
 * Green when the total closes 100%; the error below it blocks "Continuar".
 */
const props = withDefaults(
  defineProps<{
    label: string
    /** Bar width, 0–100. */
    progress: number
    isComplete: boolean
    error?: string | null
    errorId?: string
  }>(),
  { error: null, errorId: undefined },
)

const fillWidth = computed(() => `${props.progress}%`)
</script>

<template>
  <div class="owners-total" :class="{ 'owners-total--complete': isComplete }">
    <p class="owners-total__legend">
      <span class="owners-total__label text-body-sm">Participação total</span>
      <span class="owners-total__value text-label-badge">{{ label }}</span>
    </p>
    <div class="owners-total__track" aria-hidden="true">
      <div class="owners-total__fill" />
    </div>
    <!-- Focus target of a blocked "Continuar" when only the total is wrong. -->
    <p
      v-if="error"
      :id="errorId"
      class="owners-total__error text-body-sm"
      tabindex="-1"
      data-focus-on-error
    >
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.owners-total {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.owners-total__legend {
  display: flex;
  gap: var(--space-8);
  align-items: baseline;
  margin: 0;
}

.owners-total__label {
  flex: 1;
  color: var(--color-text-tertiary);
}

.owners-total__value {
  color: var(--color-status-attention);
}

.owners-total--complete .owners-total__value {
  color: var(--color-status-suitable);
}

.owners-total__track {
  overflow: hidden;
  height: var(--space-6);
  border-radius: var(--radius-pill);
  background: var(--color-border);
}

.owners-total__fill {
  width: v-bind(fillWidth);
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--color-status-attention);
  transition: width var(--duration-fast) var(--easing-standard);
}

.owners-total--complete .owners-total__fill {
  background: var(--color-status-suitable);
}

.owners-total__error {
  margin: 0;
  outline: none;
  color: var(--color-status-unsuitable);
}

.owners-total__error:focus-visible {
  border-radius: var(--radius-xs);
  box-shadow: var(--focus-ring);
}

@media (prefers-reduced-motion: reduce) {
  .owners-total__fill {
    transition: none;
  }
}
</style>
