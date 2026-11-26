<script setup lang="ts">
import type { OwnerForm, OwnerFormMode } from '~/composables/useOwnerForm'
import type { SegmentOption } from '~/components/SegmentedControl.vue'
import type { PersonType } from '~/types/owner'

/**
 * Body of the owner form (Figma 39:1045 Dados, 39:1360 Vínculos, 39:1893 edit).
 * Presentation only: state, validation and saving live in the `form` object
 * from useOwnerForm, created by the host (OwnerModal, or the new-farm wizard
 * step 3 in `wizard` mode). Actions (continue/save) belong to the host.
 */
const props = withDefaults(
  defineProps<{
    form: OwnerForm
    mode: OwnerFormMode
    /** `wizard`: name of the farm being registered ("Vínculo com a Fazenda X", Figma 41:1151). */
    farmName?: string
  }>(),
  { farmName: undefined },
)

// The form object is created once per host; its refs are unwrapped here so the
// template can bind them with v-model.
const {
  step,
  personType,
  name,
  document,
  birthDate,
  phone,
  email,
  windowAlertsEnabled,
  isDocumentLocked,
  labels,
  fieldErrors,
  documentHint,
  formError,
  pendingDocumentNotice,
  links,
  linksSummary,
  wizardShare,
  wizardShareContext,
  linksLoading,
  touch,
  toggleFarm,
  setShare,
  touchShare,
  goToLinks,
} = props.form

const PERSON_TYPES: readonly SegmentOption<PersonType>[] = [
  { value: 'individual', label: 'Pessoa física' },
  { value: 'company', label: 'Pessoa jurídica' },
]

const isEdit = computed(() => props.mode === 'edit')
const isWizard = computed(() => props.mode === 'wizard')
/** Edit and wizard pair the document with WhatsApp (Figma 39:1925, 41:1134); create pairs it with the birth date. */
const pairsDocumentWithPhone = computed(() => isEdit.value || isWizard.value)
const isIndividual = computed(() => personType.value === 'individual')
const selectedLinks = computed(() => links.value.filter((link) => link.selected))
const isPendingNoticeDismissed = ref(false)
const alertsDescriptionId = useId()
const alertsErrorId = useId()
const shareErrorPrefix = useId()

function shareErrorId(farmId: string): string {
  return `${shareErrorPrefix}-${farmId}`
}

/** The switch is described by its card text and, when present, by its validation error. */
const alertsDescribedBy = computed(() =>
  fieldErrors.value.windowAlerts ? `${alertsDescriptionId} ${alertsErrorId}` : alertsDescriptionId,
)

function totalTone(total: number): 'complete' | 'partial' | 'over' {
  if (total > 100) return 'over'
  return total === 100 ? 'complete' : 'partial'
}

function onShareInput(farmId: string, event: Event) {
  setShare(farmId, (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="owner-form">
    <!-- ===== Dados ===== -->
    <template v-if="step === 'data'">
      <ToastAlert
        v-if="pendingDocumentNotice && !isPendingNoticeDismissed"
        tone="attention"
        :title="pendingDocumentNotice.title"
        :message="pendingDocumentNotice.description"
        @dismiss="isPendingNoticeDismissed = true"
      />

      <SegmentedControl
        v-if="!isEdit"
        v-model="personType"
        :options="PERSON_TYPES"
        label="Tipo de pessoa"
        block
      />

      <TextField
        v-model="name"
        :label="labels.name"
        name="name"
        autocomplete="name"
        :error="fieldErrors.name"
        data-autofocus
        @blur="touch('name')"
      />

      <div class="owner-form__row">
        <TextField
          v-if="isDocumentLocked"
          :model-value="document"
          :label="labels.document"
          name="document"
          disabled
        />
        <TextField
          v-else
          v-model="document"
          :label="labels.document"
          name="document"
          inputmode="numeric"
          :hint="documentHint ?? undefined"
          :error="fieldErrors.document"
          @blur="touch('document')"
        />
        <TextField
          v-if="pairsDocumentWithPhone"
          v-model="phone"
          label="WhatsApp"
          name="phone"
          type="tel"
          inputmode="tel"
          autocomplete="tel"
          :placeholder="isWizard ? '(35) 99812-4470' : undefined"
          :error="fieldErrors.phone"
          @blur="touch('phone')"
        />
        <TextField
          v-else-if="isIndividual"
          v-model="birthDate"
          label="Data de nascimento"
          name="birthDate"
          inputmode="numeric"
          placeholder="dd/mm/aaaa"
          :error="fieldErrors.birthDate"
          @blur="touch('birthDate')"
        />
      </div>

      <div v-if="!pairsDocumentWithPhone" class="owner-form__row">
        <TextField
          v-model="phone"
          label="WhatsApp"
          name="phone"
          type="tel"
          inputmode="tel"
          autocomplete="tel"
          placeholder="(35) 99812-4470"
          :error="fieldErrors.phone"
          @blur="touch('phone')"
        />
        <TextField
          v-model="email"
          label="E-mail (opcional)"
          name="email"
          type="email"
          autocomplete="email"
          placeholder="nome@email.com"
          :error="fieldErrors.email"
          @blur="touch('email')"
        />
      </div>

      <!-- Wizard: share in the farm being registered (Figma 41:1151). -->
      <section v-if="isWizard" class="owner-form__wizard-link" :aria-label="farmName ? `Vínculo com a ${farmName}` : 'Vínculo com esta fazenda'">
        <p class="owner-form__wizard-title text-label-strong">
          {{ farmName ? `Vínculo com a ${farmName}` : 'Vínculo com esta fazenda' }}
        </p>
        <div class="owner-form__wizard-row">
          <TextField
            v-model="wizardShare"
            class="owner-form__wizard-share"
            label="Participação (%)"
            name="wizardShare"
            inputmode="decimal"
            :error="fieldErrors.wizardShare"
            @blur="touch('wizardShare')"
          />
          <!-- Beside the field as in the Figma; announced politely when the share changes the hint. -->
          <div v-if="wizardShareContext" class="owner-form__wizard-context text-body-sm" aria-live="polite">
            <p class="owner-form__wizard-current">{{ wizardShareContext.currentLabel }}</p>
            <p v-if="wizardShareContext.hint" class="owner-form__wizard-hint">{{ wizardShareContext.hint }}</p>
          </div>
        </div>
      </section>

      <!-- Documents (ADR 0020): sent once the owner exists, so only while editing. -->
      <OwnerDocuments
        v-if="isEdit && form.ownerId.value"
        :owner-id="form.ownerId.value"
        :person-type="personType"
        :documents="form.documents.value"
        :pending="form.documentsPending.value"
        @changed="form.refreshOwner()"
      />
      <FileDropzone
        v-else-if="mode !== 'wizard'"
        title="Documento com foto (opcional)"
        description="RG, CNH ou contrato social · envie depois de salvar, ao editar o proprietário"
        disabled
      />

      <div v-if="isEdit" class="owner-form__links-summary text-label-field">
        <span class="owner-form__links-summary-text">{{ linksSummary }}</span>
        <button type="button" class="owner-form__link-button text-label-field" @click="goToLinks()">
          Editar vínculos
        </button>
      </div>
    </template>

    <!-- ===== Vínculos ===== -->
    <template v-else>
      <p v-if="linksLoading" class="owner-form__muted text-body-md" role="status">Carregando fazendas…</p>
      <p v-else-if="links.length === 0" class="owner-form__muted text-body-md">Nenhuma fazenda cadastrada para vincular.</p>
      <ul v-else class="owner-form__farms">
        <li
          v-for="link in links"
          :key="link.farmId"
          class="owner-form__farm"
          :class="{ 'owner-form__farm--selected': link.selected, 'owner-form__farm--error': link.selected && link.error }"
        >
          <div class="owner-form__farm-row">
            <BaseCheckbox
              :model-value="link.selected"
              :label="link.farmName"
              size="md"
              class="owner-form__farm-check"
              @update:model-value="toggleFarm(link.farmId, $event)"
            />
            <label v-if="link.selected" class="owner-form__share">
              <span class="owner-form__muted text-body-sm">Participação</span>
              <span class="owner-form__share-box">
                <input
                  class="owner-form__share-input text-label-strong"
                  :value="link.share"
                  inputmode="decimal"
                  :aria-label="`Participação em ${link.farmName}, em porcentagem`"
                  :aria-invalid="Boolean(link.error) || undefined"
                  :aria-describedby="link.error ? shareErrorId(link.farmId) : undefined"
                  @input="onShareInput(link.farmId, $event)"
                  @blur="touchShare(link.farmId)"
                >
                <span class="text-label-strong" aria-hidden="true">%</span>
              </span>
            </label>
          </div>
          <p v-if="link.selected && link.error" :id="shareErrorId(link.farmId)" class="owner-form__error text-body-sm" role="alert">{{ link.error }}</p>
        </li>
      </ul>

      <div v-for="link in selectedLinks" :key="`total-${link.farmId}`" class="owner-form__total">
        <div class="owner-form__total-legend text-body-sm">
          <span class="owner-form__muted">{{ link.totalLabel }}</span>
          <span class="owner-form__total-value text-label-badge" :class="`owner-form__total-value--${totalTone(link.totalShare)}`">
            {{ link.totalShareLabel }}
          </span>
        </div>
        <div
          class="owner-form__track"
          role="progressbar"
          :aria-label="link.totalLabel"
          :aria-valuenow="link.totalShare"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <span
            class="owner-form__fill"
            :class="`owner-form__fill--${totalTone(link.totalShare)}`"
            :style="{ width: `${Math.min(link.totalShare, 100)}%` }"
          />
        </div>
      </div>

      <div class="owner-form__alerts">
        <AppIcon name="message" :size="20" class="owner-form__alerts-icon" />
        <div class="owner-form__alerts-text">
          <p class="text-label-strong">Alertas de janela no WhatsApp</p>
          <p :id="alertsDescriptionId" class="owner-form__muted text-body-sm">
            Avisa quando o agente encontrar uma janela apta nas fazendas deste proprietário.
          </p>
        </div>
        <BaseSwitch
          v-model="windowAlertsEnabled"
          label="Alertas de janela no WhatsApp"
          hide-label
          :described-by="alertsDescribedBy"
        />
      </div>
      <p v-if="fieldErrors.windowAlerts" :id="alertsErrorId" class="owner-form__error text-body-sm" role="alert">
        {{ fieldErrors.windowAlerts }}
      </p>
    </template>

    <ToastAlert
      v-if="formError"
      tone="error"
      title="Não foi possível salvar"
      :message="formError"
      :dismissible="false"
    />
  </div>
</template>

<style scoped>
/* Figma modal body: 20 px between blocks, 12 px between paired fields. */
.owner-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

/* Side by side when there is room; each field wraps below a usable minimum width. */
.owner-form__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
}

.owner-form__row > * {
  flex: 1 1 var(--form-field-min-width);
  min-width: 0;
}

.owner-form__muted {
  color: var(--color-text-tertiary);
}

.owner-form__error {
  margin-top: var(--space-6);
  color: var(--color-status-unsuitable);
}

/* Wizard: link with the farm being registered (Figma 41:1151). */
.owner-form__wizard-link {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-16);
  border-radius: var(--radius-lg);
  background: var(--color-green-100);
}

.owner-form__wizard-title {
  color: var(--color-green-800);
}

.owner-form__wizard-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: flex-end;
}

.owner-form__wizard-share {
  flex: 0 0 var(--wizard-share-field-width);
}

.owner-form__wizard-context {
  display: flex;
  flex: 1 1 var(--form-field-min-width);
  flex-direction: column;
  gap: var(--space-4);
  padding-bottom: var(--space-4);
}

.owner-form__wizard-current {
  color: var(--color-text-secondary);
}

.owner-form__wizard-hint {
  color: var(--color-status-attention);
}

/* Edit: links summary (Figma 39:1956). */
.owner-form__links-summary {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-sand);
}

.owner-form__links-summary-text {
  flex: 1;
  min-width: 0;
  color: var(--color-text-secondary);
}

.owner-form__link-button {
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-green-700);
  cursor: pointer;
}

.owner-form__link-button:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

/* Links (Figma 39:1383 / 39:1393). */
.owner-form__farms {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  margin: 0;
  padding: 0;
  list-style: none;
}

.owner-form__farm {
  padding: var(--space-12);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
}

.owner-form__farm--selected {
  border: var(--border-width-control) solid var(--color-green-700);
  background: var(--color-green-100);
}

.owner-form__farm--error {
  border-color: var(--color-status-unsuitable);
}

.owner-form__farm-row {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  min-height: var(--size-icon-tile-md);
}

.owner-form__farm-check {
  flex: 1;
  min-width: 0;
}

.owner-form__share {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
}

.owner-form__share-box {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
  padding: var(--space-8) var(--space-12);
  border: var(--border-width-control) solid var(--color-green-700);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  color: var(--color-text-primary);
}

.owner-form__share-box:focus-within {
  box-shadow: var(--focus-ring);
}

.owner-form__share-input {
  width: var(--share-input-width);
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: right;
  outline: none;
}

/* Total share bar (Figma "Barra de progresso" 26:174). */
.owner-form__total {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.owner-form__total-legend {
  display: flex;
  gap: var(--space-8);
  justify-content: space-between;
}

.owner-form__total-value--complete {
  color: var(--color-status-suitable);
}

.owner-form__total-value--partial {
  color: var(--color-status-attention);
}

.owner-form__total-value--over {
  color: var(--color-status-unsuitable);
}

.owner-form__track {
  height: var(--size-progress-track);
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--color-border);
}

.owner-form__fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
}

.owner-form__fill--complete {
  background: var(--color-status-suitable);
}

.owner-form__fill--partial {
  background: var(--color-status-attention);
}

.owner-form__fill--over {
  background: var(--color-status-unsuitable);
}

/* WhatsApp alerts (Figma 39:1407). */
.owner-form__alerts {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
}

.owner-form__alerts-icon {
  color: var(--color-green-700);
}

.owner-form__alerts-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
</style>
