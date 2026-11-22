<script setup lang="ts">
import type { WizardOwnerRowView } from '~/utils/wizardOwners'

/**
 * Linked owner card of wizard step 3 (Figma 35:989; highlighted "nova" in
 * 41:1176): avatar, name and document, editable share and remove.
 */
const props = withDefaults(
  defineProps<{
    row: WizardOwnerRowView
    /** Extra description of the share input (e.g. the total error). */
    describedBy?: string
  }>(),
  { describedBy: undefined },
)

const emit = defineEmits<{
  updateShare: [text: string]
  blurShare: []
  remove: []
}>()

const shareId = useId()
const errorId = useId()

const shareDescribedBy = computed(() =>
  [props.row.error ? errorId : null, props.describedBy].filter(Boolean).join(' ') || undefined,
)

function onInput(event: Event) {
  emit('updateShare', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div
    class="owners-card"
    :class="{ 'owners-card--highlighted': row.isHighlighted, 'owners-card--error': row.error }"
  >
    <div class="owners-card__row">
      <BaseAvatar :seed="row.ownerId" :initials="row.initials" size="md" class="owners-card__avatar" />
      <div class="owners-card__text">
        <p class="owners-card__name text-label-strong">{{ row.name }}</p>
        <p class="owners-card__detail text-body-sm">{{ row.detail }}</p>
      </div>

      <div class="owners-card__share">
        <label :for="shareId" class="visually-hidden">Participação de {{ row.name }} (%)</label>
        <input
          :id="shareId"
          class="owners-card__share-input text-label-strong"
          :value="row.shareInput"
          type="text"
          inputmode="decimal"
          autocomplete="off"
          maxlength="6"
          :data-share-input="row.ownerId"
          :aria-invalid="row.error ? true : undefined"
          :aria-describedby="shareDescribedBy"
          @input="onInput"
          @blur="emit('blurShare')"
        >
        <span class="owners-card__share-unit text-label-strong" aria-hidden="true">%</span>
        <span class="owners-card__share-label text-body-sm" aria-hidden="true">participação</span>
      </div>

      <button
        type="button"
        class="owners-card__remove"
        :aria-label="`Remover ${row.name}`"
        :data-remove-owner="row.ownerId"
        @click="emit('remove')"
      >
        <AppIcon name="x" :size="16" />
      </button>
    </div>

    <p v-if="row.error" :id="errorId" class="owners-card__error text-body-sm">{{ row.error }}</p>
  </div>
</template>

<style scoped>
.owners-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  padding: var(--space-12);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
  background: var(--color-white);
}

/* Figma 41:1176: just registered → green tint and 2 px border (inset keeps the box size). */
.owners-card--highlighted {
  border-color: var(--color-green-700);
  background: var(--color-green-100);
  box-shadow: inset 0 0 0 var(--border-width-hairline) var(--color-green-700);
}

.owners-card--error {
  border-color: var(--color-status-unsuitable);
  box-shadow: none;
}

.owners-card__row {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.owners-card__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.owners-card__name,
.owners-card__detail {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.owners-card__name {
  color: var(--color-text-primary);
}

.owners-card__detail {
  color: var(--color-text-tertiary);
}

.owners-card__share {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  flex-shrink: 0;
  padding: var(--space-8) var(--space-12);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile);
  background: var(--color-white);
}

.owners-card__share:focus-within {
  border-color: var(--color-green-700);
  box-shadow: var(--focus-ring);
}

.owners-card--error .owners-card__share {
  border-color: var(--color-status-unsuitable);
}

.owners-card__share-input {
  width: 3.5ch;
  padding: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--color-text-primary);
  text-align: right;
}

.owners-card__share-unit {
  color: var(--color-text-primary);
}

.owners-card__share-label {
  margin-left: var(--space-4);
  color: var(--color-text-tertiary);
}

.owners-card__remove {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-sm);
  height: var(--size-icon-tile-sm);
  flex-shrink: 0;
  margin-right: calc(-1 * var(--space-4));
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}

.owners-card__remove:hover {
  background: var(--color-sand);
  color: var(--color-text-primary);
}

.owners-card__remove:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.owners-card__error {
  margin: 0;
  padding-left: calc(var(--size-avatar-md) + var(--space-12));
  color: var(--color-status-unsuitable);
}

@media (max-width: 599px) {
  .owners-card__row {
    flex-wrap: wrap;
  }

  .owners-card__text {
    flex-basis: calc(100% - var(--size-avatar-md) - var(--size-icon-tile-sm) - 2 * var(--space-12));
  }

  .owners-card__share {
    order: 4;
    margin-left: calc(var(--size-avatar-md) + var(--space-12));
  }

  .owners-card__error {
    padding-left: 0;
  }
}
</style>
