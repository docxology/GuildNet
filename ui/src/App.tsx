import { lazy, onMount, createResource, createSignal, For, Show, createEffect, onCleanup } from 'solid-js'
import {
  A,
  Route,
  Router,
  useNavigate,
  useParams,
  useSearchParams,
  type RouteSectionProps
} from '@solidjs/router'
import Toaster, { pushToast } from './components/Toaster'
import { listClusters, clusterHealth, createClusterRecord, attachClusterKubeconfig } from './lib/api'
import Modal from './components/Modal'

const Servers = lazy(() => import('./routes/Servers'))
const ServerDetail = lazy(() => import('./routes/ServerDetail'))
const Launch = lazy(() => import('./routes/Launch'))
const Databases = lazy(() => import('./routes/Databases'))
const DatabaseDetail = lazy(() => import('./routes/DatabaseDetail'))
const TableView = lazy(() => import('./routes/TableView'))
const TableSchema = lazy(() => import('./routes/TableSchema'))
const TableAudit = lazy(() => import('./routes/TableAudit'))
const TablePermissions = lazy(() => import('./routes/TablePermissions'))
const TableImportExport = lazy(() => import('./routes/TableImportExport'))
const Deploy = lazy(() => import('./routes/Deploy'))
const Settings = lazy(() => import('./routes/Settings'))

const getFirst = (v: string | string[] | undefined): string =>
  Array.isArray(v) ? (v[0] || '') : (v || '')

const Home = () => (
  <div class="p-6 text-sm text-neutral-600">
    <div class="font-semibold mb-2">No cluster selected</div>
    <div>Select a cluster from the sidebar or click Add to connect one.</div>
  </div>
)

function LegacyRedirect(target: 'servers' | 'launch' | 'databases' | 'settings') {
  return function Legacy() {
    const navigate = useNavigate()
    const [s] = useSearchParams()
    createEffect(() => {
      const legacyCid = getFirst((s as any).cluster)
      if (legacyCid) {
        navigate(`/c/${encodeURIComponent(legacyCid)}/${target}`, { replace: true })
      }
    })
    return <Home />
  }
}

const LegacyServers = LegacyRedirect('servers')
const LegacyLaunch = LegacyRedirect('launch')
const LegacyDatabases = LegacyRedirect('databases')
const LegacySettings = LegacyRedirect('settings')

function Sidebar() {
  const navigate = useNavigate()
  const [clusters, { refetch }] = createResource(listClusters)
  const [busy, setBusy] = createSignal(false)
  const [kc, setKc] = createSignal('')
  const [open, setOpen] = createSignal(false)
  const [name, setName] = createSignal('')

  const looksLikeKubeconfig = (s: string) => {
    const t = s.trim()
    if (!t) return false
    // Very light heuristic to avoid obvious paste mistakes
    return /apiVersion:\s*v1/i.test(t) && /(clusters|contexts|users):/i.test(t)
  }

  const startWizard = () => {
    setOpen(true)
  }

  const submitWizard = async () => {
    if (busy()) return
    const pasted = kc().trim()
    if (pasted && !looksLikeKubeconfig(pasted)) {
      pushToast({ type: 'error', message: 'Kubeconfig does not look valid' })
      return
    }
    setBusy(true)
    try {
      const rec = await createClusterRecord(name().trim() || undefined)
      if (!rec?.id) {
        pushToast({ type: 'error', message: 'Failed to create cluster record' })
        return
      }
      if (pasted) {
        const ok = await attachClusterKubeconfig(rec.id, pasted)
        if (!ok) {
          pushToast({ type: 'error', message: 'Attach failed. Fix the kubeconfig and try again.' })
          return
        }
      }
      setKc('')
      setName('')
      setOpen(false)
      refetch()
      navigate(`/c/${encodeURIComponent(rec.id)}/servers`)
    } catch (e) {
      pushToast({ type: 'error', message: (e as Error).message || 'Error' })
    } finally {
      setBusy(false)
    }
  }

  const ClusterRow = (props: { id: string; name?: string }) => {
    const [health, { refetch: refetchHealth }] = createResource(() => props.id, clusterHealth)
    let timer: number | undefined
    createEffect(() => {
      if (timer) window.clearInterval(timer)
      refetchHealth()
      timer = window.setInterval(() => refetchHealth(), 30000)
      onCleanup(() => { if (timer) window.clearInterval(timer) })
    })
    const dot = () => {
      const h = health()
      if (h === 'ok') return 'bg-green-500'
      if (h === 'error') return 'bg-red-500'
      return 'bg-neutral-400'
    }
    const statusText = () => health() || 'unknown'
    return (
      <A href={`/c/${encodeURIComponent(props.id)}/servers`} class="flex items-center gap-2 px-2 py-1 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800" title={`Health: ${statusText()}`}>
        <span class={`w-2 h-2 rounded-full ${dot()}`} />
        <span class="truncate text-sm">{props.name || props.id}</span>
        <span class="ml-auto text-[10px] text-neutral-500 uppercase">{statusText()}</span>
      </A>
    )
  }

  return (
    <aside class="w-64 border-r bg-neutral-50/40 dark:bg-neutral-900/30 p-3 space-y-3">
      <div class="flex items-center justify-between">
        <div class="font-semibold text-sm">Clusters</div>
        <div class="flex items-center gap-2">
          <button class="btn" onClick={startWizard}>Add</button>
        </div>
      </div>
      <div class="space-y-1">
        <For each={clusters() ?? []}>{(c) => (
          <ClusterRow id={c.id} name={c.name} />
        )}</For>
      </div>
      <Modal
        title="Connect a cluster"
        open={open()}
        onClose={() => { if (!busy()) setOpen(false) }}
        footer={
          <>
            <button class="btn" onClick={() => setOpen(false)} disabled={busy()}>Cancel</button>
            <button class="btn" onClick={submitWizard} disabled={busy()}>{busy() ? 'Connecting…' : 'Save'}</button>
          </>
        }
      >
        <div class="space-y-3">
          <label class="block text-sm">
            Name (optional)
            <input class="mt-1 w-full rounded-md border px-3 py-2" value={name()} onInput={e => setName(e.currentTarget.value)} />
          </label>
          <label class="block text-sm">
            Paste kubeconfig (optional)
            <textarea class="mt-1 w-full h-40 rounded-md border px-3 py-2 font-mono text-xs" placeholder="Paste kubeconfig YAML" value={kc()} onInput={e => setKc(e.currentTarget.value)} />
            <div class="text-xs text-neutral-500 mt-1">If omitted, you can attach later in Settings.</div>
          </label>
        </div>
      </Modal>
    </aside>
  )
}

function ClusterShell(props: RouteSectionProps) {
  const params = useParams()
  const [search] = useSearchParams()
  const cid = () => params.clusterId || getFirst((search as any).cluster) || ''
  const enc = (s: string) => encodeURIComponent(s || '')
  return (
    <div class="min-h-screen flex flex-col">
      <header class="border-b sticky top-0 z-10 bg-white/70 dark:bg-neutral-900/70 backdrop-blur">
        <div class="px-4 sm:px-6 lg:px-8 flex items-center gap-4 h-12">
          <A href="/" class="font-semibold">GuildNet</A>
          <Show when={!!cid()}>
            <nav class="flex items-center gap-3 text-sm">
              <A href={`/c/${enc(cid())}/servers`} activeClass="text-brand-600" class="hover:underline">Servers</A>
              <A href={`/c/${enc(cid())}/launch`} activeClass="text-brand-600" class="hover:underline">Launch</A>
              <A href={`/c/${enc(cid())}/databases`} activeClass="text-brand-600" class="hover:underline">Databases</A>
              <A href={`/c/${enc(cid())}/settings`} activeClass="text-brand-600" class="hover:underline">Settings</A>
            </nav>
          </Show>
        </div>
      </header>
      <div class="flex flex-1 min-h-0">
        <Sidebar />
        <main class="flex-1 px-4 sm:px-6 lg:px-8 py-4 overflow-auto">
          <Show when={props.children} fallback={<Home />}>{props.children}</Show>
        </main>
      </div>
      <Toaster />
    </div>
  )
}

export default function App() {
  return (
    <Router>
      <Route path="/" component={ClusterShell}>
        <Route path="/c/:clusterId" component={Servers} />
        <Route path="/c/:clusterId/servers" component={Servers} />
        <Route path="/c/:clusterId/launch" component={Launch} />
        <Route path="/c/:clusterId/databases" component={Databases} />
        <Route path="/c/:clusterId/settings" component={Settings} />
        <Route path="/servers/:id" component={ServerDetail} />
        {/* Home route when no cluster */}
        <Route path="/" component={Home} />
        {/* Legacy routes: redirect if ?cluster=ID, else show Home */}
        <Route path="/servers" component={LegacyServers} />
        <Route path="/launch" component={LegacyLaunch} />
        <Route path="/databases" component={LegacyDatabases} />
        <Route path="/settings" component={LegacySettings} />
        {/* Legacy deep DB routes unchanged */}
        <Route path="/databases/:dbId" component={DatabaseDetail} />
        <Route path="/databases/:dbId/tables/:table" component={TableView} />
        <Route path="/databases/:dbId/tables/:table/schema" component={TableSchema} />
        <Route path="/databases/:dbId/tables/:table/audit" component={TableAudit} />
        <Route path="/databases/:dbId/tables/:table/permissions" component={TablePermissions} />
        <Route path="/databases/:dbId/tables/:table/import-export" component={TableImportExport} />
      </Route>
    </Router>
  )
}
