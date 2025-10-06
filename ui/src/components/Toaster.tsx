import { For, Show, createSignal } from 'solid-js';

type Toast = { id: number; type: 'info' | 'error' | 'success'; message: string };

let pushFn: ((t: Omit<Toast, 'id'>) => void) | null = null;
export function pushToast(t: Omit<Toast, 'id'>) { pushFn?.(t); }

export default function Toaster() {
  const [toasts, setToasts] = createSignal<Toast[]>([]);
  pushFn = (t) => {
    const id = Date.now() + Math.random();
    setToasts((prev) => [...prev, { ...t, id }]);
    setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== id)), 4000);
  };
  const color = (t: Toast['type']) => t === 'error' ? 'bg-red-600' : t === 'success' ? 'bg-green-600' : 'bg-neutral-800';
  return (
    <div aria-live="polite" class="fixed bottom-4 right-4 flex flex-col gap-2 z-50">
      <For each={toasts()}>{(t) => (
        <div class={`text-white rounded shadow px-3 py-2 ${color(t.type)}`}>{t.message}</div>
      )}</For>
      <Show when={toasts().length === 0}><div class="sr-only">No notifications</div></Show>
    </div>
  );
}
