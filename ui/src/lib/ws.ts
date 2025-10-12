import type { LogLine } from './types'
import { apiUrl } from './config'

type WSState = 'connecting' | 'open' | 'closed' | 'reconnecting' | 'error'

type Listener = (...args: any[]) => void

class Emitter {
  private map = new Map<string, Set<Listener>>()
  on(ev: string, fn: Listener) {
    if (!this.map.has(ev)) this.map.set(ev, new Set())
    this.map.get(ev)!.add(fn)
    return () => this.off(ev, fn)
  }
  off(ev: string, fn: Listener) {
    this.map.get(ev)?.delete(fn)
  }
  emit(ev: string, ...args: any[]) {
    this.map.get(ev)?.forEach((fn) => fn(...args))
  }
}

export class WSManager extends Emitter {
  private urlOrFn: string | (() => string)
  private es?: EventSource // SSE only
  private state: WSState = 'closed'
  private retries = 0
  private maxRetries = 10
  private heartbeatInterval = 15000
  private heartbeatTimer?: number
  private connectTimer?: number
  private lastPong = Date.now()
  private didProbe = false // single-shot status probe on error

  constructor(
    url: string | (() => string),
    opts?: { maxRetries?: number; heartbeatInterval?: number }
  ) {
    super()
    this.urlOrFn = url
    if (opts?.maxRetries != null) this.maxRetries = opts.maxRetries
    if (opts?.heartbeatInterval != null)
      this.heartbeatInterval = opts.heartbeatInterval
  }

  getState() {
    return this.state
  }

  private resolveUrl() {
    return typeof this.urlOrFn === 'function'
      ? (this.urlOrFn as () => string)()
      : this.urlOrFn
  }

  setUrl(url: string | (() => string)) {
    this.urlOrFn = url
  }

  open() {
    this.cleanup()
    this.state = this.retries > 0 ? 'reconnecting' : 'connecting'
    this.emit('state', this.state, this.retries)
    try {
      // Dev diagnostics
      try {
        console.info('[SSE] connecting', this.resolveUrl())
      } catch {}
      this.es = new EventSource(this.resolveUrl())
    } catch (e) {
      this.fail(e instanceof Error ? e.message : String(e))
      return
    }
    // Mark open on first message, since EventSource has no explicit onopen in all browsers
    const markOpenOnce = () => {
      if (this.state !== 'open') {
        this.state = 'open'
        this.retries = 0
        this.emit('state', this.state, this.retries)
      }
    }
    ;(this.es as EventSource).onopen = () => {
      try {
        console.info('[SSE] open', this.resolveUrl())
      } catch {}
      this.state = 'open'
      this.retries = 0
      this.emit('state', this.state, this.retries)
    }
    const onMessage = (text: string) => {
      markOpenOnce()
      try {
        const obj = JSON.parse(text)
        this.emit('message', obj)
      } catch {}
    }
    if (this.es)
      this.es.onmessage = (ev) => {
        if (typeof ev.data === 'string') onMessage(ev.data)
      }
    if (this.es)
      this.es.onerror = async (ev) => {
        try {
          console.error('[SSE] error event', {
            readyState: (this.es as EventSource).readyState,
            ev
          })
        } catch {}
        // Best-effort: try to fetch the same URL once to surface HTTP status/body when server rejects (e.g., 400/404/500)
        if (!this.didProbe) {
          this.didProbe = true
          try {
            const res = await fetch(this.resolveUrl(), {
              method: 'GET',
              headers: { Accept: 'application/json' }
            })
            const ct = res.headers.get('content-type') || ''
            let body: any = undefined
            if (ct.includes('application/json')) {
              try {
                body = await res.json()
              } catch {}
            } else {
              try {
                body = await res.text()
              } catch {}
            }
            console.error('[SSE] probe status', {
              status: res.status,
              statusText: res.statusText,
              body
            })
            this.emit('error', {
              status: res.status,
              statusText: res.statusText,
              body
            })
          } catch (e) {
            console.error('[SSE] probe failed', e)
            this.emit('error', {
              probeFailed: true,
              message: e instanceof Error ? e.message : String(e)
            })
          }
        }
        this.fail('sse error')
      }
  }

  private fail(reason: string) {
    this.cleanup()
    if (this.state === 'closed') return
    this.state = 'error'
    this.emit('state', this.state, this.retries, reason)
    if (this.retries >= this.maxRetries) {
      this.state = 'closed'
      this.emit('state', this.state, this.retries, 'max_retries')
      return
    }
    const backoff =
      Math.min(30000, 2 ** this.retries * 500) * (0.8 + Math.random() * 0.4)
    this.retries++
    this.connectTimer = window.setTimeout(() => this.open(), backoff)
    this.emit('backoff', backoff, this.retries)
  }

  private startHeartbeat() {
    // No-op for SSE (kept for API compatibility). The server sends a heartbeat comment.
    this.stopHeartbeat()
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) window.clearInterval(this.heartbeatTimer)
    this.heartbeatTimer = undefined
  }

  close() {
    this.cleanup()
    this.state = 'closed'
    try {
      this.es?.close()
    } catch {}
    this.emit('state', this.state, this.retries)
  }

  private cleanup() {
    if (this.connectTimer) window.clearTimeout(this.connectTimer)
    this.connectTimer = undefined
    this.es?.close?.()
    this.es = undefined
  }
}

export function openLogsStream(params: {
  target: string
  level: 'info' | 'debug' | 'error'
  tail?: number
}) {
  const qs = new URLSearchParams({
    target: params.target,
    level: params.level,
    tail: String(params.tail ?? 200)
  })
  const url = apiUrl(`/sse/logs?${qs.toString()}`)
  const ws = new WSManager(url)
  return ws
}

export function ringBuffer<T>(capacity: number) {
  let items: T[] = []
  return {
    push(item: T) {
      items.push(item)
      if (items.length > capacity) items.shift()
    },
    pushMany(arr: T[]) {
      for (const it of arr) this.push(it)
    },
    clear() {
      items = []
    },
    get() {
      return items
    },
    size() {
      return items.length
    }
  }
}
