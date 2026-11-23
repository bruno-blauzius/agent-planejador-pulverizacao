<script setup lang="ts">
import type { IconName } from '~/components/app/AppIcon.vue'
import type { BottomNavItem } from '~/components/planner/PlannerBottomNav.vue'

/**
 * Operador · Hoje (screen 18, Figma 20:18). Own mobile route with the `mobile`
 * layout. Presentation only: status, metrics, hour line and task come ready
 * from useTodayTask; whether it is suitable is computed by the API. Starting
 * an application only changes the status of a task that was already confirmed
 * by a human.
 *
 * Tabs: "Hoje" is this page; "Perfil" goes to /profile (open to every signed-in
 * role); "Mapa" and "Aplicações" have no screen yet ("Em breve"). The central
 * agent button opens /planner. The alerts bell has no endpoint yet.
 */
definePageMeta({ layout: 'mobile', permission: 'planner.use' })
useHead({ title: 'Hoje · Agentes Agro' })

const {
  greeting,
  locationLabel,
  status,
  metrics,
  hours,
  task,
  pending,
  error,
  isForbidden,
  isWeatherUnavailable,
  isEmpty,
  refresh,
  canStart,
  starting,
  startError,
  startConflict,
  startForbidden,
  start,
} = useTodayTask()
const { homePath, can } = usePermissions()

const UNAVAILABLE_HINT = 'Em breve'

const navItems: readonly BottomNavItem[] = [
  { key: 'today', label: 'Hoje', icon: 'home', to: '/today' },
  { key: 'map', label: 'Mapa', icon: 'map', unavailableHint: UNAVAILABLE_HINT },
  { key: 'applications', label: 'Aplicações', icon: 'calendar', to: '/applications' },
  { key: 'profile', label: 'Perfil', icon: 'user', to: '/profile' },
]

const METRIC_ICONS: Record<'delta_t' | 'wind' | 'rain', IconName> = {
  delta_t: 'thermometer',
  wind: 'wind',
  rain: 'drop',
}

const hourItems = computed(() =>
  hours.value.map(hour => ({ label: hour.label, condition: hour.condition, current: hour.isCurrent })),
)

const hasLoadFailure = computed(
  () => Boolean(error.value) && !isForbidden.value && !isWeatherUnavailable.value,
)

/** Confirmation read by screen readers after the task starts. */
const startedNotice = ref(false)
// A reloaded task is a new state: the announcement belongs to the previous one.
watch(task, () => {
  startedNotice.value = false
})

const taskActionLabel = computed(() => (task.value?.isStarted ? 'Aplicação em andamento' : 'Iniciar aplicação'))
/** A started task has its own label; otherwise a profile without the capability gets the reason. */
const taskActionHint = computed(() => {
  if (!task.value || task.value.isStarted || can('applications.register')) return undefined
  return 'Seu perfil não pode iniciar aplicações'
})

const startFailureMessage = computed(() => {
  if (startConflict.value) return 'Não foi possível iniciar: a aplicação já foi iniciada ou a janela fechou.'
  if (startForbidden.value) return 'Você não tem permissão para iniciar esta aplicação.'
  return 'Não foi possível iniciar a aplicação agora. Tente novamente.'
})

async function onStart() {
  startedNotice.value = await start()
}
</script>

<template>
  <div class="today">
    <div class="today__scroll" :aria-busy="pending || undefined">
      <header class="today__header">
        <div class="today__heading">
          <h1 class="today__greeting text-h3">{{ greeting }}</h1>
          <p v-if="locationLabel" class="today__location text-body-sm">
            <AppIcon name="pin" :size="13" />
            {{ locationLabel }}
          </p>
        </div>
        <button class="today__bell" type="button" aria-disabled="true" :title="UNAVAILABLE_HINT">
          <AppIcon name="bell" :size="20" />
          <span class="visually-hidden">Alertas ({{ UNAVAILABLE_HINT }})</span>
        </button>
      </header>

      <ForbiddenState v-if="isForbidden" :back-to="homePath" back-label="Voltar" />

      <template v-else>
        <ToastAlert
          v-if="isWeatherUnavailable"
          tone="attention"
          title="A previsão do tempo está indisponível"
          message="Não é possível dizer se pode pulverizar agora. Tente de novo em instantes."
          :dismissible="false"
        >
          <template #actions>
            <BaseButton size="sm" variant="secondary" @click="refresh()">Tentar novamente</BaseButton>
          </template>
        </ToastAlert>

        <ToastAlert
          v-else-if="hasLoadFailure"
          tone="error"
          title="Não foi possível carregar o dia"
          message="Verifique a conexão e tente novamente."
          :dismissible="false"
        >
          <template #actions>
            <BaseButton size="sm" variant="secondary" @click="refresh()">Tentar novamente</BaseButton>
          </template>
        </ToastAlert>

        <template v-else-if="pending && !status">
          <div class="today__skeleton today__skeleton--now" aria-hidden="true" />
          <div class="today__skeleton today__skeleton--tiles" aria-hidden="true" />
          <div class="today__skeleton today__skeleton--strip" aria-hidden="true" />
          <div class="today__skeleton today__skeleton--task" aria-hidden="true" />
          <p class="visually-hidden" role="status">Carregando o dia…</p>
        </template>

        <template v-else-if="status">
          <PlannerNowCard
            :eyebrow="status.nowLabel"
            :headline="status.headline"
            :until="status.untilLabel ?? undefined"
            :remaining="status.remainingLabel ?? undefined"
            :progress="status.progressRatio"
            :start-label="status.startLabel ?? undefined"
            :end-label="status.endLabel ?? undefined"
            :condition="status.condition"
          />

          <div class="today__metrics">
            <PlannerMetricTile
              v-for="metric in metrics"
              :key="metric.kind"
              :icon="METRIC_ICONS[metric.kind]"
              :value="metric.valueLabel"
              :label="metric.label"
              :condition="metric.condition"
            />
          </div>

          <PlannerHourStrip v-if="hourItems.length" title="Hoje, hora a hora" :hours="hourItems" />

          <ToastAlert
            v-if="startError"
            :tone="startConflict ? 'attention' : 'error'"
            :title="startFailureMessage"
            :dismissible="false"
          >
            <template v-if="startConflict" #actions>
              <BaseButton size="sm" variant="secondary" @click="refresh()">Atualizar</BaseButton>
            </template>
          </ToastAlert>
          <p v-if="startedNotice" class="visually-hidden" role="status">Aplicação iniciada.</p>

          <PlannerTaskCard
            v-if="task"
            :field-code="task.fieldLabel"
            :title="task.title"
            :detail="task.detail"
            :action-label="taskActionLabel"
            :action-disabled="task.isStarted || !canStart"
            :action-loading="starting"
            :action-hint="taskActionHint"
            @action="onStart"
          />
          <ToastAlert
            v-else-if="isEmpty"
            tone="info"
            title="Nenhuma aplicação planejada para hoje"
            message="Quando houver uma aplicação confirmada para você, ela aparece aqui."
            :dismissible="false"
          />
        </template>
      </template>
    </div>

    <PlannerBottomNav :items="navItems" active-key="today" agent-to="/planner" />
  </div>
</template>

<style scoped>
.today {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.today__scroll {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-22) var(--space-18) var(--space-16);
}

.today__header {
  display: flex;
  gap: var(--space-10);
  align-items: center;
}

.today__heading {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.today__greeting {
  margin: 0;
  color: var(--color-text-primary);
}

.today__location {
  display: flex;
  gap: var(--space-4);
  align-items: center;
  margin: 0;
  color: var(--color-text-secondary);
}

.today__bell {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-header-action);
  height: var(--size-planner-header-action);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
  background: var(--color-white);
  color: var(--color-text-primary);
  cursor: not-allowed;
  opacity: var(--opacity-skeleton-dim);
}

.today__bell:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.today__metrics {
  display: flex;
  gap: var(--space-10);
}

.today__skeleton {
  border-radius: var(--radius-card-xl);
  background: var(--color-border);
  animation: today-pulse var(--duration-skeleton) ease-in-out infinite;
}

.today__skeleton--now {
  height: var(--planner-skeleton-now-height);
  border-radius: var(--radius-2xl);
}

.today__skeleton--tiles {
  height: var(--planner-skeleton-tile-height);
}

.today__skeleton--strip {
  height: var(--planner-skeleton-strip-height);
}

.today__skeleton--task {
  height: var(--planner-skeleton-task-height);
}

@keyframes today-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .today__skeleton {
    animation: none;
  }
}
</style>
