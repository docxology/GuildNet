import { JSX, Show } from 'solid-js'

export default function Modal(props: {
  title?: string
  open: boolean
  onClose?: () => void
  children: JSX.Element
  footer?: JSX.Element
  maxWidthClass?: string
}) {
  return (
    <Show when={props.open}>
      <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto p-6">
        <div
          class="absolute inset-0 bg-black/40"
          onClick={() => props.onClose?.()}
        />
        <div
          class={`relative z-10 w-full ${props.maxWidthClass || 'max-w-lg'} rounded-lg border bg-white dark:bg-neutral-900 shadow-xl flex flex-col`}
        >
          <div class="px-4 py-3 border-b flex items-center justify-between">
            <h2 class="text-sm font-semibold">{props.title}</h2>
            <button
              class="text-neutral-400 hover:text-neutral-600"
              onClick={() => props.onClose?.()}
            >
              ✕
            </button>
          </div>
          <div class="p-4 space-y-4 max-h-[65vh] overflow-y-auto">
            {props.children}
          </div>
          {props.footer && (
            <div class="px-4 py-3 border-t flex justify-end gap-2">
              {props.footer}
            </div>
          )}
        </div>
      </div>
    </Show>
  )
}
