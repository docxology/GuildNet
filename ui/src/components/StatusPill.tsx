export default function StatusPill(props: { status?: string }) {
  const s = (props.status ?? '').toLowerCase()
  const map: Record<string, string> = {
    running:
      'bg-green-100 text-green-700 border-green-200 dark:bg-green-900/20 dark:text-green-300',
    pending:
      'bg-amber-100 text-amber-700 border-amber-200 dark:bg-amber-900/20 dark:text-amber-300',
    failed:
      'bg-red-100 text-red-700 border-red-200 dark:bg-red-900/20 dark:text-red-300',
    stopped:
      'bg-neutral-100 text-neutral-700 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-300'
  }
  const cls =
    map[s] ??
    'bg-neutral-100 text-neutral-700 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-300'
  return (
    <span class={`px-2 py-0.5 rounded-full text-xs border ${cls}`}>
      {props.status ?? 'unknown'}
    </span>
  )
}
