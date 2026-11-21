import { describe, expect, it } from 'vitest'
import { PROBE_CACHE_CONTROL, probeStatus } from '../../server/utils/health'

describe('probeStatus', () => {
  it('reports the server as up', () => {
    expect(probeStatus()).toEqual({ status: 'up' })
  })

  it('disables caching of probe responses', () => {
    expect(PROBE_CACHE_CONTROL).toBe('no-store')
  })
})
