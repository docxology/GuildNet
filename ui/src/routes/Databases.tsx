import { createResource, For, createSignal, Show } from 'solid-js'
import { apiUrl } from '../lib/config'
import Button from '../components/Button'
import Modal from '../components/Modal'
import Input from '../components/Input'
import {
  createDatabase,
  deleteDatabase,
  listDatabases,
  type DatabaseInstance
} from '../lib/api'
import { createEffect, onCleanup } from 'solid-js'
import { pushToast } from '../components/Toaster'

async function fetchDatabases(): Promise<DatabaseInstance[]> {
  return listDatabases()
}

export default function Databases() {
  const [dbs, { refetch }] = createResource(fetchDatabases)
  const [open, setOpen] = createSignal(false)
  const [id, setId] = createSignal('')
  const [name, setName] = createSignal('')
  const [desc, setDesc] = createSignal('')
  const [creating, setCreating] = createSignal(false)
  const [dbStatus, setDbStatus] = createSignal<
    'ok' | 'unavailable' | 'unknown'
  >('unknown')
  // poll db health lightly while page is open
  createEffect(() => {
    let cancelled = false
    const tick = async () => {
      try {
        const res = await fetch(apiUrl('/api/db/health'))
        const j = await res.json()
        if (!cancelled) setDbStatus(j?.status === 'ok' ? 'ok' : 'unavailable')
      } catch {
        if (!cancelled) setDbStatus('unavailable')
      }
    }
    tick()
    const id = setInterval(tick, 10000)
    onCleanup(() => {
      cancelled = true
      clearInterval(id)
    })
  })
  const submit = async () => {
    if (!id().trim() || !name().trim()) {
      pushToast({ type: 'error', message: 'Database ID and Name are required' })
      return
    }
    setCreating(true)
    try {
      const res = await createDatabase({
        id: id().trim(),
        name: name().trim(),
        description: desc().trim() || undefined
      })
      if (!res) {
        pushToast({ type: 'error', message: 'Create failed' })
        return
      }
      pushToast({ type: 'success', message: 'Database created' })
      setOpen(false)
      setId('')
      setName('')
      setDesc('')
      refetch()
    } catch (e: any) {
      pushToast({ type: 'error', message: e?.message || 'Create failed' })
    } finally {
      setCreating(false)
    }
  }
  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h1 class="text-xl font-semibold">Databases</h1>
        <Button onClick={() => setOpen(true)} variant="primary">
          New Database
        </Button>
      </div>
      <div class="border rounded-lg divide-y overflow-hidden bg-white dark:bg-neutral-900">
        {dbStatus() === 'unavailable' && (
          <div class="p-3 text-xs bg-amber-50 text-amber-900 border-b">
            Database backend is unavailable. You can create metadata, but tables
            and data operations are disabled until the DB is reachable.
          </div>
        )}
        <For each={dbs()}>
          {(d) => {
            const onDelete = async (e: Event) => {
              e.preventDefault()
              e.stopPropagation()
              if (
                !window.confirm(
                  `Delete database ${d.id}? This cannot be undone.`
                )
              )
                return
              const ok = await deleteDatabase(d.id)
              if (ok) {
                pushToast({ type: 'success', message: 'Database deleted' })
                refetch()
              } else {
                pushToast({ type: 'error', message: 'Delete failed' })
              }
            }
            return (
              <a
                href={`/databases/${encodeURIComponent(d.id)}`}
                class="flex items-center px-4 py-3 hover:bg-neutral-50 dark:hover:bg-neutral-800"
              >
                <div class="flex-1">
                  <div class="font-medium">{d.name}</div>
                  <div class="text-xs text-neutral-500">ID: {d.id}</div>
                </div>
                <div class="flex items-center gap-3">
                  <div class="text-xs text-neutral-500">{d.created_at}</div>
                  <button
                    class="text-xs text-red-600 hover:underline"
                    onClick={onDelete}
                  >
                    Delete
                  </button>
                </div>
              </a>
            )
          }}
        </For>
        {dbs.loading && <div class="p-4 text-sm">Loading…</div>}
        {dbs() && dbs()!.length === 0 && !dbs.loading && (
          <div class="p-4 text-sm text-neutral-500">No databases yet.</div>
        )}
      </div>
      <Modal
        title="Create Database"
        open={open()}
        onClose={() => setOpen(false)}
        footer={
          <>
            <Button
              type="button"
              onClick={() => setOpen(false)}
              class="!bg-neutral-100 dark:!bg-neutral-800"
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="primary"
              disabled={creating() || name().trim().length === 0}
              onClick={submit}
            >
              {creating() ? 'Creating…' : 'Create'}
            </Button>
          </>
        }
      >
        <div class="space-y-4">
          <div>
            <label class="block text-xs font-medium mb-1">Database ID</label>
            <Input
              value={id()}
              onInput={(e) => setId(e.currentTarget.value)}
              placeholder="analytics"
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Name</label>
            <Input
              value={name()}
              onInput={(e) => setName(e.currentTarget.value)}
              placeholder="Analytics"
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1">Description</label>
            <Input
              value={desc()}
              onInput={(e) => setDesc(e.currentTarget.value)}
              placeholder="Optional description"
            />
          </div>
          <p class="text-xs text-neutral-500">
            Create a new logical database. ID must be unique in your org. Name
            is optional.
          </p>
        </div>
      </Modal>
    </div>
  )
}
