import { Tabs, TabPanel } from '../components/Tabs';
import Card from '../components/Card';
import StatusPill from '../components/StatusPill';
import KeyValueList from '../components/KeyValueList';
import LogViewer from '../components/LogViewer';
import { A, useParams } from '@solidjs/router';
import { Show, createResource, createSignal } from 'solid-js';
import { getServer } from '../lib/api';
import { formatDate } from '../lib/format';

export default function ServerDetail() {
  const params = useParams();
  const [tab, setTab] = createSignal<'info'|'debug'|'error'>('info');
  const [srv] = createResource(() => params.id, (id: string) => getServer(id));

  return (
    <div class="flex flex-col gap-4">
      <Show when={srv()} fallback={<div>Loading…</div>}>
        {(server) => (
          <>
            <Card title={server().name} actions={<StatusPill status={server().status} />}>
              <div class="grid sm:grid-cols-2 gap-4">
                <div>
                  <div class="text-sm"><span class="text-neutral-500">Image:</span> {server().image}</div>
                  <div class="text-sm"><span class="text-neutral-500">Node:</span> {server().node ?? '-'}</div>
                  <div class="text-sm"><span class="text-neutral-500">Created:</span> {formatDate(server().created_at)}</div>
                  <div class="text-sm"><span class="text-neutral-500">Updated:</span> {formatDate(server().updated_at)}</div>
                  <div class="text-sm"><span class="text-neutral-500">Ports:</span> {(server().ports ?? []).map((p) => `${p.name ?? ''}:${p.port}`).join(', ') || '-'}</div>
                </div>
                <div>
                  <div class="text-sm"><span class="text-neutral-500">Resources:</span> {server()?.resources ? `${server()?.resources?.cpu ?? ''} ${server()?.resources?.memory ?? ''}` : '-'}</div>
                  <div class="mt-2"><div class="text-xs text-neutral-500">Args</div><div class="text-sm">{(server().args ?? []).join(' ') || '—'}</div></div>
                  <div class="mt-2"><div class="text-xs text-neutral-500">Env</div><KeyValueList data={server().env ?? {}} /></div>
                </div>
              </div>
            </Card>

            <Card title="Logs">
              <Tabs tabs={[{id:'info',label:'Info'},{id:'debug',label:'Debug'},{id:'error',label:'Error'}]} value={tab()} onChange={(t) => setTab(t as any)} />
              <TabPanel when={tab() === 'info'}>
                <LogViewer serverId={server().id} level="info" />
              </TabPanel>
              <TabPanel when={tab() === 'debug'}>
                <LogViewer serverId={server().id} level="debug" />
              </TabPanel>
              <TabPanel when={tab() === 'error'}>
                <LogViewer serverId={server().id} level="error" />
              </TabPanel>
            </Card>

            <Card title="Events">
              <div class="text-sm text-neutral-500">No events stream connected.</div>
            </Card>
          </>
        )}
      </Show>
    </div>
  );
}
