import { createEffect, createSignal } from 'solid-js';
import { z } from 'zod';
import Card from '../components/Card';
import Input from '../components/Input';
import { getImageDefaults, postJob } from '../lib/api';
import { pushToast } from '../components/Toaster';
import { useNavigate } from '@solidjs/router';

const schema = z.object({
  image: z.string().min(1).regex(/^[\w\-\.\/]+:[\w\-\.]+|^[\w\-\.\/]+$/),
  name: z.string().optional(),
  args: z.array(z.string()).optional(),
  cpu: z.string().optional(),
  memory: z.string().optional(),
  env: z.record(z.string(), z.string()).optional(),
  labels: z.record(z.string(), z.string()).optional(),
  ports: z.array(z.object({ name: z.string().optional(), port: z.coerce.number().int().min(1).max(65535) })).optional(),
});

export default function Launch() {
  type ImageDefaults = { ports?: { name?: string; port: number }[]; env?: Record<string, string> };
  const navigate = useNavigate();
  const [image, setImage] = createSignal('');
  const imagePresets: Array<{ label: string; value: string }> = [
    { label: 'GuildNet Agent (VS Code)', value: 'guildnet/agent:dev' },
  ];
  const [name, setName] = createSignal('');
  const [args, setArgs] = createSignal<string[]>([]);
  const [env, setEnv] = createSignal<Array<{k:string;v:string}>>([]);
  const [labels, setLabels] = createSignal<Array<{k:string;v:string}>>([]);
  const [ports, setPorts] = createSignal<Array<{name:string;port:number}>>([]);
  const [cpu, setCpu] = createSignal('');
  const [memory, setMemory] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const [errors, setErrors] = createSignal<string[]>([]);

  // Prefill defaults by querying backend for the selected image
  createEffect(async () => {
    const img = image() || 'guildnet/agent:dev';
    if (!image()) setImage(img);
    const d: ImageDefaults = await getImageDefaults(img).catch(() => ({} as ImageDefaults));
    if ((ports()?.length ?? 0) === 0 && Array.isArray(d.ports)) setPorts(d.ports.map((p) => ({ name: p.name || '', port: p.port })));
    const existing: Record<string, string> = Object.fromEntries(env().map((e) => [e.k, e.v] as const));
    const merged: Record<string, string> = { ...(d.env || {}), ...existing };
    const arr = Object.entries(merged).map(([k, v]) => ({ k, v }));
    setEnv(arr);
  });

  const submit = async () => {
    setErrors([]);
    const envObj = Object.fromEntries(env().filter(e=>e.k).map((e)=>[e.k,e.v]));
    const labelsObj = Object.fromEntries(labels().filter(e=>e.k).map((e)=>[e.k,e.v]));
    const spec = { image: image(), name: name() || undefined, args: args(), env: envObj, labels: labelsObj, resources: { cpu: cpu()||undefined, memory: memory()||undefined }, expose: ports() } as any;
    const parsed = schema.safeParse({ image: spec.image, name: spec.name, args: spec.args, env: spec.env, labels: spec.labels, cpu: spec.resources.cpu, memory: spec.resources.memory, ports: spec.expose });
    if (!parsed.success) {
      setErrors(parsed.error.issues.map((e) => `${(e.path ?? []).join('.')}: ${e.message}`));
      pushToast({ type: 'error', message: 'Validation failed' });
      return;
    }
    setBusy(true);
    try {
      const res = await postJob(spec);
      pushToast({ type: 'success', message: `Job accepted (${res.id})` });
      // Optimistic navigate to servers; actual ID may be server/job
      navigate(`/servers/${encodeURIComponent(res.id)}`);
    } catch (e) {
      pushToast({ type: 'error', message: (e as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="flex flex-col gap-4">
      <Card title="Launch new workload">
        <div class="grid md:grid-cols-2 gap-4">
          <div class="space-y-3">
            <label class="block text-sm">Image preset
              <select class="w-full rounded-md border px-3 py-2 bg-white dark:bg-neutral-900 mt-1"
        onChange={async (e) => {
                  const v = e.currentTarget.value;
                  if (v) {
                    setImage(v);
                    // Load defaults for selected image
          const d: ImageDefaults = await getImageDefaults(v).catch(() => ({} as ImageDefaults));
          if ((ports()?.length ?? 0) === 0 && Array.isArray(d.ports)) setPorts(d.ports.map((p) => ({ name: p.name || '', port: p.port })));
          const existing: Record<string, string> = Object.fromEntries(env().map((e) => [e.k, e.v] as const));
          const merged: Record<string, string> = { ...(d.env || {}), ...existing };
          const arr = Object.entries(merged).map(([k, v]) => ({ k, v }));
          setEnv(arr);
                  }
                }}>
                <option value="">Custom (enter URL below)</option>
                {imagePresets.map((p) => (
                  <option value={p.value}>{p.label}</option>
                ))}
              </select>
            </label>
            <label class="block text-sm">Image URL<span class="text-red-600">*</span><Input value={image()} onInput={(e) => setImage(e.currentTarget.value)} placeholder="ghcr.io/org/app:tag" /></label>
            <label class="block text-sm">Name<Input value={name()} onInput={(e) => setName(e.currentTarget.value)} placeholder="optional" /></label>
            <label class="block text-sm">Args
              <div class="space-y-2">
                {args().map((a, i) => (
                  <div class="flex gap-2"><Input value={a} onInput={(e)=> setArgs(args().with(i, e.currentTarget.value))} /><button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setArgs(args().filter((_,idx)=>idx!==i))}>Remove</button></div>
                ))}
                <button class="btn" onClick={()=> setArgs([...args(), ''])}>Add arg</button>
              </div>
            </label>
            <div class="grid grid-cols-2 gap-2">
              <label class="block text-sm">CPU<Input value={cpu()} onInput={(e)=> setCpu(e.currentTarget.value)} placeholder="500m" /></label>
              <label class="block text-sm">Memory<Input value={memory()} onInput={(e)=> setMemory(e.currentTarget.value)} placeholder="256Mi" /></label>
            </div>
          </div>
          <div class="space-y-3">
      <label class="block text-sm">Env
              <div class="space-y-2">
                {env().map((p, i) => (
                  <div class="flex gap-2">
                    <Input placeholder="KEY" value={p.k} onInput={(e)=> setEnv(env().with(i, { ...p, k: e.currentTarget.value }))} />
                    <Input placeholder="value" value={p.v} onInput={(e)=> setEnv(env().with(i, { ...p, v: e.currentTarget.value }))} />
                    <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setEnv(env().filter((_,idx)=>idx!==i))}>Remove</button>
                  </div>
                ))}
                <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setEnv([...env(), {k:'',v:''}])}>Add env</button>
                <div class="text-xs text-neutral-500">Tip: Image defaults prefill suggested env and ports; you can override as needed. For IDE, AGENT_HOST should be the agent's reachable DNS or IP (e.g., service name).</div>
              </div>
            </label>
            <label class="block text-sm">Labels
              <div class="space-y-2">
                {labels().map((p, i) => (
                  <div class="flex gap-2">
                    <Input placeholder="key" value={p.k} onInput={(e)=> setLabels(labels().with(i, { ...p, k: e.currentTarget.value }))} />
                    <Input placeholder="value" value={p.v} onInput={(e)=> setLabels(labels().with(i, { ...p, v: e.currentTarget.value }))} />
                    <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setLabels(labels().filter((_,idx)=>idx!==i))}>Remove</button>
                  </div>
                ))}
                <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setLabels([...labels(), {k:'',v:''}])}>Add label</button>
              </div>
            </label>
            <label class="block text-sm">Ports
              <div class="space-y-2">
                {ports().map((p, i) => (
                  <div class="flex gap-2">
                    <Input placeholder="name" value={p.name} onInput={(e)=> setPorts(ports().with(i, { ...p, name: e.currentTarget.value }))} />
                    <Input placeholder="port" type="number" value={String(p.port)} onInput={(e)=> setPorts(ports().with(i, { ...p, port: Number(e.currentTarget.value) }))} />
                    <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setPorts(ports().filter((_,idx)=>idx!==i))}>Remove</button>
                  </div>
                ))}
                <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" onClick={()=> setPorts([...ports(), {name:'', port: 0}])}>Add port</button>
              </div>
            </label>
          </div>
        </div>
        {errors().length > 0 && (
          <div class="text-sm text-red-600 mt-2">
            {errors().map((e) => <div>{e}</div>)}
          </div>
        )}
        <div class="mt-4">
          <button class="inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700" disabled={busy()} onClick={submit}>{busy() ? 'Submitting…' : 'Launch'}</button>
        </div>
      </Card>
    </div>
  );
}
