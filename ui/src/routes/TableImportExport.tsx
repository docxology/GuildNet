import { For, Show, createMemo, createSignal } from 'solid-js'
import { useParams } from '@solidjs/router'
import Button from '../components/Button'
import Input from '../components/Input'
import { pushToast } from '../components/Toaster'
import { apiUrl } from '../lib/config'

export default function TableImportExport() {
  const params = useParams()
  const [raw, setRaw] = createSignal('[{"id":"1","name":"example"}]')
  const [preview, setPreview] = createSignal<any>(null)
  const [result, setResult] = createSignal<any>(null)
  const [format, setFormat] = createSignal<'json' | 'csv'>('json')
  const [mapping, setMapping] = createSignal<Record<string, string>>({})
  const previewRows = () =>
    (preview()?.rows || preview()?.Rows || preview()?.data || []) as any[]
  const sourceColumns = createMemo(() => {
    const first = previewRows()[0]
    if (!first) return []
    // try raw first, fallback mapped
    const obj = (first.raw || first) as Record<string, any>
    return Object.keys(obj || {})
  })
  const targetColumns = createMemo(() => {
    // Cannot fetch schema here easily; allow free text target columns users can set
    // Alternatively, users can set any desired column names.
    return Array.from(
      new Set([...sourceColumns(), ...Object.values(mapping())])
    )
  })
  const setMap = (from: string, to: string) =>
    setMapping((m) => ({ ...m, [from]: to }))
  function tryParseRaw(text: string, fmt: 'json' | 'csv'): any[] {
    if (fmt === 'json') {
      try {
        const v = JSON.parse(text)
        if (Array.isArray(v)) return v
        if (v && typeof v === 'object') return [v]
        return []
      } catch {
        return []
      }
    }
    // naive CSV: first line headers, comma-separated, no advanced quoting
    const lines = text
      .split(/\r?\n/)
      .map((l) => l.trim())
      .filter((l): l is string => !!l)
    if (lines.length === 0) return []
    const first = lines[0] ?? ''
    const head = first.split(',').map((h) => h.trim())
    const out: any[] = []
    for (let i = 1; i < lines.length; i++) {
      const line = lines[i] ?? ''
      const cols = line.split(',')
      const row: Record<string, any> = {}
      head.forEach((h, idx) => {
        const cell = idx < cols.length ? cols[idx] : ''
        row[h] = (cell ?? '').trim()
      })
      out.push(row)
    }
    return out
  }
  const dryRun = async () => {
    const res = await fetch(
      apiUrl(
        `/api/cluster/${encodeURIComponent(params.clusterId || '')}/db/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(
          params.table || ''
        )}/import?dry_run=1`
      ),
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          rows: tryParseRaw(raw(), format()),
          mapping: mapping()
        })
      }
    )
    try {
      setPreview(await res.json())
      pushToast({ type: 'success', message: 'Dry run complete' })
    } catch {
      setPreview({ error: 'parse failed' })
      pushToast({ type: 'error', message: 'Dry run parse failed' })
    }
  }
  const runImport = async () => {
    const res = await fetch(
      apiUrl(
        `/api/cluster/${encodeURIComponent(params.clusterId || '')}/db/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(
          params.table || ''
        )}/import`
      ),
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          rows: tryParseRaw(raw(), format()),
          mapping: mapping()
        })
      }
    )
    try {
      setResult(await res.json())
      pushToast({ type: 'success', message: 'Import completed' })
    } catch {
      setResult({ error: 'import failed' })
      pushToast({ type: 'error', message: 'Import failed' })
    }
  }
  const exportRows = () => {
    window.open(
      apiUrl(
        `/api/cluster/${encodeURIComponent(params.clusterId || '')}/db/${encodeURIComponent(params.dbId || '')}/tables/${encodeURIComponent(
          params.table || ''
        )}/export?format=${format()}`
      ),
      '_blank'
    )
    pushToast({ type: 'info', message: 'Export started (new tab)' })
  }
  return (
    <div class="space-y-4">
      <h1 class="text-xl font-semibold">Import / Export</h1>
      <div class="space-y-2">
        <div class="flex gap-2 items-center text-xs">
          <label class="flex items-center gap-1">
            <input
              type="radio"
              checked={format() === 'json'}
              onChange={() => setFormat('json')}
            />{' '}
            JSON
          </label>
          <label class="flex items-center gap-1">
            <input
              type="radio"
              checked={format() === 'csv'}
              onChange={() => setFormat('csv')}
            />{' '}
            CSV
          </label>
          <Button onClick={dryRun}>Dry Run</Button>
          <Button variant="primary" onClick={runImport}>
            Import
          </Button>
          <Button onClick={exportRows}>Export</Button>
        </div>
        <textarea
          class="w-full h-48 border rounded p-2 font-mono text-xs"
          value={raw()}
          onInput={(e) => setRaw(e.currentTarget.value)}
        />
      </div>
      {preview() && (
        <div class="text-xs space-y-2">
          <div class="font-semibold">Preview</div>
          <pre class="bg-neutral-100 dark:bg-neutral-800 p-2 rounded overflow-auto max-h-60">
            {JSON.stringify(preview(), null, 2)}
          </pre>
          <div class="space-y-2">
            <div class="font-semibold">Column mapping</div>
            <div
              class="grid gap-2"
              style={{ 'grid-template-columns': '1fr 20px 1fr' }}
            >
              <For each={sourceColumns()}>
                {(src) => (
                  <>
                    <div class="border rounded px-2 py-1 bg-neutral-50 dark:bg-neutral-800">
                      {src}
                    </div>
                    <div class="text-center">→</div>
                    <input
                      class="border rounded px-2 py-1"
                      value={mapping()[src] || src}
                      onInput={(e) => setMap(src, e.currentTarget.value)}
                    />
                  </>
                )}
              </For>
            </div>
            <div class="text-[10px] text-neutral-500">
              Adjust targets to match your table schema before running import.
            </div>
            <Show when={previewRows().length}>
              <div class="mt-3">
                <div class="font-semibold mb-1">Row errors</div>
                <For each={preview()?.rows || preview()?.Rows}>
                  {(r: any, i) => (
                    <div class="text-[11px] px-2 py-1 border-b last:border-0 flex items-center gap-2">
                      <span class="text-neutral-500">#{i() + 1}</span>
                      <Show
                        when={(r.errors || r.Errors || []).length}
                        fallback={<span class="text-green-700">ok</span>}
                      >
                        <For each={r.errors || r.Errors}>
                          {(e: string) => (
                            <span class="px-1.5 py-0.5 rounded bg-red-100 text-red-800">
                              {e}
                            </span>
                          )}
                        </For>
                      </Show>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </div>
      )}
      {result() && (
        <div class="text-xs space-y-2">
          <div class="font-semibold">Result</div>
          <pre class="bg-neutral-100 dark:bg-neutral-800 p-2 rounded overflow-auto max-h-60">
            {JSON.stringify(result(), null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
