import {
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
  onMount
} from 'solid-js'
import { escapeHtml } from '../lib/format'
import { ringBuffer, openLogsStream, WSManager } from '../lib/ws'
import StreamStatus from './StreamStatus'

type Level = 'info' | 'debug' | 'error'

export default function LogViewer(props: {
  serverId: string
  level: Level
  tail?: number
  capacity?: number
}) {
  const capacity = props.capacity ?? 5000
  const buffer = ringBuffer<string>(capacity)
  const [lines, setLines] = createSignal<string[]>([])
  const [paused, setPaused] = createSignal(false)
  const [state, setState] = createSignal<
    'connecting' | 'open' | 'closed' | 'reconnecting' | 'error'
  >('connecting')
  const [retry, setRetry] = createSignal(0)
  const [backoff, setBackoff] = createSignal<number | undefined>()
  const [error, setError] = createSignal<string | undefined>()
  let ws: WSManager | undefined
  let scroller!: HTMLDivElement

  const autoscroll = createMemo(() => !paused())

  const connect = () => {
    ws?.close()
    ws = openLogsStream({
      target: props.serverId,
      level: props.level,
      tail: props.tail ?? 200
    })
    const off1 = ws.on('state', (s: any, r?: number, err?: string) => {
      setState(s)
      setRetry(r ?? 0)
      setError(err)
      if (s !== 'reconnecting') setBackoff(undefined)
    })
    const off2 = ws.on('backoff', (ms: number, r?: number) => {
      setBackoff(ms)
      setRetry(r ?? 0)
    })
    const off3 = ws.on('message', (obj: any) => {
      if (paused()) return // backpressure via pause
      const msg = typeof obj?.msg === 'string' ? obj.msg : JSON.stringify(obj)
      buffer.push(`${obj?.t ?? ''} ${obj?.lvl ?? ''} ${msg}`.trim())
      setLines([...buffer.get()])
      if (autoscroll()) scroller.scrollTop = scroller.scrollHeight
    })
    onCleanup(() => {
      off1()
      off2()
      off3()
      ws?.close()
    })
    ws.open()
  }

  onMount(() => {
    connect()
  })
  createEffect(() => {
    /* reconnect when level changes */ props.level
    connect()
  })

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(lines().join('\n'))
    } catch {}
  }
  const download = () => {
    const blob = new Blob([lines().join('\n')], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${props.serverId}-${props.level}-logs.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const full = () => buffer.size() >= capacity

  return (
    <div class="flex flex-col gap-2">
      <div class="flex items-center gap-2">
        <button
          class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700"
          onClick={() => setPaused((p) => !p)}
        >
          {paused() ? 'Resume' : 'Pause'}
        </button>
        <button
          class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700"
          onClick={copy}
        >
          Copy
        </button>
        <button
          class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700"
          onClick={download}
        >
          Download
        </button>
        {full() && <span class="text-xs text-amber-600">buffer full</span>}
        <div class="ml-auto">
          <StreamStatus
            state={state()}
            retry={retry()}
            backoffMs={backoff()}
            error={error()}
          />
        </div>
      </div>
      <div
        ref={scroller!}
        class="h-80 overflow-auto rounded border bg-neutral-950 text-neutral-100 font-mono text-xs p-2"
        aria-live="polite"
        aria-atomic="false"
      >
        {lines().length === 0 ? (
          <div class="text-neutral-400">No logs yet…</div>
        ) : (
          <div
            innerHTML={lines()
              .map((l) => `<div>${escapeHtml(l)}</div>`)
              .join('')}
          />
        )}
      </div>
    </div>
  )
}
