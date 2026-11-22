<script setup lang="ts">
/**
 * Wizard actions (Figma "ações" 35:679 / 14:271). Lives inside the step form:
 * the main button is the form's submit ("Continuar", or "Salvar fazenda" on the
 * last step); the first step shows "Cancelar" in place of "Voltar".
 */
withDefaults(
  defineProps<{
    isFirstStep?: boolean
    isLastStep?: boolean
    /** Busy state of the main action (saving the farm). */
    saving?: boolean
    disabled?: boolean
  }>(),
  { isFirstStep: false, isLastStep: false, saving: false, disabled: false },
)

const emit = defineEmits<{
  back: []
  cancel: []
  saveAndExit: []
}>()
</script>

<template>
  <div class="wizard-footer">
    <BaseButton v-if="isFirstStep" variant="ghost" :disabled="saving" @click="emit('cancel')">
      Cancelar
    </BaseButton>
    <BaseButton v-else variant="ghost" :disabled="saving" @click="emit('back')">
      Voltar
    </BaseButton>

    <span class="wizard-footer__spacer" aria-hidden="true" />

    <BaseButton variant="secondary" :disabled="saving || disabled" @click="emit('saveAndExit')">
      Salvar e sair
    </BaseButton>
    <!-- Last step: accent button with plus icon (Figma 35:1396). -->
    <BaseButton
      type="submit"
      :variant="isLastStep ? 'accent' : 'primary'"
      :icon="isLastStep ? 'plus' : undefined"
      :loading="saving"
      :disabled="disabled"
    >
      {{ isLastStep ? 'Salvar fazenda' : 'Continuar' }}
    </BaseButton>
  </div>
</template>

<style scoped>
.wizard-footer {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
  /* Figma centers the buttons in a 100 px row (spacer frame 35:680). */
  padding-block: var(--space-24);
}

.wizard-footer__spacer {
  flex: 1;
}

@media (max-width: 599px) {
  .wizard-footer {
    flex-direction: column-reverse;
    align-items: stretch;
  }

  .wizard-footer__spacer {
    display: none;
  }
}
</style>
