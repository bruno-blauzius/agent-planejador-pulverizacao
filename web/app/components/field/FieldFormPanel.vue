<script setup lang="ts">
import type { FieldFormStepView } from './FieldFormSteps.vue'
import type { CropChipView, FieldFormErrors, FieldFormField, FieldFormStep, FieldFormValues } from '~/utils/fieldForm'

/**
 * "Painel · Novo talhão" (Figma 16:420): steps, approximate area of the
 * drawing, perimeter warning, details form and actions. Presentation only:
 * values, validation and submit live in useFieldForm (the page wires them).
 * POST + novalidate + submit.prevent: a submit before hydration never puts the
 * data in the URL, and validation messages come from the form logic.
 */
type TextFieldKey = Exclude<keyof FieldFormValues, 'cropCode'>

defineProps<{
  values: Readonly<FieldFormValues>
  eyebrow: string
  title: string
  steps: readonly FieldFormStepView[]
  areaPreview: { readonly label: string, readonly caption: string }
  perimeterWarning: string | null
  namePlaceholder: string
  cropChips: { readonly featured: readonly CropChipView[], readonly others: readonly CropChipView[] }
  standHint: string
  fieldErrors: FieldFormErrors
  formError: string | null
  saving: boolean
  cancelTo: string
  step: FieldFormStep
}>()

const emit = defineEmits<{
  'update-value': [key: TextFieldKey, value: string]
  'select-crop': [code: string]
  'touch': [field: FieldFormField]
  'go-to-draw': []
  'go-to-details': []
  'submit': []
}>()

const rootRef = ref<HTMLElement | null>(null)

/** After a failed submit: first invalid input, else the drawing action (no polygon yet). */
function focusFirstInvalid() {
  const root = rootRef.value
  if (!root) return
  const invalid = root.querySelector<HTMLElement>('input[aria-invalid="true"], fieldset[aria-invalid="true"] input')
  const target = invalid ?? root.querySelector<HTMLElement>('[data-area-action]')
  target?.focus()
}

defineExpose({ focusFirstInvalid })
</script>

<template>
  <section ref="rootRef" class="field-form" aria-labelledby="field-form-title">
    <header class="field-form__header">
      <div class="field-form__heading">
        <p class="field-form__eyebrow text-overline">{{ eyebrow }}</p>
        <h2 id="field-form-title" class="field-form__title text-h3">{{ title }}</h2>
      </div>
      <NuxtLink :to="cancelTo" class="field-form__close" aria-label="Cancelar e voltar" title="Cancelar">
        <AppIcon name="x" :size="18" />
      </NuxtLink>
    </header>

    <FieldFormSteps :steps="steps" @select="emit('go-to-draw')" />

    <FieldAreaCard
      :value="areaPreview.label"
      :caption="areaPreview.caption"
      :action-label="step === 'draw' ? 'Continuar' : 'Ajustar'"
      :error="fieldErrors.polygon"
      @action="step === 'draw' ? emit('go-to-details') : emit('go-to-draw')"
    />

    <ToastAlert
      v-if="perimeterWarning"
      tone="attention"
      title="Fora do perímetro?"
      :message="perimeterWarning"
      :dismissible="false"
    />

    <form class="field-form__form" method="post" novalidate @submit.prevent="emit('submit')">
      <TextField
        :model-value="values.name"
        label="Nome do talhão"
        name="name"
        :placeholder="namePlaceholder"
        :error="fieldErrors.name"
        required
        @update:model-value="emit('update-value', 'name', $event)"
        @blur="emit('touch', 'name')"
      />
      <TextField
        :model-value="values.block"
        label="Área (bloco)"
        name="block"
        placeholder="Área Sul"
        :error="fieldErrors.block"
        @update:model-value="emit('update-value', 'block', $event)"
        @blur="emit('touch', 'block')"
      />
      <FieldCropPicker
        :featured="cropChips.featured"
        :others="cropChips.others"
        :selected-code="values.cropCode"
        :error="fieldErrors.cropCode"
        @select="emit('select-crop', $event)"
      />
      <div class="field-form__row">
        <TextField
          :model-value="values.variety"
          label="Variedade"
          name="variety"
          placeholder="Catuaí 144"
          :error="fieldErrors.variety"
          @update:model-value="emit('update-value', 'variety', $event)"
          @blur="emit('touch', 'variety')"
        />
        <TextField
          :model-value="values.plantingDate"
          label="Data de plantio"
          name="plantingDate"
          type="date"
          :error="fieldErrors.plantingDate"
          @update:model-value="emit('update-value', 'plantingDate', $event)"
          @blur="emit('touch', 'plantingDate')"
        />
      </div>
      <TextField
        :model-value="values.spacing"
        label="Espaçamento (m)"
        name="spacing"
        placeholder="3,5 × 0,7"
        inputmode="decimal"
        :hint="standHint"
        :error="fieldErrors.spacing"
        @update:model-value="emit('update-value', 'spacing', $event)"
        @blur="emit('touch', 'spacing')"
      />

      <ToastAlert
        v-if="formError"
        tone="error"
        title="Não foi possível salvar"
        :message="formError"
        :dismissible="false"
      />

      <div class="field-form__actions">
        <BaseButton variant="secondary" :to="cancelTo">Cancelar</BaseButton>
        <BaseButton type="submit" block :loading="saving">Salvar talhão</BaseButton>
      </div>
    </form>
  </section>
</template>

<style scoped>
.field-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  padding: var(--space-24);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.field-form__header {
  display: flex;
  gap: var(--space-10);
  align-items: flex-start;
}

.field-form__heading {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.field-form__eyebrow {
  margin: 0;
  color: var(--color-coffee-600);
}

.field-form__title {
  margin: 0;
  color: var(--color-text-primary);
}

.field-form__close {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.field-form__close:hover {
  color: var(--color-text-primary);
}

.field-form__close:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-form__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

.field-form__row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-12);
}

.field-form__actions {
  display: flex;
  gap: var(--space-10);
}
</style>
