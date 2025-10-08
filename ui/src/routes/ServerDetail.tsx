import { Tabs } from '../components/Tabs'
import Card from '../components/Card'
import StatusPill from '../components/StatusPill'
import KeyValueList from '../components/KeyValueList'
import LogViewer from '../components/LogViewer'
import { useParams } from '@solidjs/router'
import {
  Show,
  createEffect,
  createMemo,
  createResource,
  createSignal,
  onCleanup
} from 'solid-js'
import { getServer } from '../lib/api'
import { formatDate } from '../lib/format'
import { apiUrl } from '../lib/config'

export default function ServerDetail() {
  const params = useParams()
  const [tab, setTab] = createSignal<'info' | 'debug' | 'error' | 'ide'>('info')
  const [srv] = createResource(
    () => params.id,
    (id: string) => getServer(id)
  )

  // Prefer direct URL (LB IP or ingress) when present; fallback to internal proxy
  const ideUrl = createMemo(() => {
    const s = srv()
    if (!s) return ''
    console.log(s)
    if (s.url) return s.url
    return apiUrl(`/proxy/server/${encodeURIComponent(s.id)}/`)
  })

  // Preflight: poll IDE URL until it responds, then set iframe src
  const [frameSrc, setFrameSrc] = createSignal<string | null>(null)
  const [ideChecking, setIdeChecking] = createSignal(false)
  const [ideError, setIdeError] = createSignal<string | null>(null)

  createEffect(() => {
    // Reset when switching tabs or server changes
    const url = ideUrl()
    const active = tab() === 'ide' && !!url
    if (!active) {
      setFrameSrc(null)
      setIdeChecking(false)
      setIdeError(null)
      return
    }

    let cancelled = false
    setIdeChecking(true)
    setIdeError(null)

    const controller = new AbortController()
    const start = Date.now()

    const rand = Math.random().toString(36).slice(2, 10)

    const check = async (attempt = 0) => {
      if (cancelled) return
      try {
        // Use GET to ensure we mimic the iframe navigation and avoid cache
        const res = await fetch(url, {
          method: 'GET',
          credentials: 'include',
          cache: 'no-store',
          headers: {
            Pragma: 'no-cache',
            'Cache-Control': 'no-cache',
            Accept: 'text/html'
          },
          signal: controller.signal
        })
        if (!res.ok) {
          // Log non-OK for diagnostics
          console.warn('IDE preflight non-OK', {
            url,
            status: res.status,
            statusText: res.statusText,
            attempt
          })
        }
        if (res.ok || (res.status >= 200 && res.status < 400)) {
          // Ready: set iframe src with a correlation id to bust caches
          const q = url.includes('?') ? '&' : '?'
          setFrameSrc(`${url}${q}xid=${rand}`)
          setIdeChecking(false)
          return
        }
        // Not ready yet; fall through to retry
      } catch (e) {
        // Network/abort errors: retry unless cancelled
        console.warn('IDE preflight error', { url, error: e, attempt })
        if (cancelled) return
      }
      const elapsed = Date.now() - start
      const backoff = Math.min(1500 + attempt * 250, 2500)
      if (elapsed > 15000) {
        setIdeError(
          'IDE is taking longer than expected to start. You can retry switching tabs or wait a moment.'
        )
        setIdeChecking(false)
        return
      }
      setTimeout(() => check(attempt + 1), backoff)
    }

    // Kick off checks
    check(0)

    onCleanup(() => {
      cancelled = true
      controller.abort()
    })
  })

  return (
    <div class="flex flex-col gap-4">
      <Show when={srv()} fallback={<div>Loading…</div>}>
        {(server) => (
          <>
            <Card
              title={server().name}
              actions={<StatusPill status={server().status} />}
            >
              <div class="grid sm:grid-cols-2 gap-4">
                <div>
                  <div class="text-sm">
                    <span class="text-neutral-500">Image:</span>{' '}
                    {server().image}
                  </div>
                  <div class="text-sm">
                    <span class="text-neutral-500">Node:</span>{' '}
                    {server().node ?? '-'}
                  </div>
                  <div class="text-sm">
                    <span class="text-neutral-500">Created:</span>{' '}
                    {formatDate(server().created_at)}
                  </div>
                  <div class="text-sm">
                    <span class="text-neutral-500">Updated:</span>{' '}
                    {formatDate(server().updated_at)}
                  </div>
                  <div class="text-sm">
                    <span class="text-neutral-500">Ports:</span>{' '}
                    {(server().ports ?? [])
                      .map((p) => `${p.name ?? ''}:${p.port}`)
                      .join(', ') || '-'}
                  </div>
                </div>
                <div>
                  <div class="text-sm">
                    <span class="text-neutral-500">Resources:</span>{' '}
                    {server()?.resources
                      ? `${server()?.resources?.cpu ?? ''} ${server()?.resources?.memory ?? ''}`
                      : '-'}
                  </div>
                  <div class="mt-2">
                    <div class="text-xs text-neutral-500">Args</div>
                    <div class="text-sm">
                      {(server().args ?? []).join(' ') || '—'}
                    </div>
                  </div>
                  <div class="mt-2">
                    <div class="text-xs text-neutral-500">Env</div>
                    <KeyValueList data={server().env ?? {}} />
                  </div>
                </div>
              </div>
            </Card>

            <Card title="Logs & Tools">
              <Tabs
                tabs={
                  [
                    { id: 'info', label: 'Info' },
                    { id: 'debug', label: 'Debug' },
                    { id: 'error', label: 'Error' },
                    ...(ideUrl() ? [{ id: 'ide', label: 'IDE' }] : [])
                  ] as { id: string; label: string }[]
                }
                value={tab()}
                onChange={(t) => setTab(t as any)}
              />
              {tab() === 'info' && (
                <LogViewer serverId={server().id} level="info" />
              )}
              {tab() === 'debug' && (
                <LogViewer serverId={server().id} level="debug" />
              )}
              {tab() === 'error' && (
                <LogViewer serverId={server().id} level="error" />
              )}
              {tab() === 'ide' && (
                <Show
                  when={ideUrl()}
                  fallback={
                    <div class="text-sm text-neutral-500">
                      IDE not available.
                    </div>
                  }
                >
                  {(url) => {
                    const [loaded, setLoaded] = createSignal(false)
                    return (
                      <div class="relative border rounded-md overflow-hidden h-[70vh]">
                        <iframe
                          src={frameSrc() || ''}
                          title="code-server"
                          class="w-full h-full bg-white"
                          referrerpolicy="no-referrer"
                          allow="clipboard-read; clipboard-write;"
                          onLoad={() => setLoaded(true)}
                        />
                        <Show when={!loaded() || ideChecking()}>
                          <div class="absolute inset-0 flex items-center justify-center bg-white/70 text-sm text-neutral-600">
                            {ideError() || 'Opening IDE…'}
                          </div>
                        </Show>
                      </div>
                    )
                  }}
                </Show>
              )}
            </Card>

            <Card title="Events">
              <div class="text-sm text-neutral-500">
                No events stream connected.
              </div>
            </Card>
          </>
        )}
      </Show>
    </div>
  )
}
