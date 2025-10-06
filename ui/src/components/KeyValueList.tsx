export default function KeyValueList(props: { data?: Record<string, string> }) {
  const entries = Object.entries(props.data ?? {});
  if (!entries.length) return <div class="text-sm text-neutral-500">None</div>;
  return (
    <dl class="grid grid-cols-1 sm:grid-cols-2 gap-2">
      {entries.map(([k, v]) => (
        <div class="flex gap-2" role="group">
          <dt class="text-xs text-neutral-500 w-24 shrink-0">{k}</dt>
          <dd class="text-sm break-all">{v}</dd>
        </div>
      ))}
    </dl>
  );
}
