import { computed } from 'vue'
import type { FarmInput } from '~/types/farm'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'

/**
 * Writes of the new-farm wizard and of the farm edit form. Each call resolves
 * with the result or `null`; failures land in `saveError` / `importError`
 * (`details` carries per-field messages for the form).
 */
export function useFarmActions() {
  const { farms } = useApiServices()

  const create = useAction((input: FarmInput) => farms.create(input))
  const update = useAction((farmId: string, input: FarmInput) => farms.update(farmId, input))
  const perimeter = useAction((file: File) => farms.importPerimeter(file))

  return {
    createFarm: create.execute,
    updateFarm: update.execute,
    saving: computed(() => create.pending.value || update.pending.value),
    saveError: computed(() => create.error.value ?? update.error.value),
    /** KML/KMZ/GeoJSON/Shapefile (.zip) → perimeter + area computed by the API. */
    importPerimeter: perimeter.execute,
    importing: perimeter.pending,
    importError: perimeter.error,
  }
}
