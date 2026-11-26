<script setup lang="ts">
import type { SoilInspectionForm } from '~/composables/useSoilInspectionForm'
import type { InspectableSoilState } from '~/types/trafficability'
import { normalizeTimeInput } from '~/utils/fieldConditionsForm'

/**
 * "Registrar vistoria" (ADR 0022, phase 2; no Figma frame: follows the relief
 * adjustment modal 71:1395). Soil state seen in the field, date and time and an
 * optional note, with the 12 h validity notice. State, validation and saving
 * come from `form` (useSoilInspectionForm); mount it with `v-if`.
 */
const props = withDefaults(defineProps<{
  form: SoilInspectionForm
  /** Field name for the title ("Registrar vistoria · Talhão 4"). */
  fieldName?: string | null
}>(), { fieldName: null })

const emit = defineEmits<{
  close: []
  saved: []
}>()

// The form object is created once per host; `values` is reactive and bound with v-model.
const { values, errors, formError, stateOptions, saving, validityNotice, noteMaxLength, maxDate } = props.form
props.form.reset()

const open = ref(true)
const formId = useId()
const noteId = useId()
const noteDescriptionId = useId()

const title = computed(() => (props.fieldName ? `Registrar vistoria · ${props.fieldName}` : 'Registrar vistoria'))

const state = computed<InspectableSoilState | null>({
  get: () => values.state,
  set: (value) => { values.state = value },
})

const noteLength = computed(() => values.note.length)

function close() {
  open.value = false
  emit('close')
}

async function onSubmit() {
  const saved = await props.form.submit()
  if (!saved) return
  emit('saved')
  close()
}
</script>

<template>
  <BaseModal
    v-model:open="open"
    icon="drop"
    :title="title"
    description="Informe o estado do solo observado no talhão. A vistoria vale mais que o cálculo enquanto estiver válida."
    @close="emit('close')"
  >
    <!-- method="post": a submit before hydration must never put the form in the URL. -->
    <form :id="formId" class="soil-inspection-form" method="post" novalidate @submit.prevent="onSubmit">
      <ToastAlert v-if="formError" tone="error" title="Não foi possível registrar a vistoria" :message="formError" :dismissible="false" />

      <BaseSelect
        v-model="state"
        label="Estado do solo"
        name="state"
        :items="stateOptions"
        data-autofocus
        required
        :error="errors.state"
      />

      <div class="soil-inspection-form__pair">
        <TextField
          v-model="values.date"
          label="Data da vistoria"
          type="date"
          name="date"
          :max="maxDate"
          required
          :error="errors.date"
        />
        <!-- Text, not numeric: the iPhone numeric pad has no ":" (the form accepts "7:30"). -->
        <TextField
          v-model="values.time"
          label="Hora"
          inputmode="text"
          autocomplete="off"
          name="time"
          placeholder="07:30"
          :maxlength="5"
          required
          hint="Horário de Brasília (HH:MM)."
          :error="errors.time"
          @blur="values.time = normalizeTimeInput(values.time)"
        />
      </div>

      <div class="soil-inspection-form__group">
        <label :for="noteId" class="soil-inspection-form__label text-label-field">Observação (opcional)</label>
        <textarea
          :id="noteId"
          v-model="values.note"
          class="soil-inspection-form__textarea text-body-md"
          :class="{ 'soil-inspection-form__textarea--error': errors.note }"
          name="note"
          rows="3"
          :maxlength="noteMaxLength"
          :aria-invalid="errors.note ? 'true' : undefined"
          :aria-describedby="noteDescriptionId"
        />
        <p :id="noteDescriptionId" class="soil-inspection-form__meta text-caption-xs">
          <span v-if="errors.note" class="soil-inspection-form__error" role="alert">{{ errors.note }}</span>
          <span class="soil-inspection-form__counter">{{ noteLength }}/{{ noteMaxLength }}</span>
        </p>
      </div>

      <p class="soil-inspection-form__notice text-label-chip" role="note">{{ validityNotice }}</p>
    </form>

    <template #actions>
      <BaseButton variant="ghost" block @click="close">Cancelar</BaseButton>
      <BaseButton type="submit" :form="formId" block :loading="saving">Registrar vistoria</BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.soil-inspection-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
}

.soil-inspection-form__pair {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-14);
}

.soil-inspection-form__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.soil-inspection-form__label {
  color: var(--color-text-primary);
}

.soil-inspection-form__textarea {
  display: block;
  box-sizing: border-box;
  width: 100%;
  min-height: 78px;
  padding: var(--space-12) var(--space-14);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-primary);
  resize: vertical;
}

.soil-inspection-form__textarea:focus-visible {
  outline: none;
  border-color: var(--color-green-700);
  box-shadow: var(--focus-ring);
}

.soil-inspection-form__textarea--error {
  border-color: var(--color-status-unsuitable);
}

.soil-inspection-form__meta {
  display: flex;
  gap: var(--space-8);
  justify-content: space-between;
  margin: 0;
}

.soil-inspection-form__error {
  color: var(--color-status-unsuitable);
}

.soil-inspection-form__counter {
  margin-inline-start: auto;
  color: var(--color-text-tertiary);
}

.soil-inspection-form__notice {
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-info-rain-bg);
  color: color-mix(in srgb, var(--color-info-rain) 70%, var(--color-green-950));
}
</style>
