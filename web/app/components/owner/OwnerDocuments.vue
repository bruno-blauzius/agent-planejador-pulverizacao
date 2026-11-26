<script setup lang="ts">
import type { SelectItem } from '~/components/base/BaseSelect.vue'
import type { OwnerDocument, OwnerDocumentKind, PersonType } from '~/types/owner'
import {
  OWNER_DOCUMENT_ACCEPT,
  OWNER_DOCUMENT_KIND_LABELS,
  documentKindsFor,
  formatUploadedAt,
} from '~/utils/ownerDocuments'

/**
 * Documents of an owner in the edit form (ADR 0020; block of Figma 39:1942).
 * Lists the files, links each to its download and, with `registry.write`,
 * sends and removes them. Whether a required document is missing comes from
 * the API (`pending`); after every change the host reloads the owner.
 */
const props = defineProps<{
  ownerId: string
  personType: PersonType
  documents: readonly OwnerDocument[]
  pending: boolean
}>()

const emit = defineEmits<{
  changed: []
}>()

const { can } = usePermissions()
const { kind, file, uploading, removingId, errorMessage, canUpload, upload, remove, urlOf } = useOwnerDocuments({
  ownerId: () => props.ownerId,
  onChanged: () => emit('changed'),
})

const canEdit = computed(() => can('registry.write'))
const required = computed(() => (props.personType === 'individual' ? 'Documento com foto (RG ou CNH)' : 'Contrato social'))
const statusText = computed(() =>
  props.pending ? 'Pendente · sem ele, o proprietário não assina o registro de aplicações' : 'Em dia',
)
const kindItems = computed<SelectItem<OwnerDocumentKind>[]>(() =>
  documentKindsFor(props.personType).map(value => ({ value, label: OWNER_DOCUMENT_KIND_LABELS[value] })),
)

function onSelect(files: File[]) {
  file.value = files[0] ?? null
}
</script>

<template>
  <section class="owner-documents" aria-labelledby="owner-documents-title">
    <div class="owner-documents__status" :class="{ 'owner-documents__status--pending': pending }">
      <span class="owner-documents__icon" aria-hidden="true">
        <AppIcon :name="pending ? 'alert' : 'check'" :size="18" />
      </span>
      <div class="owner-documents__status-text">
        <h3 id="owner-documents-title" class="text-label-strong">{{ required }}</h3>
        <p class="owner-documents__muted text-body-sm">{{ statusText }}</p>
      </div>
    </div>

    <ul v-if="documents.length" class="owner-documents__list">
      <li v-for="document in documents" :key="document.id" class="owner-documents__item">
        <AppIcon name="document" :size="18" aria-hidden="true" />
        <div class="owner-documents__item-text">
          <a
            class="owner-documents__link text-label-field"
            :href="urlOf(document.id)"
            :download="document.fileName"
            rel="noopener"
          >{{ document.fileName }}</a>
          <span class="owner-documents__muted text-body-sm">
            {{ OWNER_DOCUMENT_KIND_LABELS[document.kind] }} · enviado em {{ formatUploadedAt(document.uploadedAt) }}
          </span>
        </div>
        <BaseButton
          v-if="canEdit"
          variant="ghost"
          size="sm"
          icon="trash"
          :loading="removingId === document.id"
          :disabled="removingId !== null"
          @click="remove(document.id)"
        >
          Remover<span class="visually-hidden"> {{ document.fileName }}</span>
        </BaseButton>
      </li>
    </ul>
    <p v-else class="owner-documents__muted text-body-sm">Nenhum documento enviado.</p>

    <div v-if="canEdit" class="owner-documents__upload">
      <BaseSelect v-model="kind" label="Tipo do documento" :items="kindItems" name="document_kind" />
      <FileDropzone
        title="Arquivo"
        description="PDF, JPEG ou PNG, até 10 MB"
        action-label="Escolher"
        :accept="OWNER_DOCUMENT_ACCEPT"
        :file-name="file?.name"
        :disabled="uploading"
        @select="onSelect"
      />
      <BaseButton variant="secondary" size="sm" icon="upload" :disabled="!canUpload" :loading="uploading" @click="upload()">
        Enviar documento
      </BaseButton>
    </div>

    <p v-if="errorMessage" class="owner-documents__error text-body-sm" role="alert">{{ errorMessage }}</p>
  </section>
</template>

<style scoped>
.owner-documents {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.owner-documents__status {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-14);
  border-radius: var(--radius-tile-lg);
  background: var(--color-sand);
}

.owner-documents__status--pending {
  background: var(--color-status-attention-bg);
}

.owner-documents__icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  flex-shrink: 0;
  border-radius: var(--radius-tile);
  background: var(--color-white);
  color: var(--color-text-secondary);
}

.owner-documents__status--pending .owner-documents__icon {
  color: var(--color-status-attention);
}

.owner-documents__status-text,
.owner-documents__item-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.owner-documents__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.owner-documents__item {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-12);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-md);
}

.owner-documents__link {
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.owner-documents__upload {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  align-items: flex-start;
}

.owner-documents__upload > :not(:last-child) {
  align-self: stretch;
}

.owner-documents__muted {
  color: var(--color-text-tertiary);
}

.owner-documents__error {
  color: var(--color-status-unsuitable);
}
</style>
