/**
 * Presentation of owner documents (labels and messages). Which document an
 * owner must have is decided by the API (`has_pending_documents`).
 */
import type { ApiError } from '~/services/errors'
import type { OwnerDocumentKind, PersonType } from '~/types/owner'

export const OWNER_DOCUMENT_KIND_LABELS: Readonly<Record<OwnerDocumentKind, string>> = {
  id_card: 'RG ou documento de identidade',
  driver_license: 'CNH',
  articles_of_incorporation: 'Contrato social',
  other: 'Outro documento',
}

/** Kinds offered first for the person type; "other" is always the last option. */
export function documentKindsFor(personType: PersonType): readonly OwnerDocumentKind[] {
  return personType === 'individual'
    ? ['id_card', 'driver_license', 'other']
    : ['articles_of_incorporation', 'other']
}

/** Accepted by the API (checked by content there; this only filters the file picker). */
export const OWNER_DOCUMENT_ACCEPT = 'application/pdf,image/jpeg,image/png,.pdf,.jpg,.jpeg,.png'

/** One sentence for a failed upload or removal. */
export function ownerDocumentErrorMessage(error: ApiError): string {
  switch (error.code) {
    case 'payload_too_large':
      return 'O arquivo passa de 10 MB. Envie um arquivo menor.'
    case 'unsupported_media_type':
      return 'Formato não aceito. Envie PDF, JPEG ou PNG.'
    case 'not_found':
      return 'O documento não existe mais. Atualize a página.'
    case 'forbidden':
      return 'Você não tem permissão para alterar documentos.'
    case 'validation_failed':
      return error.details[0]?.message ?? 'Confira o tipo e o arquivo.'
    default:
      return error.message
  }
}

export function formatUploadedAt(date: Date): string {
  return new Intl.DateTimeFormat('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric' }).format(date)
}
