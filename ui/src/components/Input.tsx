import { JSX, splitProps } from 'solid-js'

export default function Input(
  props: JSX.InputHTMLAttributes<HTMLInputElement>
) {
  const [local, rest] = splitProps(props, ['class'])
  return (
    <input
      class={`w-full rounded-md border px-3 py-2 bg-white dark:bg-neutral-900 ${local.class ?? ''}`}
      {...rest}
    />
  )
}
