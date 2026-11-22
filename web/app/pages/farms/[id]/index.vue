<script setup lang="ts">
import type { MapFeature } from '~/components/map/MapView.vue'
import type { MapLegendItem } from '~/components/map/MapLegend.vue'
import type { TabItem } from '~/components/TabsNav.vue'
import type { FlashMessage } from '~/types/flash'
import { FARM_MAP_MODE_LABELS, type FarmDetailTab } from '~/utils/farmDetail'
import { fieldColor, STATUS_COLORS } from '~/theme/map-style'

/**
 * "Fazenda · Áreas e talhões" (Figma 15:56, content 15:126).
 * Presentation only: data, selection and map state come from useFarmDetailPage.
 */
const route = useRoute()
const config = useRuntimeConfig()

const {
  header,
  tab,
  tabs,
  summary,
  blocks,
  toggleBlock,
  selectedFieldId,
  selectField,
  popover,
  mapMode,
  mapColorBy,
  mapBasemap,
  mapFeatures,
  mapPerimeter,
  mapLabels,
  legend,
  newFieldTo,
  addFieldToBlock,
  newArea,
  pending,
  error,
  isNotFound,
  isEmpty,
  refresh,
  canWrite,
} = useFarmDetailPage(() => String(route.params.id))

useHead({ title: () => `${header.value.title || 'Fazenda'} · Agentes Agro` })

const flash = useFlash()
const toast = ref<FlashMessage | null>(null)

const isSatelliteAvailable = Boolean(config.public.mapSatelliteUrl)
const TAB_UNAVAILABLE_HINT = 'Em breve'
const listPanelRef = ref<{ focusField: (id: string) => void } | null>(null)
const isFirstLoad = computed(() => pending.value && !header.value.title)

const tabItems = computed<readonly TabItem<FarmDetailTab>[]>(() =>
  tabs.value.map((item) => ({
    value: item.value,
    label: item.label,
    count: item.count ?? undefined,
    disabled: item.disabled,
    hint: item.disabled ? TAB_UNAVAILABLE_HINT : undefined,
  })),
)

/** Closes the popover and returns focus to the field row that represents it. */
async function closePopover() {
  const id = selectedFieldId.value
  selectField(null)
  if (!id) return
  await nextTick()
  listPanelRef.value?.focusField(id)
}

// Escape closes the popover wherever the focus is (map canvas, list, body).
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && popover.value && !event.defaultPrevented) {
    event.preventDefault()
    void closePopover()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  // Back from "Novo/Editar talhão": the saved field comes selected via ?field=.
  toast.value = flash.consume()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

/** Label pills ("T1") joined to the features drawn on the map. */
const features = computed<readonly MapFeature[]>(() => {
  const labelById = new Map(mapLabels.value.map((item) => [item.id, item.label]))
  return mapFeatures.value.map((feature) => ({ ...feature, label: labelById.get(feature.id) }))
})

const legendItems = computed<readonly MapLegendItem[]>(() => {
  const current = legend.value
  if (current.kind === 'status') {
    return current.items.map((item) => ({ key: item.value, label: item.label, color: STATUS_COLORS[item.value] }))
  }
  return current.items.map((item) => ({ key: `${item.colorIndex}`, label: item.label, color: fieldColor(item.colorIndex) }))
})

const mapAlt = computed(() =>
  header.value.title ? `Mapa dos talhões da ${header.value.title}` : 'Mapa dos talhões da fazenda',
)
</script>

<template>
  <div class="farm-detail">
    <section v-if="isNotFound" class="farm-detail__not-found">
      <EmptyState
        icon="map"
        title="Fazenda não encontrada"
        description="Ela pode ter sido removida ou o endereço está incorreto."
      >
        <template #actions>
          <BaseButton to="/farms">Voltar para fazendas</BaseButton>
        </template>
      </EmptyState>
    </section>

    <template v-else>
      <PageHeader :title="isFirstLoad ? 'Carregando fazenda…' : header.title" :breadcrumbs="header.breadcrumbs">
        <template #badge>
          <StatusBadge v-if="header.condition" :tone="header.condition.value" :label="header.condition.label" />
        </template>
        <template v-if="canWrite" #actions>
          <BaseButton variant="secondary" icon="plus" :unavailable-hint="newArea.disabled ? newArea.hint : undefined">
            Nova área
          </BaseButton>
          <BaseButton icon="plus" :to="newFieldTo">Novo talhão</BaseButton>
        </template>
      </PageHeader>

      <TabsNav v-model="tab" :items="tabItems" label="Seções da fazenda" panel-id-prefix="farm-detail" />

      <ToastAlert
        v-if="error"
        tone="error"
        title="Não foi possível carregar a fazenda"
        :message="error.message"
        :dismissible="false"
      >
        <template #actions>
          <BaseButton variant="secondary" size="sm" icon="undo" @click="refresh()">Tentar novamente</BaseButton>
        </template>
      </ToastAlert>

      <div
        v-else
        id="farm-detail-fields"
        class="farm-detail__body"
        role="tabpanel"
        aria-labelledby="farm-detail-tab-fields"
      >
        <FieldListPanel
          ref="listPanelRef"
          class="farm-detail__list"
          :summary="summary"
          :blocks="blocks"
          :add-field="canWrite ? addFieldToBlock : null"
          :loading="pending && !blocks.length && !error"
          :empty="isEmpty"
          @select="selectField"
          @toggle="toggleBlock"
        />

        <MapView
          class="farm-detail__map"
          variant="farm"
          rounded
          height="var(--farm-detail-map-height)"
          :alt="mapAlt"
          :perimeter="mapPerimeter"
          :features="features"
          :color-by="mapColorBy"
          :basemap="mapBasemap"
          :selected-id="selectedFieldId"
          controls
          :draw-label="canWrite ? 'Desenhar novo talhão' : undefined"
          @select="selectField"
          @deselect="selectField(null)"
          @draw="navigateTo(newFieldTo)"
        >
          <template #toolbar>
            <MapLayerSwitch v-model="mapMode" :labels="FARM_MAP_MODE_LABELS" :satellite-available="isSatelliteAvailable" />
          </template>
          <template v-if="legendItems.length" #legend>
            <MapLegend :items="legendItems" />
          </template>
          <template v-if="popover" #popover>
            <FieldMapPopover :field="popover" :editable="canWrite" @close="closePopover" />
          </template>
        </MapView>
      </div>
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
.farm-detail {
  --farm-detail-map-height: var(--farm-map-view-height);

  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

/* Figma 15:182: 360 px list + map filling the rest, 20 px apart. */
.farm-detail__body {
  display: grid;
  grid-template-columns: var(--field-panel-width) minmax(0, 1fr);
  gap: var(--space-20);
  align-items: start;
}

/* Narrower screens: the Figma is 1440 px; the map goes above the list. */
@media (max-width: 1099px) {
  .farm-detail {
    --farm-detail-map-height: var(--farm-map-view-height-compact);
  }

  .farm-detail__body {
    grid-template-columns: minmax(0, 1fr);
  }

  .farm-detail__map {
    order: -1;
  }
}
</style>
