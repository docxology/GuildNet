export default function StreamStatus(props: {
  state: string
  retry?: number
  backoffMs?: number
  error?: string
}) {
  const color =
    props.state === 'open'
      ? 'text-green-600'
      : props.state === 'reconnecting'
        ? 'text-amber-600'
        : props.state === 'error'
          ? 'text-red-600'
          : 'text-neutral-500'
  return (
    <div class={`text-xs ${color}`} aria-live="polite">
      WS: {props.state}
      {props.retry ? ` (#${props.retry})` : ''}
      {props.backoffMs
        ? ` backoff ${(props.backoffMs / 1000).toFixed(1)}s`
        : ''}
      {props.error ? ` ${props.error}` : ''}
    </div>
  )
}
