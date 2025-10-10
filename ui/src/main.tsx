import { render } from 'solid-js/web'
import App from './App'
import './index.css'

// Lightweight client-side error reporting to help diagnose blank screens
function setupClientErrorReporting() {
	const post = (payload: any) => {
		try {
			navigator.sendBeacon(
				'/api/ui-error',
				new Blob([JSON.stringify(payload)], { type: 'application/json' })
			)
		} catch {
			// best-effort fallback
			try {
				fetch('/api/ui-error', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(payload)
				})
			} catch {}
		}
	}
	window.addEventListener('error', (ev) => {
		try {
			post({
				type: 'error',
				message: ev.message,
				filename: (ev as any).filename,
				lineno: (ev as any).lineno,
				colno: (ev as any).colno,
				time: new Date().toISOString()
			})
		} catch {}
	})
	window.addEventListener('unhandledrejection', (ev) => {
		try {
			post({
				type: 'unhandledrejection',
				reason:
					ev && typeof ev.reason === 'object'
						? { name: ev.reason?.name, message: ev.reason?.message }
						: String(ev?.reason ?? ''),
				time: new Date().toISOString()
			})
		} catch {}
	})
}

setupClientErrorReporting()

render(() => <App />, document.getElementById('root') as HTMLElement)
