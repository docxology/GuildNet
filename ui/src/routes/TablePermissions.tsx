import { createMemo, createResource, For, Show, createSignal } from 'solid-js'
import { useParams } from '@solidjs/router'
import { apiUrl } from '../lib/config'
import Button from '../components/Button'
import { pushToast } from '../components/Toaster'
import Input from '../components/Input'

type Binding = { principal: string; scope: string; role: string; created_at?: string }

async function fetchPerms(db: string): Promise<Binding[]> {
  try {
    const r = await fetch(apiUrl(`/api/db/${encodeURIComponent(db)}/permissions`))
    if (!r.ok) return []
    return await r.json()
  } catch {
    return []
  }
}

export default function TablePermissions() {
  const params = useParams()
  const [perms, { refetch }] = createResource(() => params.dbId!, fetchPerms)
  const [schema] = createResource(async () => {
    if (!params.dbId || !params.table) return null
    try {
      const r = await fetch(apiUrl(`/api/db/${encodeURIComponent(params.dbId)}/tables/${encodeURIComponent(params.table)}`))
      if (!r.ok) return null
      return await r.json()
    } catch { return null }
  })
  const [principal, setPrincipal] = createSignal('user:demo')
  const [scope, setScope] = createSignal('db:' + (params.dbId || ''))
  const [role, setRole] = createSignal('viewer')
  const grouped = createMemo(() => {
    const byScope = new Map<string, Binding[]>()
    for (const p of perms() || []) {
      const arr = byScope.get(p.scope) || []
      arr.push(p)
      byScope.set(p.scope, arr)
    }
    return Array.from(byScope.entries())
  })
  const add = async () => {
    try {
      const res = await fetch(
        apiUrl(`/api/db/${encodeURIComponent(params.dbId || '')}/permissions`),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ principal: principal(), scope: scope(), role: role() })
        }
      )
      if (!res.ok) throw new Error(`${res.status}`)
      pushToast({ type: 'success', message: 'Permission saved' })
      refetch()
    } catch (e: any) {
      pushToast({ type: 'error', message: `Save failed: ${e?.message || ''}` })
    }
  }
  const revoke = async (s: string, p: string) => {
    try {
      const url = apiUrl(`/api/db/${encodeURIComponent(params.dbId || '')}/permissions?scope=${encodeURIComponent(s)}&principal=${encodeURIComponent(p)}`)
      const res = await fetch(url, { method: 'DELETE' })
      if (!res.ok) throw new Error(`${res.status}`)
      pushToast({ type: 'success', message: 'Permission revoked' })
      refetch()
    } catch (e: any) {
      pushToast({ type: 'error', message: `Revoke failed: ${e?.message || ''}` })
    }
  }
  return (
    <div class="space-y-4">
      <h1 class="text-xl font-semibold">Permissions</h1>
      <Show when={schema() && Array.isArray(schema()!.schema)}>
        <div class="text-xs p-3 border rounded bg-neutral-50 dark:bg-neutral-800">
          <div class="font-semibold mb-1">Masked columns in this table</div>
          <div class="flex flex-wrap gap-1">
            <For each={(schema()!.schema as any[]).filter((c:any)=>c.mask).map((c:any)=>c.name)}>
              {(n) => <span class="px-2 py-0.5 rounded bg-amber-100 text-amber-800">{n}</span>}
            </For>
            <Show when={!((schema()!.schema as any[]).some((c:any)=>c.mask))}>
              <span class="text-neutral-500">No masked columns</span>
            </Show>
          </div>
          <div class="mt-1 text-[10px] text-neutral-500">Viewer/Editor roles will see masked values as ***</div>
        </div>
      </Show>
      <div class="space-y-2 border rounded p-3 bg-white dark:bg-neutral-900">
        <div class="grid gap-2 md:grid-cols-3">
          <Input
            value={principal()}
            onInput={(e) => setPrincipal(e.currentTarget.value)}
            placeholder="principal (user:alice)"
          />
          <div class="flex gap-2">
            <Input
              value={scope()}
              onInput={(e) => setScope(e.currentTarget.value)}
              placeholder="scope (db:.. or table:..)"
            />
            <button class="text-xs border rounded px-2" onClick={() => setScope(`db:${params.dbId||''}`)}>db</button>
            <button class="text-xs border rounded px-2" onClick={() => setScope(`table:${params.table||''}`)}>table</button>
          </div>
          <select
            class="rounded-md border px-2 py-2 bg-white dark:bg-neutral-900"
            value={role()}
            onChange={(e) => setRole(e.currentTarget.value)}
          >
            <option value="viewer">viewer</option>
            <option value="editor">editor</option>
            <option value="maintainer">maintainer</option>
            <option value="admin">admin</option>
          </select>
        </div>
        <Button variant="primary" onClick={add}>
          Add / Update
        </Button>
      </div>
      <div class="border rounded bg-white dark:bg-neutral-900 text-sm">
        <For each={grouped()}>
          {([s, arr]) => (
            <div class="border-b last:border-0">
              <div class="px-4 py-2 font-semibold text-xs">{s}</div>
              <For each={arr}>
                {(p) => (
                  <div class="flex items-center gap-4 px-4 py-2">
                    <span class="font-mono text-xs">{p.principal}</span>
                    <span class="text-xs font-semibold uppercase">{p.role}</span>
                    <span class="ml-auto text-[10px] text-neutral-500">{p.created_at}</span>
                    <button class="text-xs text-red-600" onClick={() => revoke(s, p.principal)}>Revoke</button>
                  </div>
                )}
              </For>
            </div>
          )}
        </For>
        <Show when={(perms() || []).length === 0}><div class="px-4 py-2 text-xs text-neutral-500">No permissions</div></Show>
      </div>
    </div>
  )
}
