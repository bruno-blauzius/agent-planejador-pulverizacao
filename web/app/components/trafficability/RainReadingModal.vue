<script setup lang="ts">
import type { RainReadingForm } from '~/composables/useRainReadingForm'
import { normalizeTimeInput } from '~/utils/fieldConditionsForm'

/**
 * "Lançar pluviômetro" (ADR 0022, phase 2; no Figma frame: follows the relief
 * adjustment modal 71:1395). Gauge, rain in mm, date and time of the reading.
 * State, validation and saving come from `form` (useRainReadingForm). The host
 * resets the form when it opens the modal (not when it comes back from the gauge
 * registry). Without a gauge in the farm it explains that one must be
 * registered first.
 */
const props = withDefaults(defineProps<{
  form: RainReadingForm
  /** The farm's gauges are still loading. */
  gaugesPending?: boolean
  /** Message of a failed gauge list load. */
  gaugesError?: string | null
  /** `registry.write`: offers "Cadastrar pluviômetro" in the empty state. */
  canManageGauges?: boolean
}>(), { gaugesPending: false, gaugesError: null, canManageGauges: false })

const emit = defineEmits<{
  close: []
  saved: []
  /** Opens the gauge registration. */
  manageGauges: []
}>()

// The form object is created once per host; `values` is reactive and bound with v-model.
const { values, errors, formError, gaugeOptions, saving, notice, rainMmMax, maxDate } = props.form

const open = ref(true)
const formId = useId()
const rainInputId = useId()

const hasGauges = computed(() => gaugeOptions.value.length > 0)

/** The form may appear after the modal opened (gauges still loading): focus the rain field then. */
watch(hasGauges, async (shown) => {
  if (!shown || !import.meta.client) return
  await nextTick()
  document.getElementById(rainInputId)?.focus()
})

const gaugeId = computed<string | null>({
  get: () => values.gaugeId,
  set: (value) => { values.gaugeId = value },
})

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
    icon="rain"
    title="Lançar pluviômetro"
    description="Registre a chuva lida no pluviômetro da fazenda. A leitura substitui o modelo de previsão nos talhões ligados a ele."
    @close="emit('close')"
  >
    <div v-if="gaugesPending && !hasGauges" class="field-conditions-form__loading text-caption" role="status">
      Carregando pluviômetros…
    </div>

    <ToastAlert
      v-else-if="gaugesError && !hasGauges"
      tone="error"
      title="Não foi possível carregar os pluviômetros"
      :message="gaugesError"
      :dismissible="false"
    />

    <EmptyState
      v-else-if="!hasGauges"
      icon="gauge"
      title="Nenhum pluviômetro nesta fazenda"
      :description="canManageGauges
        ? 'Cadastre um pluviômetro e ligue-o aos talhões para lançar a chuva medida.'
        : 'Peça a um administrador da empresa para cadastrar um pluviômetro nesta fazenda.'"
    >
      <template v-if="canManageGauges" #actions>
        <BaseButton icon="plus" @click="emit('manageGauges')">Cadastrar pluviômetro</BaseButton>
      </template>
    </EmptyState>

    <!-- method="post": a submit before hydration must never put the form in the URL. -->
    <form v-else :id="formId" class="field-conditions-form" method="post" novalidate @submit.prevent="onSubmit">
      <ToastAlert v-if="formError" tone="error" title="Não foi possível lançar a leitura" :message="formError" :dismissible="false" />

      <BaseSelect
        v-model="gaugeId"
        label="Pluviômetro"
        name="gaugeId"
        :items="gaugeOptions"
        required
        :error="errors.gaugeId"
      />

      <TextField
        :id="rainInputId"
        v-model="values.rainMm"
        label="Chuva"
        inputmode="decimal"
        autocomplete="off"
        name="rainMm"
        placeholder="12,5"
        data-autofocus
        required
        :hint="`De 0 a ${rainMmMax} mm, acumulada desde a leitura anterior.`"
        :error="errors.rainMm"
      >
        <template #trailing><span class="field-conditions-form__unit text-label-meta">mm</span></template>
      </TextField>

      <div class="field-conditions-form__pair">
        <TextField
          v-model="values.date"
          label="Data da leitura"
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

      <p class="field-conditions-form__notice text-label-chip" role="note">{{ notice }}</p>
    </form>

    <template #actions>
      <BaseButton variant="ghost" block @click="close">{{ hasGauges ? 'Cancelar' : 'Fechar' }}</BaseButton>
      <BaseButton v-if="hasGauges" type="submit" :form="formId" block :loading="saving">Lançar leitura</BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.field-conditions-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
}

.field-conditions-form__pair {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-14);
}

.field-conditions-form__unit {
  color: var(--color-text-tertiary);
}

.field-conditions-form__loading {
  margin: 0;
  color: var(--color-text-secondary);
}

.field-conditions-form__notice {
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-info-rain-bg);
  color: color-mix(in srgb, var(--color-info-rain) 70%, var(--color-green-950));
}
</style>
