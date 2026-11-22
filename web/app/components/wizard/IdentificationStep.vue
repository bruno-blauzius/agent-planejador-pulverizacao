<script setup lang="ts">
import type { ChoiceChipOption } from './ChoiceChips.vue'
import type { ProductionType } from '~/types/farm'

/**
 * Step 1 fields — Identificação (Figma 35:630): name, code, cooperative,
 * production type and the farm photo (not in the API yet: disabled, "Em breve").
 * Presentation only: values via v-model, errors and touch handled by the caller.
 */
export type IdentificationField = 'name' | 'code' | 'cooperative' | 'productionType'

withDefaults(
  defineProps<{
    errors?: Partial<Record<string, string>>
  }>(),
  { errors: () => ({}) },
)

const name = defineModel<string>('name', { required: true })
const code = defineModel<string>('code', { required: true })
const cooperative = defineModel<string>('cooperative', { required: true })
const productionType = defineModel<ProductionType | null>('productionType', { required: true })

const emit = defineEmits<{
  touch: [field: IdentificationField]
}>()

// Emojis are decorative presentation of the Figma chips (35:661).
const PRODUCTION_EMOJI: Readonly<Record<ProductionType, string>> = {
  coffee: '☕',
  grains: '🌱',
  livestock: '🐄',
  mixed: '🌾',
}

const productionOptions = computed<ChoiceChipOption<ProductionType>[]>(() =>
  (Object.keys(PRODUCTION_EMOJI) as ProductionType[]).map((value) => ({
    value,
    label: PRODUCTION_TYPE_LABELS[value],
    emoji: PRODUCTION_EMOJI[value],
  })),
)

function onProductionChange(value: ProductionType | null) {
  productionType.value = value
  emit('touch', 'productionType')
}
</script>

<template>
  <div class="identification-step">
    <TextField
      v-model="name"
      label="Nome da fazenda"
      name="name"
      placeholder="Fazenda Santa Rita"
      autocomplete="off"
      required
      :error="errors.name"
      @blur="emit('touch', 'name')"
    />

    <div class="identification-step__row">
      <TextField
        v-model="code"
        label="Apelido / código"
        name="code"
        placeholder="STR-01"
        hint="Aparece nas etiquetas do mapa"
        autocomplete="off"
        :error="errors.code"
        @blur="emit('touch', 'code')"
      />
      <TextField
        v-model="cooperative"
        label="Cooperativa"
        name="cooperative"
        placeholder="Cooxupé"
        autocomplete="organization"
        :error="errors.cooperative"
        @blur="emit('touch', 'cooperative')"
      />
    </div>

    <WizardChoiceChips
      :model-value="productionType"
      label="Tipo de produção"
      name="productionType"
      required
      :options="productionOptions"
      :error="errors.productionType"
      @update:model-value="onProductionChange"
    />

    <!-- No photo field/upload in openapi.yaml yet (contract tela-05, item 3). -->
    <FileDropzone
      layout="stacked"
      title="Foto ou logotipo da fazenda (opcional)"
      description="Em breve · PNG ou JPG até 5 MB"
      accept="image/png,image/jpeg"
      disabled
    />
  </div>
</template>

<style scoped>
.identification-step {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.identification-step__row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-16);
  align-items: start;
}

@media (max-width: 599px) {
  .identification-step__row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
