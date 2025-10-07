# GuildNet Agent Image

A lightweight container that runs VS Code in the browser (code-server) behind Caddy with iframe-friendly headers. Exposes a single configurable HTTP port (default 8080). Includes a tiny landing page and a /healthz endpoint.

- Non-root user `app` (uid 10001)
- code-server bound to loopback, proxied by Caddy
- Iframe embeddable: `Content-Security-Policy: frame-ancestors 'self' *` and `X-Frame-Options` removed
- Single port via `$PORT` (default 8080)
- Volumes: `/workspace` for projects, `/data` for settings, extensions, and password persistence

## Build

```bash
docker build -t guildnet/agent:dev images/agent
```

## Run locally

```bash
mkdir -p workspace data

docker run --rm \
  -p 8080:8080 \
  -e PASSWORD=changeme \
  -v "$(pwd)/workspace:/workspace" \
  -v "$(pwd)/data:/data" \
  guildnet/agent:dev
```

Open http://localhost:8080 to access code-server. Health at http://localhost:8080/healthz.

**Default port:** The agent always exposes code-server via Caddy on port 8080 (HTTP, iframe-friendly). For most use cases, only 8080 needs to be mapped/exposed. Advanced users can set the PORT env var to change this.

**Kubernetes:** The example manifest exposes both 8080 and 8443, but only 8080 is required for the UI to work out-of-the-box. Set AGENT_HOST to the pod/service DNS name for best results.

If `PASSWORD` is not set, the container will generate one on first run and store it at `/data/.code-server-password` (printed once at startup). Persist `/data` to keep the password, user settings, and extensions.

## Kubernetes example

See `k8s/agent-example.yaml` for a minimal Deployment + Service with liveness/readiness probes and volumes.

## Iframe embedding

The embedded Caddy reverse proxy removes `X-Frame-Options` and sets `Content-Security-Policy: frame-ancestors 'self' *`, allowing your UI to embed the service in an `<iframe>`. Ensure your outer site uses same-origin or terminates TLS at an ingress/gateway as appropriate.

## Security notes

- Always set `PASSWORD` or mount `/data` to persist the generated password.
- The container runs as non-root and binds code-server to `127.0.0.1`; only Caddy listens on `$PORT`.
- No admin endpoints are exposed and directory listings are disabled for health path.
