import type { DeployImage, JobAccepted, JobSpec, LogLine, Server } from './types';
import { apiUrl } from './config';

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const data = await res.json();
      if (data?.message) message = data.message;
    } catch {}
    throw new Error(message);
  }
  return (await res.json()) as T;
}

export async function listServers(signal?: AbortSignal): Promise<Server[]> {
  const res = await fetch(apiUrl('/api/servers'), { signal });
  return handle<Server[]>(res).catch(() => []);
}

export async function getServer(id: string, signal?: AbortSignal): Promise<Server | null> {
  try {
  const res = await fetch(apiUrl(`/api/servers/${encodeURIComponent(id)}`), { signal });
    return await handle<Server>(res);
  } catch {
    return null;
  }
}

export async function getLogs(
  id: string,
  params: { level?: 'info' | 'debug' | 'error'; since?: string; until?: string; limit?: number },
  signal?: AbortSignal,
): Promise<LogLine[]> {
  const qs = new URLSearchParams();
  if (params.level) qs.set('level', params.level);
  if (params.since) qs.set('since', params.since);
  if (params.until) qs.set('until', params.until);
  if (params.limit != null) qs.set('limit', String(params.limit));
  try {
  const res = await fetch(apiUrl(`/api/servers/${encodeURIComponent(id)}/logs?${qs.toString()}`), { signal });
    return await handle<LogLine[]>(res);
  } catch {
    return [];
  }
}

export async function postJob(payload: JobSpec, signal?: AbortSignal): Promise<JobAccepted> {
  const res = await fetch(apiUrl('/api/jobs'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
  return handle<JobAccepted>(res);
}

export async function getImageDefaults(image: string, signal?: AbortSignal): Promise<{ ports?: {name?: string; port: number}[]; env?: Record<string,string> }> {
  if (!image) return {};
  const url = apiUrl(`/api/image-defaults?image=${encodeURIComponent(image)}`);
  const res = await fetch(url, { signal });
  try {
    return await handle<{ ports?: {name?: string; port: number}[]; env?: Record<string,string> }>(res);
  } catch {
    return {};
  }
}

export async function listImages(signal?: AbortSignal): Promise<DeployImage[]> {
  try {
    const res = await fetch(apiUrl('/api/images'), { signal });
    return await handle<DeployImage[]>(res);
  } catch {
    return [];
  }
}
