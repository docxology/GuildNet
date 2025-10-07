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

  // Prefer external ingress URL when present; fallback to internal proxy
  const ideUrl = createMemo(() => {
    const s = srv();
    if (!s) return '';
    if (s.url && s.url.startsWith('https://')) return s.url;
    return apiUrl(`/proxy/server/${encodeURIComponent(s.id)}/`);
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
                <Show when={ideUrl()} fallback={<div class="text-sm text-neutral-500">IDE not available.</div>}>
                  {(url) => {
                    const [loaded, setLoaded] = createSignal(false);
                    return (
                      <div class="relative border rounded-md overflow-hidden h-[70vh]">
                        <iframe
                          src={url()}
                          title="code-server"
                          class="w-full h-full bg-white"
                          referrerpolicy="no-referrer"
                          allow="clipboard-read; clipboard-write;"
                          onLoad={() => setLoaded(true)}
                        />
                        <Show when={!loaded()}>
                          <div class="absolute inset-0 flex items-center justify-center bg-white/70 text-sm text-neutral-600">
                            Opening IDE…
                          </div>
                        </Show>
                      </div>
                    );
                  }}
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
