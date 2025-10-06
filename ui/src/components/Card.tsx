import { JSX } from 'solid-js';

export default function Card(props: { title?: string; children?: JSX.Element; actions?: JSX.Element; class?: string }) {
  return (
  <section class={`bg-white dark:bg-neutral-800 rounded-lg border shadow-sm p-4 ${props.class ?? ''}`}>
      {(props.title || props.actions) && (
        <header class="flex items-center justify-between mb-2">
          <h2 class="text-sm font-semibold">{props.title}</h2>
          <div>{props.actions}</div>
        </header>
      )}
      <div>{props.children}</div>
    </section>
  );
}
