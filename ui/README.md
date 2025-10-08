# GuildNet UI

A simple SolidJS + Vite + Tailwind web UI to browse servers, stream logs, and launch new workloads.

## Scripts

- npm run dev — start dev server
- npm run build — production build
- npm run preview — preview build
- npm run lint — eslint
- npm run format — prettier

## Backend contract (same-origin)

- GET /api/servers
- GET /api/servers/:id
- WS /ws/logs?target=...&level=info|debug|error&tail=200
- GET /api/servers/:id/logs?level=...&since=...&until=...&limit=...
- POST /api/jobs
- WS /ws/events (optional)

No environment variables are used; all requests are relative.

## Dev notes

Open the app at https://127.0.0.1:8080 during local development. The Host App proxies the Vite dev server (which listens on https://localhost:5173) to provide a single HTTPS origin.

If your API runs on a different port during local dev, configure a Vite proxy in vite.config.ts.
