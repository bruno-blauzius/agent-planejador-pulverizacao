/**
 * Kubernetes probe payload, matching the Go API probes.
 *
 * The front has no dependencies of its own, so every probe reports "up" as
 * long as the Nitro server is serving requests. Readiness deliberately does
 * NOT check services/api: if the API is down, marking every front pod as not
 * ready would only cascade the failure (no pod receives traffic, users get a
 * load balancer error instead of a handled message from the front).
 */
export interface ProbeStatus {
  readonly status: 'up'
}

export const PROBE_CACHE_CONTROL = 'no-store'

export function probeStatus(): ProbeStatus {
  return { status: 'up' }
}
