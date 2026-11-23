import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { ApplicationDetail } from '~/types/application'
import { buildApplicationDetailView } from '~/utils/applications'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { usePermissions } from './usePermissions'
import { usePlannerContext } from './usePlannerContext'

export const PLANNER_PATH = '/planner'

/**
 * Side panel "Aplicação · detalhe" (screen 20). Loads when `applicationId` is
 * set; with `null` the panel is closed and nothing is requested. Out-of-scope or
 * other-company applications arrive as 403/404 and become honest states.
 */
export function useApplicationDetail(applicationId: MaybeRefOrGetter<string | null | undefined>) {
  const { applications } = useApiServices()
  const { can } = usePermissions()
  const plannerContext = usePlannerContext()

  const resource = useResource<ApplicationDetail | null>(
    () => `applications:detail:${toValue(applicationId) ?? 'none'}`,
    () => {
      const id = toValue(applicationId)
      return id ? applications.get(id) : Promise.resolve(null)
    },
    () => null,
  )
  const detail = computed(() => resource.data.value)
  const code = computed(() => resource.error.value?.code)

  /**
   * "Planejar repetição": puts this farm, field and product kind in the Planner
   * bar and returns where to go. The agent still proposes and the user still
   * confirms: nothing is registered from here.
   */
  function planRepeat(): string | null {
    const current = detail.value
    if (!current || !can('planner.use')) return null
    plannerContext.selectFarm(current.farmId)
    plannerContext.setFields([current.field.id])
    plannerContext.setProductKind(current.product.kind)
    return PLANNER_PATH
  }

  return {
    detail,
    view: computed(() => (detail.value ? buildApplicationDetailView(detail.value) : null)),
    pending: resource.pending,
    error: resource.error,
    isForbidden: computed(() => code.value === 'forbidden' || code.value === 'out_of_scope'),
    isNotFound: computed(() => code.value === 'not_found'),
    refresh: resource.refresh,

    /** Needs `planner.use` and a loaded application. */
    canRepeat: computed(() => detail.value !== null && can('planner.use')),
    planRepeat,
    /** "Baixar registro (PDF)" has no endpoint yet. */
    downloadAction: { disabled: true as const, hint: 'Em breve' },
  }
}
