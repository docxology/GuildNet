import type {
  DeployImage,
  JobAccepted,
  JobSpec,
  LogLine,
  Server
} from './types'
import { apiUrl } from './config'

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const data = await res.json()
      if (data?.message) message = data.message
    } catch {}
    throw new Error(message)
  }
  return (await res.json()) as T
}

export async function listServers(signal?: AbortSignal): Promise<Server[]> {
  const res = await fetch(apiUrl('/api/servers'), { signal })
  return handle<Server[]>(res).catch(() => [])
}

// ---- Databases Feature (experimental) ----
export type DatabaseInstance = {
  id: string
  name: string
  description?: string
  created_at?: string
}
export type TableDef = {
  id: string
  name: string
  primary_key?: string
  schema?: ColumnDef[]
}
export type ColumnDef = {
  name: string
  type: string
  required?: boolean
  mask?: boolean
}

// Cluster-scoped database APIs
export async function listClusterDatabases(clusterId: string): Promise<DatabaseInstance[]> {
  try {
    const res = await fetch(apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/db`))
    if (!res.ok) return []
    return await res.json()
  } catch {
    return []
  }
}

export async function createClusterDatabase(
  clusterId: string,
  payload: { id: string; name?: string; description?: string }
): Promise<DatabaseInstance | null> {
  try {
    const res = await fetch(apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/db`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) return null
    return await res.json()
  } catch {
    return null
  }
}

export async function deleteClusterDatabase(clusterId: string, dbId: string): Promise<boolean> {
  try {
    const res = await fetch(apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/db/${encodeURIComponent(dbId)}`), { method: 'DELETE' })
    return res.ok
  } catch { return false }
}

export async function listClusterTables(clusterId: string, dbId: string): Promise<TableDef[]> {
  try {
    const res = await fetch(apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/db/${encodeURIComponent(dbId)}/tables`))
    if (!res.ok) return []
    return await res.json()
  } catch { return [] }
}

export async function createClusterTable(
  clusterId: string,
  dbId: string,
  payload: { name: string; primary_key?: string; schema: ColumnDef[] }
): Promise<TableDef | null> {
  try {
    const res = await fetch(apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/db/${encodeURIComponent(dbId)}/tables`), {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload)
    })
    if (!res.ok) return null
    return await res.json()
  } catch { return null }
}

export async function getServer(
  id: string,
  signal?: AbortSignal
): Promise<Server | null> {
  try {
    const res = await fetch(apiUrl(`/api/servers/${encodeURIComponent(id)}`), {
      signal
    })
    return await handle<Server>(res)
  } catch {
    return null
  }
}

export async function getLogs(
  id: string,
  params: {
    level?: 'info' | 'debug' | 'error'
    since?: string
    until?: string
    limit?: number
  },
  signal?: AbortSignal
): Promise<LogLine[]> {
  const qs = new URLSearchParams()
  if (params.level) qs.set('level', params.level)
  if (params.since) qs.set('since', params.since)
  if (params.until) qs.set('until', params.until)
  if (params.limit != null) qs.set('limit', String(params.limit))
  try {
    const res = await fetch(
      apiUrl(`/api/servers/${encodeURIComponent(id)}/logs?${qs.toString()}`),
      { signal }
    )
    return await handle<LogLine[]>(res)
  } catch {
    return []
  }
}

export async function postJob(
  payload: JobSpec,
  signal?: AbortSignal
): Promise<JobAccepted> {
  const res = await fetch(apiUrl('/api/jobs'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal
  })
  return handle<JobAccepted>(res)
}

export async function getImageDefaults(
  image: string,
  signal?: AbortSignal
): Promise<{
  ports?: { name?: string; port: number }[]
  env?: Record<string, string>
}> {
  if (!image) return {}
  const url = apiUrl(`/api/image-defaults?image=${encodeURIComponent(image)}`)
  const res = await fetch(url, { signal })
  try {
    return await handle<{
      ports?: { name?: string; port: number }[]
      env?: Record<string, string>
    }>(res)
  } catch {
    return {}
  }
}

export async function listImages(signal?: AbortSignal): Promise<DeployImage[]> {
  try {
    const res = await fetch(apiUrl('/api/images'), { signal })
    return await handle<DeployImage[]>(res)
  } catch {
    return []
  }
}

export type ClusterRecord = { id: string; name?: string; state?: string }

export async function listClusters(): Promise<ClusterRecord[]> {
  try {
    const res = await fetch(apiUrl('/api/deploy/clusters'))
    if (!res.ok) return []
    return (await res.json()) as ClusterRecord[]
  } catch {
    return []
  }
}

export async function getClusterRecord(id: string): Promise<ClusterRecord | null> {
  try {
    const res = await fetch(apiUrl(`/api/deploy/clusters/${encodeURIComponent(id)}`))
    if (!res.ok) return null
    return (await res.json()) as ClusterRecord
  } catch { return null }
}

export async function listClusterServers(clusterId: string): Promise<Server[]> {
  try {
    const res = await fetch(apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/servers`))
    if (!res.ok) return []
    return (await res.json()) as Server[]
  } catch {
    return []
  }
}

export async function createClusterWorkspace(
  clusterId: string,
  payload: any
): Promise<{ id: string; status: string } | null> {
  try {
    const res = await fetch(
      apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/workspaces`),
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      }
    )
    if (!res.ok) return null
    return (await res.json()) as { id: string; status: string }
  } catch {
    return null
  }
}

export async function clusterHealth(
  clusterId: string
): Promise<'ok' | 'error' | 'unknown'> {
  try {
    const res = await fetch(
      apiUrl(`/api/deploy/clusters/${encodeURIComponent(clusterId)}?action=health`),
      { method: 'POST' }
    )
    if (!res.ok) return 'unknown'
    const data = await res.json()
    return (data?.status as any) || 'unknown'
  } catch {
    return 'unknown'
  }
}

export async function createClusterRecord(
  name?: string
): Promise<{ id: string } | null> {
  try {
    const res = await fetch(apiUrl('/api/deploy/clusters'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    })
    if (!res.ok) return null
    return (await res.json()) as { id: string }
  } catch {
    return null
  }
}

export async function attachClusterKubeconfig(
  id: string,
  kubeconfig: string
): Promise<boolean> {
  try {
    const res = await fetch(
      apiUrl(`/api/deploy/clusters/${encodeURIComponent(id)}?action=attach-kubeconfig`),
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kubeconfig })
      }
    )
    return res.ok
  } catch {
    return false
  }
}

export async function getClusterKubeconfig(id: string): Promise<string | null> {
  try {
    const res = await fetch(
      apiUrl(`/api/deploy/clusters/${encodeURIComponent(id)}?action=kubeconfig`),
      { method: 'POST' }
    )
    if (!res.ok) return null
    return await res.text()
  } catch {
    return null
  }
}

export async function deleteClusterRecord(id: string): Promise<boolean> {
  try {
    const res = await fetch(apiUrl(`/api/deploy/clusters/${encodeURIComponent(id)}`), {
      method: 'DELETE'
    })
    return res.ok
  } catch {
    return false
  }
}

export async function postClusterAction<T = any>(
  id: string,
  action: string,
  body?: any
): Promise<T | null> {
  try {
    const url = apiUrl(
      `/api/deploy/clusters/${encodeURIComponent(id)}?action=${encodeURIComponent(action)}`
    )
    const init: RequestInit = { method: 'POST' }
    if (body !== undefined) {
      init.headers = { 'Content-Type': 'application/json' }
      init.body = JSON.stringify(body)
    }
    const res = await fetch(url, init)
    if (!res.ok) return null
    try {
      return (await res.json()) as T
    } catch {
      return null
    }
  } catch {
    return null
  }
}

export async function getHealthSummary(): Promise<{
  clusters: Array<{ id: string; status: 'ok' | 'error' | 'unknown' | string }>
  headscale: any[]
}> {
  try {
    const res = await fetch(apiUrl('/api/health'))
    if (!res.ok) return { clusters: [], headscale: [] }
    return (await res.json()) as any
  } catch {
    return { clusters: [], headscale: [] }
  }
}

export async function getClusterWorkspace(
  clusterId: string,
  name: string
): Promise<any | null> {
  try {
    const res = await fetch(
      apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/workspaces/${encodeURIComponent(name)}`)
    )
    if (!res.ok) return null
    return await res.json()
  } catch {
    return null
  }
}

export async function getClusterWorkspaceLogs(
  clusterId: string,
  name: string,
  limit = 200
): Promise<Array<{ t: string; msg: string }>> {
  try {
    const res = await fetch(
      apiUrl(`/api/cluster/${encodeURIComponent(clusterId)}/workspaces/${encodeURIComponent(name)}/logs?limit=${encodeURIComponent(String(limit))}`)
    )
    if (!res.ok) return []
    return (await res.json()) as Array<{ t: string; msg: string }>
  } catch {
    return []
  }
}
