<script setup lang="ts">
import type { SelectItem } from '~/components/base/BaseSelect.vue'

/**
 * Painel (screen 15, Figma 54:535 / 54:602). Gated by capability (ADR 0013);
 * the route middleware applies `permission`. Presentation only: indicators,
 * chart, windows, alerts and activity come from useDashboardPage. The planner
 * (page 02) is out of scope, so its links are shown as unavailable.
 */
definePageMeta({ permission: 'dashboard.view' })
useHead({ title: 'Painel · Agentes Agro' })

const {
  greeting,
  dateLine,
  farmOptions,
  selectedFarmId,
  canOpenPlanner,
  pending,
  error,
  isForbidden,
  isOutOfScope,
  isLoaded,
  showAllFarms,
  refresh,
  statCards,
  chart,
  upcomingWindows,
  alerts,
  dismissAlert,
  activity,
} = useDashboardPage()

const PLANNER_HINT = 'Em breve'
/** BaseSelect works with strings: "Todas" (null farm) gets a sentinel value. */
const ALL_FARMS = 'all'

const farmItems = computed<SelectItem[]>(() =>
  farmOptions.value.map((option) => ({ value: option.value ?? ALL_FARMS, label: option.label })),
)

const farmSelection = computed<string | null>({
  // A farm that is not among the options (invalid or out of scope) shows no selection.
  get: () => {
    const value = selectedFarmId.value ?? ALL_FARMS
    return farmItems.value.some((item) => item.value === value) ? value : null
  },
  set: (value) => {
    selectedFarmId.value = !value || value === ALL_FARMS ? null : value
  },
})

/** Blocks get "loading" until the data arrives and "failed" when it did not. */
const isBlockPending = computed(() => pending.value && !isLoaded.value)
const isBlockFailed = computed(() => !pending.value && !isLoaded.value)
</script>

<template>
  <div class="dashboard">
    <ForbiddenState v-if="isForbidden" />

    <template v-else>
      <header class="dashboard__header">
        <div class="dashboard__heading">
          <p class="dashboard__date text-label-field">{{ dateLine }}</p>
          <h1 class="dashboard__title text-page-title">{{ greeting }}</h1>
        </div>
        <BaseSelect v-model="farmSelection" class="dashboard__farm" label="Fazenda" :items="farmItems" name="farm" />
        <BaseButton v-if="canOpenPlanner" variant="accent" icon="plus" :unavailable-hint="PLANNER_HINT">
          Abrir o Planejador
        </BaseButton>
      </header>

      <ToastAlert
        v-if="isOutOfScope"
        tone="attention"
        title="Fazenda indisponível"
        message="Essa fazenda não existe ou não está liberada para você, então o painel não foi carregado."
        :dismissible="false"
      >
        <template #actions>
          <BaseButton variant="secondary" size="sm" @click="showAllFarms()">Ver todas as fazendas</BaseButton>
        </template>
      </ToastAlert>
      <ToastAlert
        v-else-if="error"
        tone="error"
        title="Não foi possível carregar o painel"
        :message="error.message"
        :dismissible="false"
      >
        <template #actions>
          <BaseButton variant="secondary" size="sm" icon="undo" @click="refresh()">Tentar novamente</BaseButton>
        </template>
      </ToastAlert>

      <section class="dashboard__stats" aria-label="Indicadores" :aria-busy="isBlockPending || undefined">
        <StatCard
          v-for="card in statCards"
          :key="card.key"
          :label="card.label"
          :value="card.value"
          :caption="card.caption"
          :highlight="card.highlight"
        />
      </section>

      <div class="dashboard__row">
        <DashboardWindowsChart :chart="chart" :pending="isBlockPending" :failed="isBlockFailed" />
        <DashboardUpcomingWindows
          class="dashboard__side"
          :windows="upcomingWindows"
          :show-see-all="canOpenPlanner"
          :see-all-unavailable-hint="PLANNER_HINT"
          :pending="isBlockPending"
          :failed="isBlockFailed"
        />
      </div>

      <div class="dashboard__row">
        <DashboardAgentAlerts :alerts="alerts" :pending="isBlockPending" :failed="isBlockFailed" @dismiss="dismissAlert" />
        <DashboardActivity class="dashboard__side" :activity="activity" :pending="isBlockPending" :failed="isBlockFailed" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.dashboard__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-16);
  align-items: flex-end;
}

.dashboard__heading {
  display: flex;
  flex: 1 1 var(--page-header-main-min-width);
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.dashboard__date {
  /* Keeps the line height while the date is only known on the client. */
  min-height: 1lh;
  margin: 0;
  color: var(--color-coffee-600);
}

.dashboard__title {
  margin: 0;
  color: var(--color-text-primary);
}

.dashboard__farm {
  width: var(--dashboard-farm-select-width);
}

/* Figma 54:634: four equal indicator cards, 16 px apart. */
.dashboard__stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-16);
}

.dashboard__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) var(--dashboard-side-width);
  gap: var(--space-20);
  align-items: start;
}

@media (max-width: 1199px) {
  .dashboard__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .dashboard__row {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 599px) {
  .dashboard__stats {
    grid-template-columns: minmax(0, 1fr);
  }

  .dashboard__farm {
    width: 100%;
  }
}
</style>
