import { Tabs } from '../components/Tabs';
import Card from '../components/Card';
import StatusPill from '../components/StatusPill';
import KeyValueList from '../components/KeyValueList';
import LogViewer from '../components/LogViewer';
import { useParams } from '@solidjs/router';
import { Show, createMemo, createResource, createSignal } from 'solid-js';
import { getServer } from '../lib/api';
import { formatDate } from '../lib/format';
import { apiUrl } from '../lib/config';

export default function ServerDetail() {
  const params = useParams();
  const [tab, setTab] = createSignal<'info'|'debug'|'error'|'ide'>('info');
  const [srv] = createResource(() => params.id, (id: string) => getServer(id));

  // Always use 8080 for agent iframe unless 8443 is explicitly open (rare in dev)
  const ideUrl = createMemo(() => {
    const s = srv();
    if (!s) return '';
    const env = s.env ?? ({} as Record<string, string>);
    const host = (s as any).node || env['AGENT_HOST'] || env['HOST'] || env['SERVICE_HOST'];
    if (!host) return '';
    const ports = (s.ports ?? []).map((p) => p.port);
    // Use 8080 unless 8443 is explicitly open (for advanced/production use)
    const port = ports.includes(8443) ? 8443 : 8080;
    // Agent Caddy serves HTTP; the host reverse proxy sets TLS to the browser. Use scheme=http to upstream.
    return apiUrl(`/proxy?to=${encodeURIComponent(`${host}:${port}`)}&path=${encodeURIComponent('/')}&scheme=http`);
  });

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

            <Card title="Logs & Tools">
              <Tabs
                tabs={[
                  { id: 'info', label: 'Info' },
                  { id: 'debug', label: 'Debug' },
                  { id: 'error', label: 'Error' },
                  ...(ideUrl() ? [{ id: 'ide', label: 'IDE' }] : []),
                ] as { id: string; label: string }[]}
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
                <Show when={ideUrl()} fallback={<div class="text-sm text-neutral-500">IDE not available. Ensure this agent exposes port 8443 and has a node address.</div>}>
                  {(url) => (
                    <div class="border rounded-md overflow-hidden h-[70vh]">
                      <iframe
                        src={url()}
                        title="code-server"
                        class="w-full h-full bg-white"
                        referrerpolicy="no-referrer"
                        allow="clipboard-read; clipboard-write;"
                      />
                    </div>
                  )}
                </Show>
              )}
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
