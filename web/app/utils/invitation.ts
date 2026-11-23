/**
 * Honest wording for the result of an invitation (create or resend), from the
 * `X-Invitation-Email` header (ADR 0015, D7). `not_sent` and `unknown` are
 * warnings, not success: the invitation exists and is valid, but nobody can
 * promise the person got an e-mail.
 */
import type { InvitationEmailStatus } from '~/types/admin'
import type { FlashMessage } from '~/types/flash'

export function invitationCreatedNotice(person: { name: string, email: string }, status: InvitationEmailStatus): FlashMessage {
  switch (status) {
    case 'sent':
      return {
        kind: 'success',
        title: 'Convite enviado',
        description: `${person.name} vai receber um e-mail em ${person.email} para criar a senha.`,
      }
    case 'not_needed':
      return {
        kind: 'success',
        title: 'Convite criado',
        description: `${person.name} já tem conta no login da Inceres e pode entrar sem criar senha.`,
      }
    case 'not_sent':
      return {
        kind: 'attention',
        title: 'Convite criado, mas o e-mail não foi enviado',
        description: `O convite de ${person.name} continua válido. Use "Reenviar convite" na lista para tentar de novo.`,
      }
    case 'unknown':
      return {
        kind: 'attention',
        title: 'Convite criado',
        description: `Não foi possível confirmar o envio do e-mail para ${person.email}. Se a pessoa não receber, use "Reenviar convite".`,
      }
  }
}

export function invitationResentNotice(email: string, status: InvitationEmailStatus): FlashMessage {
  switch (status) {
    case 'sent':
      return { kind: 'success', title: 'Convite reenviado', description: `Um novo e-mail foi enviado para ${email}.` }
    case 'not_needed':
      return { kind: 'success', title: 'Convite renovado', description: `${email} já tem conta no login da Inceres e pode entrar sem criar senha.` }
    case 'not_sent':
      return {
        kind: 'attention',
        title: 'Convite renovado, mas o e-mail não foi enviado',
        description: 'O convite continua válido. Tente reenviar de novo em instantes.',
      }
    case 'unknown':
      return {
        kind: 'attention',
        title: 'Convite renovado',
        description: `Não foi possível confirmar o envio do e-mail para ${email}.`,
      }
  }
}
