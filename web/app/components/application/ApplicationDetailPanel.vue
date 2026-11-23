<script setup lang="ts">
import type { IconName } from '~/components/app/AppIcon.vue'
import type { ApiError } from '~/services/errors'
import type { ApplicationDetailView } from '~/utils/applications'

/**
 * Side sheet "Aplicação · detalhe" (Figma 63:890, 520 px) over the list. A
 * dialog (focus stays inside, Esc and the scrim close it) labelled by its
 * title. It only presents `ApplicationDetailView`; every number comes ready
 * from the API.
 */
const props = defineProps<{
  view: ApplicationDetailView | null
  pending: boolean
  error: ApiError | null
  isForbidden: boolean
  isNotFound: boolean
  canRepeat: boolean
  downloadHint: string
}>()

const emit = defineEmits<{
  retry: []
  repeat: []
}>()

const open = defineModel<boolean>('open', { default: false })

const STAT_ICONS: Record<'delta_t' | 'wind' | 'rain' | 'duration', IconName> = {
  delta_t: 'thermometer',
  wind: 'wind',
  rain: 'drop',
  duration: 'clock',
}

const titleId = useId()
const heading = computed(() => props.view?.title ?? 'Aplicação')

const titleRef = ref<HTMLElement | null>(null)

function close() {
  open.value = false
}

/** Starts the reading at the title: a safe, non-interactive place inside the dialog. */
function onAfterEnter() {
  // Next task: after Vuetify's own initial focus inside the dialog.
  setTimeout(() => titleRef.value?.focus())
}

function onUpdate(value: boolean) {
  if (!value) close()
}
</script>

<template>
  <v-dialog
    :model-value="open"
    class="application-sheet-overlay"
    scrim="green-950"
    transition="slide-x-reverse-transition"
    :aria-labelledby="titleId"
    @update:model-value="onUpdate"
    @after-enter="onAfterEnter"
  >
    <v-card class="sheet" :elevation="0" :border="false">
      <header class="sheet__header">
        <div class="sheet__heading">
          <p v-if="view" class="sheet__status">
            <ApplicationStatusPill :tone="view.status.tone" :label="view.status.label" />
            <span class="sheet__registered text-caption">{{ view.registeredLabel }}</span>
          </p>
          <h2 :id="titleId" ref="titleRef" class="sheet__title text-h3" tabindex="-1">{{ heading }}</h2>
          <p v-if="view" class="sheet__subtitle text-body-md">{{ view.subtitle }}</p>
        </div>
        <button type="button" class="sheet__close" aria-label="Fechar detalhes" @click="close">
          <AppIcon name="x" :size="18" />
        </button>
      </header>

      <div class="sheet__body" :aria-busy="pending || undefined">
        <template v-if="pending && !view">
          <span class="visually-hidden" role="status">Carregando a aplicação…</span>
          <span v-for="block in 3" :key="block" class="sheet__skeleton" aria-hidden="true" />
        </template>

        <div v-else-if="isForbidden" class="sheet__state">
          <ToastAlert tone="error" title="Você não tem acesso a esta aplicação" :dismissible="false">
            <template #actions>
              <BaseButton variant="secondary" size="sm" @click="close">Fechar</BaseButton>
            </template>
          </ToastAlert>
        </div>
        <div v-else-if="isNotFound" class="sheet__state">
          <ToastAlert tone="attention" title="Esta aplicação não existe mais" :dismissible="false">
            <template #actions>
              <BaseButton variant="secondary" size="sm" @click="close">Fechar</BaseButton>
            </template>
          </ToastAlert>
        </div>
        <div v-else-if="error" class="sheet__state">
          <ToastAlert
            tone="error"
            title="Não foi possível carregar a aplicação"
            :message="error.message"
            :dismissible="false"
          >
            <template #actions>
              <BaseButton variant="secondary" size="sm" icon="undo" @click="emit('retry')">Tentar de novo</BaseButton>
            </template>
          </ToastAlert>
        </div>

        <template v-else-if="view">
          <section v-if="view.conditions" class="sheet__conditions" aria-labelledby="sheet-conditions">
            <header class="sheet__conditions-header">
              <h3 id="sheet-conditions" class="sheet__conditions-title text-label-field">
                <AppIcon name="sparkles" :size="14" />
                Condições registradas na aplicação
              </h3>
              <span class="sheet__verdict text-label-badge">{{ view.conditions.headline }}</span>
            </header>
            <ul class="sheet__stats">
              <li v-for="stat in view.conditions.stats" :key="stat.key" class="sheet__stat">
                <AppIcon :name="STAT_ICONS[stat.key]" :size="16" class="sheet__stat-icon" />
                <span class="sheet__stat-label text-caption">{{ stat.label }}</span>
                <span class="sheet__stat-value text-label-strong">{{ stat.value }}</span>
              </li>
            </ul>
          </section>
          <p v-else-if="view.cancellationLabel" class="sheet__cancelled text-body-md" role="status">
            <AppIcon name="alert" :size="16" />
            {{ view.cancellationLabel }}
          </p>

          <section class="sheet__card" aria-labelledby="sheet-product">
            <h3 id="sheet-product" class="sheet__card-title text-label-strong">Produto e calda</h3>
            <dl class="sheet__rows">
              <div v-for="row in view.productRows" :key="row.label" class="sheet__row">
                <dt class="sheet__row-label text-caption">{{ row.label }}</dt>
                <dd class="sheet__row-value text-label-field">{{ row.value }}</dd>
              </div>
            </dl>
          </section>

          <section class="sheet__card" aria-labelledby="sheet-team">
            <h3 id="sheet-team" class="sheet__card-title text-label-strong">Equipe e equipamento</h3>
            <dl class="sheet__rows">
              <div v-for="row in view.teamRows" :key="row.label" class="sheet__row">
                <dt class="sheet__row-label text-caption">{{ row.label }}</dt>
                <dd class="sheet__row-value text-label-field">{{ row.value }}</dd>
              </div>
            </dl>
          </section>

          <p v-if="view.signatureLabel" class="sheet__signature text-label-field">
            <AppIcon name="shield" :size="16" />
            {{ view.signatureLabel }}
          </p>
        </template>
      </div>

      <footer class="sheet__actions">
        <BaseButton variant="secondary" icon="download" :unavailable-hint="downloadHint">Baixar registro (PDF)</BaseButton>
        <BaseButton v-if="canRepeat" icon="plus" @click="emit('repeat')">Planejar repetição</BaseButton>
      </footer>
    </v-card>
  </v-dialog>
</template>

<style>
/* Vuetify teleports the dialog, so these rules cannot be scoped. Pinned to the right, full height. */
.application-sheet-overlay {
  justify-content: flex-end;
  align-items: stretch;
}

.application-sheet-overlay > .v-overlay__scrim {
  background: var(--color-overlay);
  opacity: 1;
}

.application-sheet-overlay > .v-overlay__content {
  width: min(var(--application-sheet-width), 100vw);
  max-width: 100vw;
  height: 100%;
  max-height: 100%;
  margin: 0;
}
</style>

<style scoped>
.sheet {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  height: 100%;
  padding: var(--space-28) var(--space-28) var(--space-24);
  border: 0;
  border-radius: var(--radius-2xl) 0 0 var(--radius-2xl);
  background: var(--color-sand);
  box-shadow: var(--shadow-modal);
}

.sheet :deep(> .v-card__overlay),
.sheet :deep(> .v-card__underlay) {
  display: none;
}

.sheet__header {
  display: flex;
  gap: var(--space-12);
  align-items: flex-start;
}

.sheet__heading {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.sheet__status {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
}

.sheet__registered,
.sheet__subtitle {
  color: var(--color-text-secondary);
}

.sheet__title,
.sheet__subtitle {
  margin: 0;
}

.sheet__title {
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.sheet__close {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border: 0;
  border-radius: var(--radius-tile);
  background: var(--color-white);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.sheet__close:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.sheet__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-16);
  min-height: 0;
  overflow-y: auto;
}

.sheet__state {
  flex: none;
}

.sheet__skeleton {
  flex: none;
  min-height: var(--application-skeleton-height);
  border-radius: var(--radius-lg);
  background: var(--color-white);
  animation: sheet-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

.sheet__conditions {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-16);
  border-radius: var(--radius-lg);
  background: var(--color-green-900);
  color: var(--color-text-inverse);
}

.sheet__conditions-header {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  justify-content: space-between;
}

.sheet__conditions-title {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
}

.sheet__verdict {
  color: var(--color-sprout-400);
}

.sheet__stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.sheet__stat {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-planner-track);
}

.sheet__stat-icon {
  color: var(--color-sprout-400);
}

.sheet__stat-label {
  color: var(--color-text-inverse-body);
}

.sheet__cancelled {
  display: flex;
  gap: var(--space-8);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-12) var(--space-16);
  border-radius: var(--radius-lg);
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}

.sheet__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  padding: var(--space-16);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-white);
}

.sheet__card-title {
  margin: 0;
  color: var(--color-text-primary);
}

.sheet__rows {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  margin: 0;
}

.sheet__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
  gap: var(--space-12);
}

.sheet__row dd,
.sheet__row dt {
  margin: 0;
  overflow-wrap: anywhere;
}

.sheet__row-label {
  color: var(--color-text-tertiary);
}

.sheet__row-value {
  color: var(--color-text-primary);
}

.sheet__signature {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  margin: 0;
  padding: var(--space-12) var(--space-16);
  border-radius: var(--radius-md);
  background: var(--color-status-suitable-bg);
  color: var(--color-status-suitable);
}

.sheet__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
}

.sheet__actions > * {
  flex: 1 1 auto;
}

@keyframes sheet-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (max-width: 599px) {
  .sheet {
    padding: var(--space-20) var(--space-16);
    border-radius: 0;
  }

  .sheet__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .sheet__row {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-2);
  }
}

@media (prefers-reduced-motion: reduce) {
  .sheet__skeleton {
    animation: none;
  }
}
</style>
