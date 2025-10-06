import { For, JSX } from 'solid-js';

export default function Table(props: { headers: string[]; rows: JSX.Element[] }) {
  return (
    <div class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead class="text-left border-b">
          <tr>
            <For each={props.headers}>{(h: string) => <th class="py-2 pr-4 font-semibold whitespace-nowrap">{h}</th>}</For>
          </tr>
        </thead>
        <tbody>
          <For each={props.rows}>{(r: JSX.Element) => r}</For>
        </tbody>
      </table>
    </div>
  );
}
