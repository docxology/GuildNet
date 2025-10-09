import { createResource, For, createSignal, Show } from 'solid-js'
import { apiUrl } from '../lib/config'
import Button from '../components/Button'
import Modal from '../components/Modal'
import Input from '../components/Input'
import {
  createDatabase,
  listDatabases,
  type DatabaseInstance
} from '../lib/api'

async function fetchDatabases(): Promise<DatabaseInstance[]> {
  return listDatabases()
}

export default function Databases() {
  const [dbs, { refetch }] = createResource(fetchDatabases)
  const [open, setOpen] = createSignal(false)
  const [name, setName] = createSignal('')
  const [desc, setDesc] = createSignal('')
  const [creating, setCreating] = createSignal(false)
  const submit = async () => {
    if (!name().trim()) return
    setCreating(true)
    await createDatabase({
      name: name().trim(),
      description: desc().trim() || undefined
    })
    setCreating(false)
    setOpen(false)
    setName('')
    setDesc('')
    refetch()
  }
  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h1 class="text-xl font-semibold">Databases</h1>
        <Button onClick={() => setOpen(true)} variant="primary">New Database</Button>
      </div>
      <div class="border rounded-lg divide-y overflow-hidden bg-white dark:bg-neutral-900">
        <For each={dbs()}>
          {(d) => (
            <a
              href={`/databases/${encodeURIComponent(d.id)}`}
              class="flex items-center px-4 py-3 hover:bg-neutral-50 dark:hover:bg-neutral-800"
            >
              <div class="flex-1">
                <div class="font-medium">{d.name}</div>
                <div class="text-xs text-neutral-500">ID: {d.id}</div>
              </div>
              <div class="text-xs text-neutral-500">{d.created_at}</div>
            </a>
          )}
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
            <label class="block text-xs font-medium mb-1">Name</label>
            <Input
              value={name()}
              onInput={(e) => setName(e.currentTarget.value)}
              placeholder="analytics"
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
            A single multi-tenant DB is used in this MVP; creation updates
            metadata only.
          </p>
        </div>
      </Modal>
    </div>
  )
}
