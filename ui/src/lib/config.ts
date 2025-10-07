// API base can be overridden with VITE_API_BASE.
// Example:
//  VITE_API_BASE=https://localhost:8080

const trimSlash = (s: string) => s.replace(/\/$/, '')

export const API_BASE = (() => {
  const b = import.meta.env.VITE_API_BASE
  if (b) return trimSlash(b)
  // In dev, default to same-origin and rely on Vite proxy (no base prefix)
  if (import.meta.env.DEV) return ''
  return ''
})()

export function apiUrl(path: string) {
  return API_BASE ? `${API_BASE}${path}` : path
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
export const K8S_NS = (import.meta.env.VITE_K8S_NAMESPACE as string) || 'default'
// Optional: Cluster DNS suffix for Services
export const K8S_DNS_SUFFIX = (import.meta.env.VITE_K8S_DNS_SUFFIX as string) || 'svc.cluster.local'
