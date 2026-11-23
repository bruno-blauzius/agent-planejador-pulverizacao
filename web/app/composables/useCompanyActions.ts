import type { CompanyInput, CompanyUpdateInput } from '~/types/admin'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'

/**
 * User-triggered calls of the "Empresas" screen (create, edit, status change,
 * delete). Each resolves with its result or `null`, with the normalized error
 * (403, 409 `conflict`/`company_has_active_users`, 422 `details`, 404, 502) in
 * the matching `*Error`.
 */
export function useCompanyActions() {
  const { companies } = useApiServices()
  const create = useAction((input: CompanyInput) => companies.create(input))
  const update = useAction((companyId: string, input: CompanyUpdateInput) => companies.update(companyId, input))
  // `true` instead of `void`, so success and failure (`null`) are distinguishable.
  const remove = useAction(async (companyId: string) => {
    await companies.remove(companyId)
    return true as const
  })

  return {
    create: create.execute,
    creating: create.pending,
    createError: create.error,
    update: update.execute,
    updating: update.pending,
    updateError: update.error,
    remove: remove.execute,
    removing: remove.pending,
    removeError: remove.error,
  }
}
