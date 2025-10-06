// API/WS base can be overridden with VITE_API_BASE and VITE_WS_BASE.
// Examples:
//  VITE_API_BASE=https://localhost:8080
//  VITE_WS_BASE=ws://localhost:8080

const trimSlash = (s: string) => s.replace(/\/$/, '')

export const API_BASE = (() => {
  const b = import.meta.env.VITE_API_BASE
  if (b) return trimSlash(b)
  // Dev-friendly default: when running Vite locally and no override is set,
  // talk to the backend on 127.0.0.1:8080 (our default backend port).
  if (import.meta.env.DEV) return 'https://127.0.0.1:8080'
  return ''
})()

export function apiUrl(path: string) {
  return API_BASE ? `${API_BASE}${path}` : path
}

export const WS_BASE = (() => {
  const w = import.meta.env.VITE_WS_BASE
  if (w) return trimSlash(w)
  // In dev with no explicit override, match the default API_BASE above.
  if (import.meta.env.DEV) return 'wss://127.0.0.1:8080'
  if (API_BASE) return trimSlash(API_BASE.replace(/^http/, 'ws'))
  return ''
})()

export function wsUrl(path: string) {
  if (WS_BASE) return `${WS_BASE}${path}`
  return `${location.origin.replace(/^http/, 'ws')}${path}`
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
