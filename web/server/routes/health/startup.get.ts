export default defineEventHandler((event) => {
  setResponseHeader(event, 'Cache-Control', PROBE_CACHE_CONTROL)
  return probeStatus()
})
