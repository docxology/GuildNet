import { createSignal, createEffect, onCleanup, For, Show } from 'solid-js'
import { apiUrl } from '../lib/config'
import StreamStatus from './StreamStatus'

type Row = Record<string, any>
type PageResp = { items: Row[]; next_cursor?: string }

interface GridProps {
  dbId: string
  table: string
}

const ROW_HEIGHT = 28

export default function DataGrid(props: GridProps) {
  const [rows, setRows] = createSignal<Row[]>([])
  const [nextCursor, setNextCursor] = createSignal<string | undefined>()
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<string | undefined>()
  const [live, setLive] = createSignal(true)
  const [pendingEvents, setPendingEvents] = createSignal(0)
  const [filters, setFilters] = createSignal<Record<string, string>>({})
  const [sort, setSort] = createSignal<{
    col: string
    dir: 'asc' | 'desc'
  } | null>(null)
  const [selection, setSelection] = createSignal<{
    row: number
    col: string
  } | null>(null)
  const [editing, setEditing] = createSignal<{
    row: number
    col: string
    value: string
  } | null>(null)
  const [streamState, setStreamState] = createSignal<
    'open' | 'reconnecting' | 'error'
  >('open')
  let container!: HTMLDivElement
  let es: EventSource | undefined
  let lastToken: string | undefined

  async function fetchPage(cursor?: string) {
    if (loading()) return
    setLoading(true)
    try {
      const qs = new URLSearchParams()
      if (cursor) qs.set('cursor', cursor)
      const res = await fetch(
        apiUrl(
          `/api/db/${encodeURIComponent(props.dbId)}/tables/${encodeURIComponent(props.table)}/rows?${qs.toString()}`
        )
      )
      if (!res.ok) throw new Error(`${res.status}`)
      const data: PageResp = await res.json()
      setRows((r) => [...r, ...data.items])
      setNextCursor(data.next_cursor || undefined)
    } catch (e: any) {
      setError(e.message || 'load failed')
    } finally {
      setLoading(false)
    }
  }

  function streamUrl() {
    const qs = new URLSearchParams()
    if (!live()) qs.set('pause', '1')
    if (lastToken) qs.set('cursor', lastToken)
    return apiUrl(
      `/sse/db/${encodeURIComponent(props.dbId)}/tables/${encodeURIComponent(props.table)}/changes?${qs.toString()}`
    )
  }

  function openStream() {
    es = new EventSource(streamUrl())
    es.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data)
        if (!data || !data.type) return
        if (data.type === 'paused' && typeof data.pending === 'number') {
          setPendingEvents(data.pending)
          return
        }
        if (['insert', 'update', 'delete'].includes(data.type)) {
          applyChange(data)
        }
        setStreamState('open')
      } catch {}
    }
    es.onerror = () => {
      setStreamState('reconnecting')
    }
  }

  function applyChange(ev: any) {
    const pkCol = inferPK()
    const id = ev.after?.[pkCol] ?? ev.before?.[pkCol]
    if (!id) return
    // simple resume token heuristic
    lastToken = String(id)
    setRows((r) => {
      const idx = r.findIndex((row) => row[pkCol] === id)
      if (ev.type === 'delete') {
        if (idx >= 0) return [...r.slice(0, idx), ...r.slice(idx + 1)]
        return r
      }
      if (ev.type === 'insert' && idx === -1) {
        return [ev.after, ...r]
      }
      if (ev.type === 'update' && idx >= 0) {
        const copy = r.slice()
        copy[idx] = ev.after
        return copy
      }
      return r
    })
  }

  function inferPK(): string {
    const first = rows()[0]
    return first && first['id'] !== undefined
      ? 'id'
      : (first ? Object.keys(first)[0] : 'id') || 'id'
  }

  function ensureStream() {
    if (!es) openStream()
  }

  createEffect(() => {
    fetchPage()
    ensureStream()
  })
  onCleanup(() => es?.close())

  // Infinite scroll
  const onScroll = () => {
    const el = container
    if (!el) return
    if (
      nextCursor() &&
      el.scrollTop + el.clientHeight + 400 >= el.scrollHeight
    ) {
      fetchPage(nextCursor())
    }
  }

  // Live toggle apply queued events by refetch if desired
  function resumeLive() {
    setLive(true)
    setPendingEvents(0)
    // simple refetch first page to reconcile
    setRows([])
    setNextCursor(undefined)
    fetchPage()
  }

  // Filtering & sorting (client side MVP)
  const filteredSorted = () => {
    let data = rows()
    const f = filters()
    if (Object.keys(f).length) {
      data = data.filter((row) =>
        Object.entries(f).every(
          ([k, v]) =>
            !v ||
            String(row[k] ?? '')
              .toLowerCase()
              .includes(v.toLowerCase())
        )
      )
    }
    const s = sort()
    if (s) {
      data = [...data].sort((a, b) => {
        const av = a[s.col]
        const bv = b[s.col]
        if (av == null && bv == null) return 0
        if (av == null) return -1
        if (bv == null) return 1
        if (av < bv) return s.dir === 'asc' ? -1 : 1
        if (av > bv) return s.dir === 'asc' ? 1 : -1
        return 0
      })
    }
    return data
  }

  // Virtualization calculations
  const viewportHeight = () => 500
  const total = () => filteredSorted().length
  const scrollTop = () => container?.scrollTop || 0
  const start = () => Math.max(0, Math.floor(scrollTop() / ROW_HEIGHT) - 5)
  const end = () =>
    Math.min(total(), start() + Math.ceil(viewportHeight() / ROW_HEIGHT) + 10)
  const visible = () => filteredSorted().slice(start(), end())
  const offsetY = () => start() * ROW_HEIGHT

  function cycleSort(col: string) {
    setSort((s) =>
      !s || s.col !== col
        ? { col, dir: 'asc' }
        : s.dir === 'asc'
          ? { col, dir: 'desc' }
          : null
    )
  }

  function setFilter(col: string, val: string) {
    setFilters((f) => {
      const n = { ...f }
      if (!val) delete n[col]
      else n[col] = val
      return n
    })
  }

  function onCellDoubleClick(rIndex: number, col: string, value: any) {
    setEditing({ row: rIndex, col, value: String(value ?? '') })
  }

  async function commitEdit() {
    const edit = editing()
    if (!edit) return
    const pk = inferPK()
    const row = filteredSorted()[edit.row]
    const rowId = row?.[pk]
    if (!rowId) {
      setEditing(null)
      return
    }
    try {
      await fetch(
        apiUrl(
          `/api/db/${encodeURIComponent(props.dbId)}/tables/${encodeURIComponent(props.table)}/rows/${encodeURIComponent(String(rowId))}`
        ),
        {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ [edit.col]: coerceValue(edit.value) })
        }
      )
      setRows((rs) =>
        rs.map((r) =>
          r[pk] === rowId ? { ...r, [edit.col]: coerceValue(edit.value) } : r
        )
      )
    } catch {}
    setEditing(null)
  }

  function coerceValue(v: string): any {
    if (v === '') return ''
    if (v === 'true') return true
    if (v === 'false') return false
    if (!isNaN(Number(v))) return Number(v)
    // ISO timestamp detection
    if (/^\d{4}-\d{2}-\d{2}T/.test(v)) return v
    try {
      if (
        (v.startsWith('{') && v.endsWith('}')) ||
        (v.startsWith('[') && v.endsWith(']'))
      )
        return JSON.parse(v)
    } catch {}
    return v
  }

  function onKey(e: KeyboardEvent) {
    if (!selection()) return
    const sel = selection()!
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'c') {
      const row = filteredSorted()[sel.row]
      if (!row) return
      const val = row[sel.col]
      navigator.clipboard.writeText(String(val ?? ''))
    }
    if (e.key === 'Enter') {
      const row = filteredSorted()[sel.row]
      if (!row) return
      onCellDoubleClick(sel.row, sel.col, row[sel.col])
    }
  }
  window.addEventListener('keydown', onKey)
  onCleanup(() => window.removeEventListener('keydown', onKey))

  const headers = () => {
    const first = rows()[0]
    return first ? Object.keys(first) : []
  }

  return (
    <div class="space-y-2">
      <div class="flex items-center gap-2 text-xs">
        <button
          class="px-2 py-1 border rounded"
          onClick={() =>
            setLive((l) => {
              if (l) es?.close()
              else {
                es?.close()
                es = undefined
                openStream()
                if (pendingEvents()) resumeLive()
              }
              return !l
            })
          }
        >
          {live() ? 'Pause Live' : `Resume (${pendingEvents()})`}
        </button>
        <button
          class="px-2 py-1 border rounded"
          onClick={() => {
            setRows([])
            setNextCursor(undefined)
            fetchPage()
          }}
        >
          Refresh
        </button>
        <span class="text-neutral-500">Rows: {rows().length}</span>
        {sort() && (
          <span class="text-neutral-500">
            Sort: {sort()!.col} {sort()!.dir}
          </span>
        )}
        <button
          class="ml-auto px-2 py-1 border rounded"
          onClick={() => {
            setRows([])
            setNextCursor(undefined)
            fetchPage(lastToken)
          }}
        >
          Reconcile
        </button>
        <span>
          <StreamStatus state={live() ? streamState() : 'reconnecting'} />
        </span>
      </div>
      <div class="border rounded-md bg-white dark:bg-neutral-900 text-sm relative">
        <div
          class="overflow-auto"
          style={{ height: `${viewportHeight()}px` }}
          ref={(el) => {
            container = el
            container.addEventListener('scroll', onScroll)
          }}
        >
          <table class="min-w-full relative" style={{ contain: 'strict' }}>
            <thead class="sticky top-0 bg-neutral-50 dark:bg-neutral-800 z-10">
              <tr>
                <For each={headers()}>
                  {(h) => (
                    <th
                      class="px-2 py-1 font-medium text-left cursor-pointer select-none"
                      onClick={() => cycleSort(h)}
                    >
                      {h}
                      {sort()?.col === h &&
                        (sort()!.dir === 'asc' ? ' ▲' : ' ▼')}
                    </th>
                  )}
                </For>
              </tr>
              <tr>
                <For each={headers()}>
                  {(h) => (
                    <th class="px-1 py-0.5 bg-neutral-50 dark:bg-neutral-800">
                      <input
                        class="w-full border rounded px-1 py-0.5 text-xs bg-white dark:bg-neutral-900"
                        value={filters()[h] || ''}
                        onInput={(e) => setFilter(h, e.currentTarget.value)}
                        placeholder="filter"
                      />
                    </th>
                  )}
                </For>
              </tr>
            </thead>
            <tbody>
              <tr style={{ height: `${offsetY()}px` }} aria-hidden="true" />
              <For each={visible()}>
                {(row, i) => {
                  const globalIndex = () => start() + i()
                  return (
                    <tr class="border-b last:border-0 hover:bg-neutral-50 dark:hover:bg-neutral-800">
                      <For each={headers()}>
                        {(col) => {
                          const isSel = () =>
                            selection()?.row === globalIndex() &&
                            selection()?.col === col
                          const isEdit = () =>
                            editing()?.row === globalIndex() &&
                            editing()?.col === col
                          return (
                            <td
                              class={`px-2 py-1 whitespace-nowrap relative ${isSel() ? 'outline outline-2 outline-brand-500' : ''}`}
                              onClick={() =>
                                setSelection({ row: globalIndex(), col })
                              }
                              onDblClick={() =>
                                onCellDoubleClick(globalIndex(), col, row[col])
                              }
                            >
                              <Show
                                when={isEdit()}
                                fallback={<span>{formatVal(row[col])}</span>}
                              >
                                <input
                                  autofocus
                                  class="absolute inset-0 w-full h-full px-1 py-0.5 text-xs bg-white dark:bg-neutral-900 border"
                                  value={editing()!.value}
                                  onInput={(e) =>
                                    setEditing((ed) =>
                                      ed
                                        ? {
                                            ...ed,
                                            value: e.currentTarget.value
                                          }
                                        : ed
                                    )
                                  }
                                  onKeyDown={(e) => {
                                    if (e.key === 'Enter') commitEdit()
                                    if (e.key === 'Escape') setEditing(null)
                                  }}
                                  onBlur={commitEdit}
                                />
                              </Show>
                            </td>
                          )
                        }}
                      </For>
                    </tr>
                  )
                }}
              </For>
              <tr
                style={{
                  height: `${Math.max(0, (total() - end()) * ROW_HEIGHT)}px`
                }}
                aria-hidden="true"
              />
            </tbody>
          </table>
          {loading() && (
            <div class="absolute bottom-2 right-2 text-xs bg-neutral-200 dark:bg-neutral-700 px-2 py-1 rounded">
              Loading…
            </div>
          )}
          {error() && (
            <div class="absolute bottom-2 left-2 text-xs text-red-600">
              {error()}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function formatVal(v: any): string {
  if (v == null) return ''
  if (typeof v === 'object') {
    try {
      return JSON.stringify(v)
    } catch {
      return String(v)
    }
  }
  return String(v)
}
