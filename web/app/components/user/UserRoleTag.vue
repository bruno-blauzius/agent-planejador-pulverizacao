<script setup lang="ts">
import type { TagTone } from '../base/BaseTag.vue'
import type { RoleTone } from '~/utils/userRow'

/** Role chip (Figma 52:416): the "Etiqueta" tone that the logic picked for the role. */
const props = defineProps<{
  label: string
  tone: RoleTone
}>()

const TAG_TONES: Readonly<Record<RoleTone, TagTone>> = { cafe: 'coffee', green: 'green', neutral: 'neutral' }
const tagTone = computed(() => TAG_TONES[props.tone])
</script>

<template>
  <BaseTag :label="label" :tone="tagTone" />
</template>
