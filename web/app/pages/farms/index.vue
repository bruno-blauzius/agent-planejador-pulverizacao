<script setup lang="ts">
import type { FlashMessage } from '~/types/flash'
import type { FarmCardView } from '~/utils/farmCard'
import type { MapFeature } from '~/components/map/MapView.vue'
import type { SegmentOption } from '~/components/SegmentedControl.vue'
import type { DataTableHeader } from '~/components/DataTable.vue'
import type { FarmListView } from '~/composables/useFarmListPage'

/**
 * "Suas fazendas" (Figma 7:16; after saving: 35:1401 with toast 35:1728).
 * Presentation only: data, tabs and view come from useFarmListPage.
 */
useHead({ title: 'Fazendas · Agentes Agro' })

const { tab, view, tabs, cards, summaryCards, total, pending, error, isEmpty, isTabEmpty, refresh, canWrite } = useFarmListPage()
const { greeting } = useGreeting()
const flash = useFlash()

const toast = ref<FlashMessage | null>(null)

const VIEW_OPTIONS: readonly SegmentOption<FarmListView>[] = [
  { value: 'grid', label: 'Ver em grade', icon: 'grid' },
  { value: 'list', label: 'Ver em lista', icon: 'list' },
  { value: 'map', label: 'Ver no mapa', icon: 'map' },
]

const LIST_HEADERS: readonly DataTableHeader[] = [
  { title: 'Fazenda', key: 'name' },
  { title: 'Área', key: 'areaLabel' },
  { title: 'Talhões', key: 'fieldCountLabel' },
  { title: 'Culturas', key: 'cropLabel' },
  { title: 'Proprietários', key: 'owners' },
  { title: 'Condição', key: 'condition' },
  { title: '', key: 'open', align: 'end' },
]

const isFirstLoad = computed(() => pending.value && cards.value.length === 0)
const showsPartialList = computed(() => tab.value === 'all' && total.value > cards.value.length)

const mapFeatures = computed<MapFeature[]>(() =>
  cards.value.flatMap((card) =>
    card.perimeter ? [{ id: card.id, geometry: card.perimeter, status: card.condition?.value ?? null }] : [],
  ),
)

function openFarm(farmId: string) {
  const card = cards.value.find((item) => item.id === farmId)
  if (card) navigateTo(card.to)
}

function onRowClick(card: FarmCardView) {
  navigateTo(card.to)
}

onMounted(() => {
  toast.value = flash.consume()
})
</script>

<template>
  <div class="farms">
    <PageHeader :eyebrow="greeting" title="Suas fazendas">
      <template v-if="canWrite" #actions>
        <BaseButton variant="ghost" to="/farms/new?step=location&source=file">Importar KML</BaseButton>
        <BaseButton icon="plus" to="/farms/new">Nova fazenda</BaseButton>
      </template>
    </PageHeader>

    <section class="farms__summary" aria-label="Resumo">
      <StatCard
        v-for="card in summaryCards"
        :key="card.key"
        :label="card.label"
        :value="card.value"
        :caption="card.caption"
        :highlight="card.highlight"
        size="lg"
      />
    </section>

    <ToastAlert
      v-if="error"
      tone="error"
      title="Não foi possível carregar as fazendas"
      :message="error.message"
      :dismissible="false"
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" icon="undo" @click="refresh()">Tentar novamente</BaseButton>
      </template>
    </ToastAlert>

    <EmptyState
      v-else-if="isEmpty"
      icon="map"
      title="Nenhuma fazenda cadastrada"
      :description="canWrite
        ? 'Cadastre a primeira fazenda para acompanhar talhões e janelas de pulverização.'
        : 'As fazendas liberadas para você aparecem aqui. Se precisar de acesso a uma fazenda, fale com o administrador da sua empresa.'"
    >
      <template v-if="canWrite" #actions>
        <BaseButton icon="plus" to="/farms/new">Cadastrar primeira fazenda</BaseButton>
      </template>
    </EmptyState>

    <template v-else>
      <div class="farms__toolbar">
        <FilterChips v-model="tab" :items="tabs" label="Filtrar fazendas" />
        <SegmentedControl v-model="view" :options="VIEW_OPTIONS" label="Modo de visualização" variant="outlined" />
      </div>

      <div v-if="isFirstLoad" class="farms__grid" aria-busy="true" aria-label="Carregando fazendas">
        <div v-for="index in 4" :key="index" class="farms__skeleton">
          <div class="farms__skeleton-map" />
          <div class="farms__skeleton-line farms__skeleton-line--title" />
          <div class="farms__skeleton-line" />
        </div>
      </div>

      <p v-else-if="isTabEmpty" class="farms__tab-empty text-body-md">Nenhuma fazenda nesta categoria.</p>

      <div v-else-if="view === 'grid'" class="farms__grid">
        <FarmCard v-for="card in cards" :key="card.id" :farm="card" />
      </div>

      <DataTable
        v-else-if="view === 'list'"
        :headers="LIST_HEADERS"
        :items="cards"
        label="Fazendas"
        clickable
        @row-click="onRowClick"
      >
        <template #[`item.name`]="{ item }">
          <!-- Keyboard/middle-click path; stops the row click so navigation happens once. -->
          <NuxtLink :to="item.to" class="farms__table-name text-label-strong" @click.stop>{{ item.name }}</NuxtLink>
          <span class="farms__table-location text-caption-md">{{ item.locationLabel }}</span>
        </template>
        <template #[`item.owners`]="{ item }">
          <FarmOwnerAvatars :owners="item.owners" />
        </template>
        <template #[`item.condition`]="{ item }">
          <StatusBadge v-if="item.condition" :tone="item.condition.value" :label="item.condition.label" />
          <span v-else class="farms__table-muted text-caption-md">Sem previsão</span>
        </template>
        <template #[`item.open`]>
          <AppIcon name="chevron-right" :size="16" class="farms__table-chevron" />
        </template>
      </DataTable>

      <MapView
        v-else
        variant="farm"
        rounded
        height="var(--farm-map-view-height)"
        :features="mapFeatures"
        color-by="status"
        controls
        alt="Mapa com os perímetros das fazendas"
        @select="openFarm"
      />

      <p v-if="showsPartialList" class="farms__partial text-caption-md">
        Mostrando {{ cards.length }} de {{ total }} fazendas.
      </p>
    </template>

    <ToastAlert
      v-if="toast"
      floating
      :tone="toast.kind"
      :title="toast.title"
      :message="toast.description"
      @dismiss="toast = null"
    />
  </div>
</template>

<style scoped>
.farms {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
}

/* Figma 7:117: four equal cards, 16 px apart. */
.farms__summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-16);
}

.farms__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
  justify-content: space-between;
}

/* Figma 7:172: two columns, 20 px gutter. */
.farms__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-20);
}

.farms__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding-bottom: var(--space-22);
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.farms__skeleton-map {
  height: var(--farm-card-map-height);
  background: var(--color-sand);
}

.farms__skeleton-line {
  width: var(--skeleton-line-width);
  height: var(--space-14);
  margin: 0 var(--space-22);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
}

.farms__skeleton-line--title {
  width: 60%;
  height: var(--space-22);
}

.farms__skeleton,
.farms__skeleton * {
  animation: farms-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

@keyframes farms-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .farms__skeleton,
  .farms__skeleton * {
    animation: none;
  }
}

.farms__tab-empty,
.farms__partial {
  color: var(--color-text-secondary);
}


/* List mode: no Figma frame; DataTable with the owner table style (registered as a divergence). */
.farms__table-name {
  display: block;
  color: var(--color-text-primary);
  text-decoration: none;
}

.farms__table-name:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
  border-radius: var(--radius-xs);
}

.farms__table-location,
.farms__table-muted {
  color: var(--color-text-tertiary);
}

.farms__table-chevron {
  color: var(--color-green-800);
}
/* Narrower screens: the Figma is 1440 px; cards reflow instead of squeezing. */
@media (max-width: 1199px) {
  .farms__summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 959px) {
  .farms__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 599px) {
  .farms__summary {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
