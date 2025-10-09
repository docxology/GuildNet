import { JSX, splitProps } from 'solid-js'

export default function Button(
  props: JSX.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: 'primary' | 'default'
  }
) {
  const [local, rest] = splitProps(props, ['class', 'variant', 'children'])
  const base =
    'inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border disabled:opacity-50 disabled:pointer-events-none'
  const variantClass =
    local.variant === 'primary'
      ? `${base} text-white bg-sky-600 hover:bg-sky-700 border-sky-700 bg-brand-600 hover:bg-brand-700 border-brand-700`
      : `${base} text-neutral-900 dark:text-white bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700 border-neutral-200 dark:border-neutral-700`
  return (
    <button class={`${variantClass} ${local.class ?? ''}`} {...rest}>
      {local.children}
    </button>
  )
}
