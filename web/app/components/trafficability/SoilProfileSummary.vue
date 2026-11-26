<script setup lang="ts">
import type { SoilProfile } from '~/types/trafficability'
import { SOIL_TEXTURE_LABELS } from '~/utils/trafficabilityLabels'

/**
 * Soil profile of the field in the "Chuva e solo" view (ADR 0022, phase 2):
 * texture and linked rain gauge, with "Editar perfil de solo" for
 * `registry.write`. Presentation only.
 */
withDefaults(defineProps<{
  /** Null while loading or when it failed. */
  profile: SoilProfile | null
  pending?: boolean
  canEdit?: boolean
}>(), { pending: false, canEdit: false })

const emit = defineEmits<{
  edit: []
}>()

const titleId = useId()
</script>

<template>
  <section class="soil-profile" :aria-labelledby="titleId" :aria-busy="pending || undefined">
    <div class="soil-profile__header">
      <h4 :id="titleId" class="soil-profile__title text-label-tag">Perfil de solo do talhão</h4>
      <BaseButton v-if="canEdit" variant="ghost" size="sm" icon="pen" @click="emit('edit')">
        Editar<span class="visually-hidden"> perfil de solo</span>
      </BaseButton>
    </div>
    <p v-if="!profile" class="soil-profile__muted text-caption">
      {{ pending ? 'Carregando perfil de solo…' : 'Perfil de solo indisponível.' }}
    </p>
    <dl v-else class="soil-profile__list text-caption">
      <div class="soil-profile__row">
        <dt>Textura cadastrada</dt>
        <dd>{{ profile.soilTexture ? SOIL_TEXTURE_LABELS[profile.soilTexture] : 'Não informada' }}</dd>
      </div>
      <div class="soil-profile__row">
        <dt>Pluviômetro</dt>
        <dd>{{ profile.rainGauge?.name ?? 'Nenhum (usa o modelo de previsão)' }}</dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.soil-profile {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  padding-top: var(--space-10);
  border-top: var(--border-width-hairline) solid var(--color-coffee-100);
}

.soil-profile__header {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  justify-content: space-between;
  min-height: var(--size-control-sm);
}

.soil-profile__title {
  margin: 0;
  color: var(--color-text-tertiary);
}

.soil-profile__muted {
  margin: 0;
  color: var(--color-text-secondary);
}

.soil-profile__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
}

.soil-profile__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-8);
}

.soil-profile__row dt {
  flex: 1 1 auto;
  color: var(--color-text-secondary);
  font-weight: 500;
}

.soil-profile__row dd {
  margin: 0;
  color: var(--color-text-primary);
  font-weight: 700;
  text-align: right;
}
</style>
