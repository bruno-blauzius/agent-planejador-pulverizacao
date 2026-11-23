import type { InvitationInput, UserUpdateInput } from '~/types/admin'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'

/**
 * User-triggered calls of the "Usuários" screen and the invite modal. Each
 * resolves with its result or `null`, with the normalized error (403
 * `forbidden`/`out_of_scope` included) in the matching `*Error`.
 */
export function useUserActions() {
  const { users } = useApiServices()
  const invite = useAction((input: InvitationInput) => users.invite(input))
  const update = useAction((userId: string, input: UserUpdateInput) => users.update(userId, input))
  // `true` instead of `void`, so success and failure (`null`) are distinguishable.
  // Resolves whether the e-mail left (`X-Invitation-Email`), so the screen can warn honestly.
  const resend = useAction((invitationId: string) => users.resendInvitation(invitationId))
  const remove = useAction(async (userId: string) => {
    await users.remove(userId)
    return true as const
  })
  const cancel = useAction(async (invitationId: string) => {
    await users.cancelInvitation(invitationId)
    return true as const
  })

  return {
    invite: invite.execute,
    inviting: invite.pending,
    inviteError: invite.error,
    update: update.execute,
    updating: update.pending,
    updateError: update.error,
    resendInvitation: resend.execute,
    resendError: resend.error,
    remove: remove.execute,
    removeError: remove.error,
    cancelInvitation: cancel.execute,
    cancelError: cancel.error,
  }
}
