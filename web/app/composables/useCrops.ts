import type { Crop } from '~/types/crop'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

/** Crop catalog for the field form. */
export function useCrops() {
  const { crops } = useApiServices()
  const resource = useResource<readonly Crop[]>('crops', () => crops.list(), () => [])
  return { crops: resource.data, pending: resource.pending, error: resource.error, refresh: resource.refresh }
}
