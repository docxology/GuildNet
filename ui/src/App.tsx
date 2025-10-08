import { lazy, onMount } from 'solid-js'
import {
  A,
  Route,
  Router,
  useNavigate,
  type RouteSectionProps
} from '@solidjs/router'
import Toaster from './components/Toaster'

const Servers = lazy(() => import('./routes/Servers'))
const ServerDetail = lazy(() => import('./routes/ServerDetail'))
const Launch = lazy(() => import('./routes/Launch'))

function AppShell(props: RouteSectionProps) {
  const navigate = useNavigate()
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === '/' && (e.target as HTMLElement)?.tagName !== 'INPUT') {
        const el = document.getElementById('global-search')
        if (el) {
          e.preventDefault()
          ;(el as HTMLInputElement).focus()
        }
      }
      if (e.key === 'g') {
        let next = ''
        const onNext = (ev: KeyboardEvent) => {
          if (ev.key === 's') next = '/'
          if (ev.key === 'l') next = '/launch'
          if (next) {
            ev.preventDefault()
            window.removeEventListener('keydown', onNext, true)
            navigate(next)
          }
        }
        window.addEventListener('keydown', onNext, true)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  return (
    <div class="min-h-screen flex flex-col">
      <header class="border-b sticky top-0 z-10 bg-white/70 dark:bg-neutral-900/70 backdrop-blur">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex items-center gap-4 h-14">
          <A href="/" class="font-semibold">
            GuildNet
          </A>
          <nav class="flex items-center gap-3 text-sm">
            <A
              href="/"
              end
              activeClass="text-brand-600"
              class="hover:underline"
            >
              Servers
            </A>
            <A
              href="/launch"
              activeClass="text-brand-600"
              class="hover:underline"
            >
              Launch
            </A>
          </nav>
          <div class="ml-auto">
            <input
              id="global-search"
              placeholder="Search… (/ to focus)"
              class="w-64 rounded-md border px-3 py-2 bg-white dark:bg-neutral-900"
            />
          </div>
        </div>
      </header>
      <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 flex-1 w-full">
        {props.children}
      </main>
      <Toaster />
    </div>
  )
}

export default function App() {
  return (
    <Router>
      <Route path="/" component={AppShell}>
        <Route path="/" component={Servers} />
        <Route path="/servers/:id" component={ServerDetail} />
        <Route path="/launch" component={Launch} />
      </Route>
    </Router>
  )
}
