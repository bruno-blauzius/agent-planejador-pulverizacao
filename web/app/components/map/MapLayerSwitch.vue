<script setup lang="ts">
import type { MapColorBy } from '~/theme/map-style'
import type { SegmentOption } from '../SegmentedControl.vue'

/**
 * Layer switch of the farm map (Figma 15:295): color the fields by crop or by
 * spray status, or show the satellite imagery. Labels come from the caller
 * (e.g. FARM_MAP_MODE_LABELS). Satellite is disabled, with its reason, when no
 * imagery source is configured (NUXT_PUBLIC_MAP_SATELLITE_URL, ADR 0012).
 */
export type MapLayer = MapColorBy | 'satellite'

const props = withDefaults(
  defineProps<{
    labels: Readonly<Record<MapLayer, string>>
    satelliteAvailable?: boolean
  }>(),
  { satelliteAvailable: false },
)

const model = defineModel<MapLayer>({ required: true })

const LAYERS: readonly MapLayer[] = ['crop', 'status', 'satellite']
const SATELLITE_UNAVAILABLE = 'Imagem de satélite não configurada'

const options = computed<readonly SegmentOption<MapLayer>[]>(() =>
  LAYERS.map((value) => {
    const disabled = value === 'satellite' && !props.satelliteAvailable
    return { value, label: props.labels[value], disabled, hint: disabled ? SATELLITE_UNAVAILABLE : undefined }
  }),
)
</script>

<template>
  <SegmentedControl v-model="model" :options="options" label="Camada do mapa" variant="contrast" />
</template>
