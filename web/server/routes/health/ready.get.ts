// Readiness does not depend on services/api to avoid cascading failures;
// see server/utils/health.ts.
export default defineEventHandler((event) => {
  setResponseHeader(event, 'Cache-Control', PROBE_CACHE_CONTROL)
  return probeStatus()
})
