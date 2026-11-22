<script setup lang="ts">
import type { WizardStepKey } from '~/types/wizard'
import type { BreadcrumbItem } from '~/components/PageHeader.vue'

/**
 * New-farm wizard (Figma 35:355 step 1; shell compared with 14:30).
 * Presentation only: state, steps (?step=), validation, draft and saving come
 * from useFarmWizard.
 */
definePageMeta({ permission: 'registry.write' })
useHead({ title: 'Nova fazenda · Agentes Agro' })

const {
  identification,
  location,
  perimeterImportAccept,
  importPerimeterFile,
  importing,
  importErrorMessage,
  importSummary,
  perimeterAreaLabel,
  setDrawnPerimeter,
  clearPerimeter,
  setForecastPoint,
  pasteCoordinates,
  locateMe,
  locating,
  locateError,
  headquartersWarning,
  undoPerimeter,
  canUndoPerimeter,
  ownerSearch,
  isOwnerSearchActive,
  ownerSuggestions,
  ownerSearchPending,
  ownerSearchError,
  newOwnerSuggestion,
  newOwnerPrefillName,
  ownerFormPreset,
  ownerRows,
  addOwner,
  linkRegisteredOwner,
  removeOwner,
  setOwnerShare,
  shareTotalLabel,
  shareProgress,
  isShareComplete,
  ownerNotice,
  dismissOwnerNotice,
  alertsBannerVisible,
  dismissAlertsBanner,
  confirmed,
  review,
  currentStep,
  stepNumber,
  steps,
  isFirstStep,
  isLastStep,
  next,
  back,
  goTo,
  cancel,
  fieldErrors,
  touch,
  preview,
  draftSavedLabel,
  draftSavedAt,
  save,
  saveAndExit,
  saving,
  saveError,
} = useFarmWizard()

const BREADCRUMBS: readonly BreadcrumbItem[] = [
  { label: 'Fazendas', to: '/farms' },
  { label: 'Nova fazenda' },
]

// Step headings are presentation copy of the Figma frames.
const STEP_INTRO: Readonly<Record<WizardStepKey, { title: string, description: string }>> = {
  identification: {
    title: 'Como se chama a fazenda?',
    description: 'Esses dados identificam a fazenda nos relatórios e no app do operador.',
  },
  location: {
    title: 'Onde fica a fazenda?',
    description: 'O perímetro é usado para buscar a previsão do tempo certa e calcular a área automaticamente.',
  },
  owners: {
    title: 'Quem responde pela fazenda?',
    description: 'Vincule os proprietários e a participação de cada um. O total precisa fechar 100%.',
  },
  review: {
    title: 'Tudo certo?',
    description: 'Revise os dados antes de salvar. Você pode editar qualquer etapa depois.',
  },
}

const headingId = useId()
const heading = ref<HTMLElement | null>(null)
const errorMessage = ref<HTMLElement | null>(null)
const form = ref<HTMLFormElement | null>(null)
const isSubmitting = ref(false)

const intro = computed(() => STEP_INTRO[currentStep.value])
const totalSteps = computed(() => steps.value.length)

async function onSubmit() {
  isSubmitting.value = true
  const succeeded = isLastStep.value ? await save() : await next()
  if (succeeded) await focusHeading()
  else await focusFirstProblem()
  isSubmitting.value = false
}

/** Every blocked attempt lands focus on the reason: the save error, else the first invalid field. */
async function focusFirstProblem() {
  await nextTick()
  if (saveError.value) {
    errorMessage.value?.focus()
    return
  }
  form.value
    ?.querySelector<HTMLElement>('input[aria-invalid="true"], textarea[aria-invalid="true"], [data-focus-on-error]')
    ?.focus()
}

/** Moves focus to the step heading so keyboard and screen reader users follow the change. */
async function focusHeading() {
  await nextTick()
  heading.value?.focus()
}

// Step changes outside a submit (Voltar, Editar, steps list). Submits decide focus in onSubmit.
watch(currentStep, () => {
  if (!isSubmitting.value) void focusHeading()
})
</script>

<template>
  <div class="new-farm">
    <PageHeader title="Cadastrar fazenda" :breadcrumbs="BREADCRUMBS" />

    <div class="new-farm__wizard">
      <WizardSteps
        class="new-farm__steps"
        :steps="steps"
        :draft-status="draftSavedLabel"
        :draft-saved-at="draftSavedAt"
        @select="goTo"
      />

      <form
        ref="form"
        method="post"
        novalidate
        class="new-farm__form"
        :aria-labelledby="headingId"
        @submit.prevent="onSubmit"
      >
        <header class="new-farm__intro">
          <p class="new-farm__step-count text-overline">Passo {{ stepNumber }} de {{ totalSteps }}</p>
          <h2 :id="headingId" ref="heading" class="new-farm__title text-h3" tabindex="-1">{{ intro.title }}</h2>
          <p class="new-farm__description text-body-md">{{ intro.description }}</p>
        </header>

        <WizardIdentificationStep
          v-if="currentStep === 'identification'"
          v-model:name="identification.name"
          v-model:code="identification.code"
          v-model:cooperative="identification.cooperative"
          v-model:production-type="identification.productionType"
          :errors="fieldErrors"
          @touch="touch"
        />
        <WizardLocationStep
          v-else-if="currentStep === 'location'"
          v-model:mode="location.mode"
          v-model:perimeter-method="location.perimeterMethod"
          v-model:postal-code="location.postalCode"
          v-model:city="location.city"
          v-model:state="location.state"
          v-model:car-number="location.carNumber"
          v-model:latitude="location.latitude"
          v-model:longitude="location.longitude"
          :perimeter="location.perimeter"
          :forecast-point="location.forecastPoint"
          :errors="fieldErrors"
          :perimeter-area-label="perimeterAreaLabel"
          :import-accept="perimeterImportAccept"
          :importing="importing"
          :import-error="importErrorMessage"
          :import-summary="importSummary"
          :locating="locating"
          :locate-error="locateError"
          :headquarters-warning="headquartersWarning"
          :can-undo="canUndoPerimeter"
          :coordinate-paste="pasteCoordinates"
          @touch="touch"
          @draw-perimeter="setDrawnPerimeter"
          @clear-perimeter="clearPerimeter"
          @set-forecast-point="setForecastPoint"
          @import-file="importPerimeterFile"
          @locate="locateMe"
          @undo="undoPerimeter"
        />
        <WizardReviewStep
          v-else-if="currentStep === 'review'"
          v-model:confirmed="confirmed"
          :sections="review"
          :confirm-error="fieldErrors.confirmed"
          @edit="goTo"
        />
        <WizardOwnersStep
          v-else
          v-model:search="ownerSearch"
          :rows="ownerRows"
          :suggestions="ownerSuggestions"
          :search-active="isOwnerSearchActive"
          :search-pending="ownerSearchPending"
          :search-error="ownerSearchError?.message ?? null"
          :new-owner-suggestion="newOwnerSuggestion"
          :new-owner-prefill-name="newOwnerPrefillName"
          :owner-form-preset="ownerFormPreset"
          :share-total-label="shareTotalLabel"
          :share-progress="shareProgress"
          :is-share-complete="isShareComplete"
          :errors="fieldErrors"
          :alerts-banner-visible="alertsBannerVisible"
          :notice="ownerNotice"
          @link="addOwner"
          @register="linkRegisteredOwner"
          @remove="removeOwner"
          @update-share="setOwnerShare"
          @touch="touch"
          @dismiss-banner="dismissAlertsBanner"
          @dismiss-notice="dismissOwnerNotice"
        />

        <p v-if="saveError" ref="errorMessage" class="new-farm__error text-body-sm" role="alert" tabindex="-1">{{ saveError }}</p>

        <WizardFooter
          :is-first-step="isFirstStep"
          :is-last-step="isLastStep"
          :saving="saving"
          @back="back"
          @cancel="cancel"
          @save-and-exit="saveAndExit"
        />
      </form>

      <WizardPreview
        class="new-farm__preview"
        :name="preview.name"
        :rows="preview.rows"
        :perimeter="preview.perimeter"
        :hint="preview.hint"
      />
    </div>
  </div>
</template>

<style scoped>
.new-farm {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
}

.new-farm__wizard {
  display: grid;
  grid-template-columns: var(--wizard-steps-width) minmax(0, 1fr) var(--wizard-preview-width);
  gap: var(--space-24);
  align-items: start;
}

.new-farm__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
  min-width: 0;
  padding: var(--space-28);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.new-farm__intro {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.new-farm__step-count {
  margin: 0;
  color: var(--color-coffee-600);
}

.new-farm__title {
  margin: 0;
  color: var(--color-text-primary);
}

.new-farm__title:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.new-farm__title:focus:not(:focus-visible) {
  outline: none;
}

.new-farm__description {
  max-width: var(--wizard-intro-width);
  margin: 0;
  color: var(--color-text-secondary);
}

.new-farm__error {
  margin: 0;
  outline: none;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}

/* Medium: preview moves under the form. */
@media (max-width: 1199px) {
  .new-farm__wizard {
    grid-template-columns: var(--wizard-steps-width) minmax(0, 1fr);
  }

  .new-farm__preview {
    grid-column: 2;
  }
}

/* Narrow: single column (steps become a horizontal row). */
@media (max-width: 959px) {
  .new-farm__wizard {
    grid-template-columns: minmax(0, 1fr);
  }

  .new-farm__preview {
    grid-column: auto;
  }
}

@media (max-width: 599px) {
  .new-farm__form {
    padding: var(--space-20);
  }
}
</style>
