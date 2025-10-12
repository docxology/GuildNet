// API base can be overridden with VITE_API_BASE.
// Example:
//  VITE_API_BASE=https://127.0.0.1:8090

const trimSlash = (s: string) => s.replace(/\/$/, '')

// Return the current API base, considering build-time and runtime overrides.
export function getApiBase(): string {
  // Prefer explicit build-time override
  const envBase = import.meta.env.VITE_API_BASE as string | undefined
  if (envBase) return trimSlash(envBase)
  // Allow a runtime override set by the join-file import flow
  let runBase = ''
  if (typeof window !== 'undefined') {
    try {
      runBase = sessionStorage.getItem('GN_VITE_API_BASE') || ''
    } catch {
      runBase = ''
    }
  }
  return runBase ? trimSlash(runBase) : ''
}

export function apiUrl(path: string) {
  const base = getApiBase()
  return base ? `${base}${path}` : path
}

export async function fetchUiConfig() {
  try {
    const res = await fetch(apiUrl('/api/ui-config'))
    if (!res.ok) return {} as Record<string, unknown>
    return (await res.json()) as Record<string, unknown>
  } catch {
    return {} as Record<string, unknown>
  }
}

// Optional: Kubernetes namespace to construct default Service FQDNs
export const K8S_NS =
  (import.meta.env.VITE_K8S_NAMESPACE as string) || 'default'
// Optional: Cluster DNS suffix for Services
export const K8S_DNS_SUFFIX =
  (import.meta.env.VITE_K8S_DNS_SUFFIX as string) || 'svc.cluster.local'
