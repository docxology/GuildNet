import { For, JSX, createEffect, createSignal } from 'solid-js'

export function Tabs(props: {
  tabs: { id: string; label: string }[]
  value?: string
  onChange?: (id: string) => void
}) {
  const [val, setVal] = createSignal(props.value ?? props.tabs[0]?.id ?? '')
  // Mirror external value when provided
  createEffect(() => {
    if (props.value !== undefined) setVal(props.value)
  })
  // Guard against removed/changed tabs
  createEffect(() => {
    const ids = props.tabs.map((t) => t.id)
    if (!ids.includes(val())) setVal(props.tabs[0]?.id ?? '')
  })
  const change = (id: string) => {
    setVal(id)
    props.onChange?.(id)
  }
  return (
    <div>
      <div role="tablist" class="flex gap-2 mb-3 border-b">
        <For each={props.tabs}>
          {(t: { id: string; label: string }) => (
            <button
              type="button"
              role="tab"
              aria-selected={val() === t.id}
              class={`inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700 ${val() === t.id ? 'border-b-2 border-brand-600 text-brand-600' : ''}`}
              onClick={() => change(t.id)}
            >
              {t.label}
            </button>
          )}
        </For>
      </div>
    </div>
  )
}

export function TabPanel(props: { when: boolean; children: JSX.Element }) {
  return props.when ? <div>{props.children}</div> : null
}
