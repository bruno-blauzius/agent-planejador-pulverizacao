import { computed, ref, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import type { OwnerDocument, OwnerDocumentKind } from '~/types/owner'
import { ownerDocumentErrorMessage } from '~/utils/ownerDocuments'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'

export interface OwnerDocumentsOptions {
  readonly ownerId: MaybeRefOrGetter<string>
  /** Reloads the owner after a change: documents and the pending flag come from the API. */
  readonly onChanged: () => Promise<void> | void
}

export interface OwnerDocuments {
  readonly kind: Ref<OwnerDocumentKind | null>
  readonly file: Ref<File | null>
  readonly uploading: Readonly<Ref<boolean>>
  readonly removingId: Readonly<Ref<string | null>>
  /** Message of the last failed upload or removal; null otherwise. */
  readonly errorMessage: ComputedRef<string | null>
  readonly canUpload: ComputedRef<boolean>
  readonly upload: () => Promise<OwnerDocument | null>
  readonly remove: (documentId: string) => Promise<boolean>
  readonly urlOf: (documentId: string) => string
}

/** Upload, download link and removal of an owner's documents (ADR 0020). */
export function useOwnerDocuments(options: OwnerDocumentsOptions): OwnerDocuments {
  const { owners } = useApiServices()
  const kind = ref<OwnerDocumentKind | null>(null)
  const file = ref<File | null>(null)
  const removingId = ref<string | null>(null)

  const uploadAction = useAction((ownerId: string, chosen: OwnerDocumentKind, selected: File) =>
    owners.uploadDocument(ownerId, { kind: chosen, file: selected, fileName: selected.name }))
  const removeAction = useAction((ownerId: string, documentId: string) => owners.removeDocument(ownerId, documentId))
  const lastError = ref<'upload' | 'remove' | null>(null)

  async function upload(): Promise<OwnerDocument | null> {
    if (!kind.value || !file.value) return null
    lastError.value = null
    const document = await uploadAction.execute(toValue(options.ownerId), kind.value, file.value)
    if (!document) {
      lastError.value = 'upload'
      return null
    }
    file.value = null
    kind.value = null
    await options.onChanged()
    return document
  }

  async function remove(documentId: string): Promise<boolean> {
    lastError.value = null
    removingId.value = documentId
    try {
      const result = await removeAction.execute(toValue(options.ownerId), documentId)
      // removeDocument resolves with undefined on success; useAction gives null on failure.
      if (result === null) {
        lastError.value = 'remove'
        return false
      }
      await options.onChanged()
      return true
    }
    finally {
      removingId.value = null
    }
  }

  return {
    kind,
    file,
    uploading: uploadAction.pending,
    removingId,
    errorMessage: computed(() => {
      const error = lastError.value === 'upload' ? uploadAction.error.value : lastError.value === 'remove' ? removeAction.error.value : null
      return error ? ownerDocumentErrorMessage(error) : null
    }),
    canUpload: computed(() => Boolean(kind.value && file.value) && !uploadAction.pending.value),
    upload,
    remove,
    urlOf: documentId => owners.documentUrl(toValue(options.ownerId), documentId),
  }
}
