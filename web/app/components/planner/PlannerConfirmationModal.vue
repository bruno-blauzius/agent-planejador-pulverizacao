<script setup lang="ts">
import { reactive } from 'vue'
import type { ChecklistItem, ChecklistStatus } from '~/components/planner/PlannerChecklist.vue'
import type { ReviewCheckStatus } from '~/types/planner'

/**
 * "Registrar aplicação planejada?" (Figma 19:8): the human confirmation before
 * an application is registered. It only presents the state of
 * `useApplicationConfirmation` (shared with the page, which owns the instance):
 * the checks, the blocking and the expiry are decided by the API and the
 * composable. Nothing is registered without the checkbox.
 */
export type ApplicationConfirmation = ReturnType<typeof useApplicationConfirmation>

const props = withDefaults(
  defineProps<{
    confirmation: ApplicationConfirmation
    /** Fields of the plan; with more than one, the user picks which one to review. */
    fields?: readonly { value: string, label: string }[]
    currentFieldId?: string | null
  }>(),
  { fields: () => [], currentFieldId: null },
)

const emit = defineEmits<{
  'select-field': [fieldId: string]
}>()

// The instance is created once by the page; reactive() unwraps its refs for the template.
const c = reactive(props.confirmation)

const CHECK_STATUS: Record<ReviewCheckStatus, ChecklistStatus> = { ok: 'passed', warning: 'warning', blocked: 'failed' }

const isOpen = computed({
  get: () => c.isOpen,
  set: (open) => {
    if (!open) c.close()
  },
})

const checklist = computed<ChecklistItem[]>(() =>
  (c.review?.checks ?? [])
    // An approved operator/equipment check is already the row below (Figma 19:483); a problem still shows.
    .filter(check => check.code !== 'operator_equipment' || check.status !== 'ok')
    .map(check => ({
      key: check.code,
      title: check.title,
      detail: check.detail,
      status: CHECK_STATUS[check.status],
    })),
)

const fieldChoice = computed({
  get: () => props.currentFieldId ?? '',
  set: (fieldId: string) => {
    if (fieldId && fieldId !== props.currentFieldId) emit('select-field', fieldId)
  },
})

/** One message at a time, most specific first. */
const problem = computed<{ tone: 'error' | 'attention', title: string, message?: string, canReload: boolean } | null>(() => {
  if (c.isForbidden) {
    return { tone: 'error', title: 'Você não tem permissão para registrar aplicações', message: 'Fale com o administrador da sua empresa.', canReload: false }
  }
  if (c.isPlanOutOfContext) {
    return { tone: 'attention', title: 'Este plano não é desta fazenda', message: 'Gere um novo plano.', canReload: false }
  }
  if (c.isAlreadyRegistered) {
    return { tone: 'attention', title: 'Esta aplicação já foi registrada', canReload: false }
  }
  if (c.isStale) {
    return { tone: 'attention', title: 'As condições mudaram', message: 'Revise novamente antes de confirmar.', canReload: true }
  }
  if (c.isWeatherUnavailable) {
    return { tone: 'attention', title: 'A previsão do tempo está indisponível', message: 'Sem a previsão não dá para conferir as condições.', canReload: true }
  }
  const error = c.loadError ?? c.submitError
  return error ? { tone: 'error', title: 'Não foi possível concluir', message: error.message, canReload: true } : null
})

const hasWarnings = computed(() => c.review?.checks.some(check => check.status === 'warning') ?? false)
const confirmDisabled = computed(() =>
  !c.canSubmit || c.isStale || c.isAlreadyRegistered || c.isForbidden || c.isPlanOutOfContext)
const showReview = computed(() => c.review !== null && !c.isForbidden && !c.isPlanOutOfContext)

function confirm() {
  void c.submit()
}
</script>

<template>
  <BaseModal
    v-model:open="isOpen"
    eyebrow="Confirmação humana"
    title="Registrar aplicação planejada?"
    description="O agente sugere, você decide. Nada é registrado sem a sua confirmação."
    icon="shield"
  >
    <div class="confirmation" :aria-busy="c.loading || undefined">
      <SegmentedControl
        v-if="fields.length > 1"
        v-model="fieldChoice"
        :options="fields"
        label="Talhão a registrar"
        block
      />

      <ToastAlert
        v-if="problem"
        :tone="problem.tone"
        :title="problem.title"
        :message="problem.message"
        :dismissible="false"
      >
        <template v-if="problem.canReload" #actions>
          <BaseButton variant="secondary" size="sm" icon="undo" :loading="c.loading" @click="c.reload()">
            {{ c.isStale ? 'Revisar novamente' : 'Tentar de novo' }}
          </BaseButton>
        </template>
      </ToastAlert>

      <div v-if="c.loading && !c.review" class="confirmation__skeleton" aria-hidden="true" />
      <span v-if="c.loading" class="visually-hidden" role="status">Carregando a revisão…</span>

      <template v-if="showReview && c.review">
        <dl class="confirmation__summary">
          <div class="confirmation__cell">
            <dt class="confirmation__label text-label-micro">Talhão</dt>
            <dd class="confirmation__value text-label-strong">{{ c.review.fieldTitle }}</dd>
            <dd class="confirmation__sub text-caption">{{ c.review.areaLabel }}</dd>
          </div>
          <div class="confirmation__cell">
            <dt class="confirmation__label text-label-micro">Janela</dt>
            <dd class="confirmation__value text-label-strong">{{ c.review.dayLabel }}</dd>
            <dd class="confirmation__sub text-caption">{{ c.review.timeLabel }}</dd>
          </div>
          <div class="confirmation__cell">
            <dt class="confirmation__label text-label-micro">Produto</dt>
            <dd class="confirmation__value text-label-strong">{{ c.review.productName }}</dd>
            <dd class="confirmation__sub text-caption">{{ c.review.doseLabel }}</dd>
          </div>
        </dl>

        <p class="confirmation__mix">
          <span class="confirmation__mix-label text-label-field">Volume de calda necessário</span>
          <span class="confirmation__mix-value text-h4">{{ c.review.mixVolumeLabel }}</span>
          <span class="confirmation__mix-tanks text-body-sm">{{ c.review.tanksLabel }}</span>
        </p>

        <PlannerChecklist :items="checklist" />

        <p v-if="c.isBlocked" class="confirmation__blocked text-body-sm" role="alert">
          <AppIcon name="alert" :size="16" />
          <span>
            Não é possível registrar: {{ c.review.blockedTitles.join(', ') }}.
            Volte e ajuste o plano.
          </span>
        </p>

        <div class="confirmation__operator">
          <span class="confirmation__operator-icon" aria-hidden="true"><AppIcon name="user" :size="18" /></span>
          <p class="confirmation__operator-text">
            <span class="confirmation__label-plain text-caption">Operador e equipamento</span>
            <span class="text-label-field">{{ c.review.operatorEquipment }}</span>
          </p>
        </div>

        <BaseCheckbox
          v-model="c.acknowledged"
          size="md"
          :disabled="c.isBlocked || c.isStale"
          :label="hasWarnings
            ? 'Revisei as condições, incluindo os avisos, e confirmo a aplicação'
            : 'Revisei as condições e confirmo a aplicação'"
        />
      </template>
    </div>

    <template #actions>
      <div class="confirmation__actions">
        <BaseButton variant="secondary" data-autofocus @click="c.close()">Voltar e ajustar</BaseButton>
        <BaseButton variant="primary" :disabled="confirmDisabled" :loading="c.submitting" @click="confirm">
          Confirmar e registrar
        </BaseButton>
      </div>
    </template>
  </BaseModal>
</template>

<style scoped>
.confirmation {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  min-width: 0;
}

.confirmation__actions {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: var(--space-10);
  min-width: 0;
}

.confirmation__actions > * {
  flex: 1 1 auto;
}

.confirmation__skeleton {
  min-height: var(--planner-skeleton-plan-height);
  border-radius: var(--radius-lg);
  background: var(--color-sand);
  animation: confirmation-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

.confirmation__summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 0;
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-lg);
}

.confirmation__cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-16);
  overflow-wrap: anywhere;
}

.confirmation__cell + .confirmation__cell {
  border-left: var(--border-width-hairline) solid var(--color-border);
}

.confirmation__cell dd {
  margin: 0;
}

.confirmation__label {
  color: var(--color-text-tertiary);
  text-transform: uppercase;
}

.confirmation__value {
  color: var(--color-text-primary);
}

.confirmation__sub {
  color: var(--color-text-secondary);
}

.confirmation__mix {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4) var(--space-10);
  align-items: baseline;
  margin: 0;
  padding: var(--space-12) var(--space-16);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.confirmation__mix-label {
  flex: 1 1 auto;
}

.confirmation__mix-tanks {
  color: var(--color-green-700);
}

.confirmation__blocked {
  display: flex;
  gap: var(--space-8);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-10);
  border-radius: var(--radius-md);
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}

.confirmation__operator {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
}

.confirmation__operator-icon {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border-radius: var(--radius-pill);
  background: var(--color-coffee-600);
  color: var(--color-white);
}

.confirmation__operator-text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  margin: 0;
  color: var(--color-text-primary);
}

.confirmation__label-plain {
  color: var(--color-text-tertiary);
}

@keyframes confirmation-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (max-width: 599px) {
  .confirmation__summary {
    grid-template-columns: minmax(0, 1fr);
  }

  .confirmation__cell + .confirmation__cell {
    border-top: var(--border-width-hairline) solid var(--color-border);
    border-left: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .confirmation__skeleton {
    animation: none;
  }
}
</style>
