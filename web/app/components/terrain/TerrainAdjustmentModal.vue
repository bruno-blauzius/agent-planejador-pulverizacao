<script setup lang="ts">
import type { TerrainAdjustmentForm } from '~/composables/useTerrainAdjustmentForm'
import type { TerrainAdjustmentOrigin } from '~/types/terrain'

/**
 * "Ajustar relevo · Talhão N" modal (Figma 71:1395): data origin, mean and max
 * slope, position, mandatory justification and an info notice. The class is NOT
 * computed here (ADR 0021): the modal shows the current class from the API and
 * that it is recalculated on save. State, validation and saving come from the
 * `form` object (useTerrainAdjustmentForm). Mount it with `v-if` so each opening
 * starts a fresh form (`reset` runs on mount); it emits `saved` after a
 * successful `submit` and the host unmounts it.
 */
const props = defineProps<{
  form: TerrainAdjustmentForm
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

// The form object is created once per host; `values` is reactive and bound with v-model.
const { title, values, errors, formError, hints, originOptions, positionOptions, justificationMaxLength, infoNotice, saving } = props.form
props.form.reset()

const open = ref(true)
const formId = useId()
const justificationId = useId()
const justificationDescriptionId = useId()
const positionErrorId = useId()

const origin = computed<TerrainAdjustmentOrigin>({
  get: () => values.origin,
  set: (value) => { values.origin = value },
})

// Plain strings: the position is `null` until the user (or the current relief) fills it.
const position = computed<string>({
  get: () => values.position ?? '',
  set: (value) => { values.position = positionOptions.find((option) => option.value === value)?.value ?? null },
})

// Figma 71:1408 lists the survey first; only the presentation order changes, not the form default.
const originItems = [...originOptions].sort(
  (a, b) => Number(b.value === 'topographic_survey') - Number(a.value === 'topographic_survey'),
)
const positionItems = positionOptions.map((option) => ({ value: String(option.value), label: option.label }))
const justificationLength = computed(() => values.justification.length)

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
    icon="relief"
    :title="title"
    description="Use quando houver medição melhor que o modelo de elevação (ex.: terraços, levantamento com RTK). O valor calculado continua guardado para comparação."
    @close="emit('close')"
  >
    <!-- method="post": a submit before hydration must never put the form in the URL. -->
    <form :id="formId" class="terrain-adjust" method="post" novalidate @submit.prevent="onSubmit">
      <ToastAlert v-if="formError" tone="error" title="Não foi possível salvar o ajuste" :message="formError" :dismissible="false" />

      <div class="terrain-adjust__group">
        <span class="terrain-adjust__label text-label-field" aria-hidden="true">Origem do dado</span>
        <SegmentedControl v-model="origin" :options="originItems" label="Origem do dado" block />
      </div>

      <div class="terrain-adjust__pair">
        <TextField
          v-model="values.slopeMeanPct"
          label="Declividade média"
          inputmode="decimal"
          autocomplete="off"
          name="slopeMeanPct"
          data-autofocus
          required
          :hint="hints.slopeMean ?? undefined"
          :error="errors.slopeMeanPct"
        >
          <template #trailing><span class="terrain-adjust__unit text-label-meta">%</span></template>
        </TextField>
        <TextField
          v-model="values.slopeMaxPct"
          label="Declividade máxima"
          inputmode="decimal"
          autocomplete="off"
          name="slopeMaxPct"
          required
          :hint="hints.slopeMax ?? undefined"
          :error="errors.slopeMaxPct"
        >
          <template #trailing><span class="terrain-adjust__unit text-label-meta">%</span></template>
        </TextField>
      </div>

      <p class="terrain-adjust__class text-label-meta">
        <span class="terrain-adjust__class-label">{{ hints.currentClass }}</span>
        <span class="terrain-adjust__class-notice text-caption">{{ hints.classNotice }}</span>
      </p>

      <div class="terrain-adjust__group">
        <span class="terrain-adjust__label text-label-field" aria-hidden="true">Posição no relevo</span>
        <SegmentedControl v-model="position" :options="positionItems" label="Posição no relevo" block />
        <p v-if="hints.position" class="terrain-adjust__hint text-caption-xs">{{ hints.position }}</p>
        <p v-if="errors.position" :id="positionErrorId" class="terrain-adjust__error text-caption" role="alert">{{ errors.position }}</p>
      </div>

      <div class="terrain-adjust__group">
        <label :for="justificationId" class="terrain-adjust__label text-label-field">
          Justificativa (obrigatória)
        </label>
        <textarea
          :id="justificationId"
          v-model="values.justification"
          class="terrain-adjust__textarea text-body-md"
          :class="{ 'terrain-adjust__textarea--error': errors.justification }"
          name="justification"
          rows="3"
          :maxlength="justificationMaxLength"
          required
          :aria-invalid="errors.justification ? 'true' : undefined"
          :aria-describedby="justificationDescriptionId"
        />
        <p :id="justificationDescriptionId" class="terrain-adjust__textarea-meta text-caption-xs">
          <span v-if="errors.justification" class="terrain-adjust__error" role="alert">{{ errors.justification }}</span>
          <span class="terrain-adjust__counter">{{ justificationLength }}/{{ justificationMaxLength }}</span>
        </p>
      </div>

      <p class="terrain-adjust__notice text-label-chip" role="note">{{ infoNotice }}</p>
    </form>

    <template #actions>
      <BaseButton variant="ghost" block @click="close">Cancelar</BaseButton>
      <BaseButton type="submit" :form="formId" block :loading="saving">Salvar ajuste</BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.terrain-adjust {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
}

.terrain-adjust__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.terrain-adjust__pair {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-14);
}

.terrain-adjust__unit {
  color: var(--color-text-tertiary);
}

.terrain-adjust__class {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4) var(--space-10);
  align-items: center;
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-sand);
}

.terrain-adjust__class-label {
  color: var(--color-text-primary);
  font-weight: 700;
}

.terrain-adjust__class-notice {
  color: var(--color-text-secondary);
}

.terrain-adjust__label {
  color: var(--color-text-primary);
}

.terrain-adjust__hint {
  margin: 0;
  color: var(--color-text-tertiary);
}

.terrain-adjust__error {
  margin: 0;
  color: var(--color-status-unsuitable);
}

.terrain-adjust__textarea {
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

.terrain-adjust__textarea:focus-visible {
  outline: none;
  border-color: var(--color-green-700);
  box-shadow: var(--focus-ring);
}

.terrain-adjust__textarea--error {
  border-color: var(--color-status-unsuitable);
}

.terrain-adjust__textarea-meta {
  display: flex;
  gap: var(--space-8);
  justify-content: space-between;
  margin: 0;
}

.terrain-adjust__counter {
  margin-inline-start: auto;
  color: var(--color-text-tertiary);
}

.terrain-adjust__notice {
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-info-rain-bg);
  color: color-mix(in srgb, var(--color-info-rain) 70%, var(--color-green-950));
}
</style>
