<script setup lang="ts">
import type { FarmTerrainTab } from '~/composables/useFarmTerrainTab'
// Explicit imports: auto-import would prefix these with the folder name.
import RainGaugeManagerModal from './RainGaugeManagerModal.vue'
import RainReadingModal from './RainReadingModal.vue'
import SoilInspectionModal from './SoilInspectionModal.vue'
import SoilProfileModal from './SoilProfileModal.vue'

/**
 * Dialogs of the "Chuva e solo" view (ADR 0022, phase 2): rain reading, soil
 * inspection, soil profile and the gauge registry. Container: wires the forms of
 * the Terreno tab to the presentation modals and keeps one dialog open at a time.
 * The gauge registry opened from another dialog returns to it when closed, with
 * the gauge just created selected. The forms are reset only when their dialog
 * opens from the view, never when coming back from the registry.
 * Saving reloads the trafficability on its own (the tab does it).
 */
export type FieldConditionsDialog = 'reading' | 'inspection' | 'profile' | 'gauges'

const props = withDefaults(defineProps<{
  tab: FarmTerrainTab
  fieldName?: string | null
}>(), { fieldName: null })

const dialog = defineModel<FieldConditionsDialog | null>('dialog', { default: null })

const { rainGauges, rainReadingForm, soilInspectionForm, soilProfileForm, canRecordConditions, canEditSoilProfile } = props.tab
const canManageGauges = rainGauges.canWrite

/** Dialog to reopen when the gauge registry closes. */
const returnTo = ref<FieldConditionsDialog | null>(null)
/** Last gauge created in the registry, selected in the dialog it returns to. */
const createdGaugeId = ref<string | null>(null)

// A fresh form on each opening from the view; coming back from the registry keeps the values.
watch(dialog, (next, previous) => {
  if (previous === 'gauges') return
  if (next === 'profile') soilProfileForm.open()
  else if (next === 'reading') rainReadingForm.reset()
})

function openGauges(from: FieldConditionsDialog) {
  returnTo.value = from
  createdGaugeId.value = null
  dialog.value = 'gauges'
}

function closeGauges() {
  const created = createdGaugeId.value
  if (created && returnTo.value === 'profile') soilProfileForm.selectGauge(created)
  else if (created && returnTo.value === 'reading') rainReadingForm.selectGauge(created)
  dialog.value = returnTo.value
  returnTo.value = null
  createdGaugeId.value = null
}

function close() {
  dialog.value = null
}
</script>

<template>
  <RainReadingModal
    v-if="dialog === 'reading' && canRecordConditions"
    :form="rainReadingForm"
    :gauges-pending="rainGauges.pending.value"
    :gauges-error="rainGauges.error.value?.message ?? null"
    :can-manage-gauges="canManageGauges"
    @close="close"
    @manage-gauges="openGauges('reading')"
  />
  <SoilInspectionModal
    v-else-if="dialog === 'inspection' && canRecordConditions"
    :form="soilInspectionForm"
    :field-name="fieldName"
    @close="close"
  />
  <SoilProfileModal
    v-else-if="dialog === 'profile' && canEditSoilProfile"
    :form="soilProfileForm"
    :field-name="fieldName"
    :can-manage-gauges="canManageGauges"
    @close="close"
    @manage-gauges="openGauges('profile')"
  />
  <RainGaugeManagerModal
    v-else-if="dialog === 'gauges' && canManageGauges"
    :gauges="rainGauges"
    @created="createdGaugeId = $event"
    @close="closeGauges"
  />
</template>
