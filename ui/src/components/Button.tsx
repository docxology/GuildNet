import { JSX } from 'solid-js'

export default function Button(
  props: JSX.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: 'primary' | 'default'
  }
) {
  const variant =
    props.variant === 'primary'
      ? 'inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-brand-600 text-white hover:bg-brand-700 border-brand-700'
      : 'inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium border bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700'
  const { class: klass, ...rest } = props as any
  return (
    <button class={`${variant} ${klass ?? ''}`} {...(rest as any)}>
      {props.children}
    </button>
  )
}
