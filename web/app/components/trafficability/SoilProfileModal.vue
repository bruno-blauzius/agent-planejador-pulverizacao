<script setup lang="ts">
import type { SoilProfileForm } from '~/composables/useSoilProfileForm'

/**
 * "Perfil de solo" of a field (ADR 0022, phase 2; no Figma frame: follows the
 * relief adjustment modal 71:1395): soil texture and the linked rain gauge.
 * "Não informada" and "Nenhum (usa o modelo de previsão)" are real choices
 * (null). State, validation and saving come from `form` (useSoilProfileForm).
 * The host calls `form.open()` when it opens the modal, never on mount: coming
 * back from the gauge registry must keep what the user already chose.
 */
const props = withDefaults(defineProps<{
  form: SoilProfileForm
  fieldName?: string | null
  /** `registry.write` on gauges: offers "Gerenciar pluviômetros". */
  canManageGauges?: boolean
}>(), { fieldName: null, canManageGauges: false })

const emit = defineEmits<{
  close: []
  saved: []
  manageGauges: []
}>()

// The select works with strings; null options travel as this marker.
const NONE = '__none__'

// The form object is created once per host; `values` is reactive.
const { values, errors, formError, textureOptions, gaugeOptions, gaugesPending, gaugesError, saving, notice, pending, error, ready } = props.form

const open = ref(true)
const formId = useId()

const title = computed(() => (props.fieldName ? `Perfil de solo · ${props.fieldName}` : 'Perfil de solo'))

const textureItems = textureOptions.map(option => ({ value: option.value ?? NONE, label: option.label }))
const gaugeItems = computed(() => gaugeOptions.value.map(option => ({ value: option.value ?? NONE, label: option.label })))

const texture = computed<string | null>({
  get: () => values.soilTexture ?? NONE,
  set: (value) => {
    values.soilTexture = textureOptions.find(option => option.value === value)?.value ?? null
  },
})

const gaugeId = computed<string | null>({
  get: () => values.rainGaugeId ?? NONE,
  set: (value) => { values.rainGaugeId = value === NONE ? null : value },
})

const isLoading = computed(() => pending.value && !props.form.profile.value)

/** Loading or failure of the gauge list, under the gauge select (the select keeps working with what it has). */
const gaugeHint = computed(() => (gaugesPending.value ? 'Carregando pluviômetros…' : undefined))
const gaugeError = computed(() => errors.value.rainGaugeId
  ?? (gaugesError.value ? `Não foi possível carregar os pluviômetros: ${gaugesError.value.message}` : undefined))

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
    icon="layers"
    :title="title"
    description="Textura do solo e pluviômetro que atende o talhão. Os dois definem como o estado do solo é calculado."
    @close="emit('close')"
  >
    <div v-if="isLoading" class="soil-profile-form__loading text-caption" role="status">Carregando perfil de solo…</div>

    <ToastAlert
      v-else-if="error && !form.profile.value"
      tone="error"
      title="Não foi possível carregar o perfil de solo"
      :message="error.message"
      :dismissible="false"
    />

    <!-- method="post": a submit before hydration must never put the form in the URL. -->
    <form v-else :id="formId" class="soil-profile-form" method="post" novalidate @submit.prevent="onSubmit">
      <ToastAlert v-if="formError" tone="error" title="Não foi possível salvar o perfil de solo" :message="formError" :dismissible="false" />

      <BaseSelect
        v-model="texture"
        label="Textura do solo"
        name="soilTexture"
        :items="textureItems"
        data-autofocus
        :error="errors.soilTexture"
      />

      <div class="soil-profile-form__group">
        <BaseSelect
          v-model="gaugeId"
          label="Pluviômetro vinculado"
          name="rainGaugeId"
          :items="gaugeItems"
          :hint="gaugeHint"
          :error="gaugeError"
        />
        <div v-if="canManageGauges" class="soil-profile-form__manage">
          <BaseButton variant="ghost" size="sm" icon="gauge" @click="emit('manageGauges')">Gerenciar pluviômetros</BaseButton>
        </div>
      </div>

      <p class="soil-profile-form__notice text-label-chip" role="note">{{ notice }}</p>
    </form>

    <template #actions>
      <BaseButton variant="ghost" block @click="close">Cancelar</BaseButton>
      <BaseButton type="submit" :form="formId" block :loading="saving" :disabled="!ready">Salvar perfil</BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.soil-profile-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
}

.soil-profile-form__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.soil-profile-form__manage {
  display: flex;
  margin-inline-start: calc(-1 * var(--space-14));
}

.soil-profile-form__loading {
  margin: 0;
  color: var(--color-text-secondary);
}

.soil-profile-form__notice {
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-info-rain-bg);
  color: color-mix(in srgb, var(--color-info-rain) 70%, var(--color-green-950));
}
</style>
