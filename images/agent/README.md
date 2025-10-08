# GuildNet Agent Image

A lightweight container that runs VS Code in the browser (code-server) behind Caddy with iframe-friendly headers. Exposes HTTP on 8080 and HTTPS on 8443 (enabled by default with a self-signed cert). Includes a /healthz endpoint.

- Non-root user `app` (uid 10001)
- code-server bound to loopback, proxied by Caddy
- Iframe embeddable: `Content-Security-Policy: frame-ancestors 'self' *` and `X-Frame-Options` removed
- Single HTTP port via `$PORT` (default 8080), HTTPS via `$HTTPS_PORT` (default 8443, enabled by default)
- Volumes: `/workspace` for projects, `/data` for settings, extensions, and password persistence
- TLS: provide cert/key or enable self-signed via env (see below)

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

Open https://localhost:8443 (self-signed) or http://localhost:8080 to access code-server. Health at http(s)://localhost:8443/healthz.

**Default ports:** The agent exposes code-server via Caddy on port 8080 (HTTP) and 8443 (HTTPS). HTTPS is enabled by default with a self-signed certificate. Set `PORT` and `HTTPS_PORT` to change these.

**Kubernetes:** The example manifest exposes both 8080 and 8443, but only 8080 is required for the UI to work out-of-the-box. Set AGENT_HOST to the pod/service DNS name for best results.

Auth modes:

- `AGENT_AUTH=password` (default): code-server enforces password auth. Set `PASSWORD` or the agent will generate one and store it at `/data/.code-server-password`.
- `AGENT_AUTH=none`: no auth at code-server layer (use only in trusted/private networks).

Persist `/data` to keep the password, user settings, and extensions.

TLS:

- HTTPS is on by default using a self-signed cert persisted under `/data/tls`.
- Provide `AGENT_TLS_CERT` and `AGENT_TLS_KEY` to use your own certs.
- Set `AGENT_TLS_DISABLE=1` to turn HTTPS off.

## Kubernetes example

See `k8s/agent-example.yaml` for a minimal Deployment + Service with liveness/readiness probes and volumes. Exposes both 8080 and 8443 by default.

## Iframe embedding

The embedded Caddy reverse proxy removes `X-Frame-Options` and sets `Content-Security-Policy: frame-ancestors 'self' *`, allowing your UI to embed the service in an `<iframe>`. Ensure your outer site uses same-origin or terminates TLS at an ingress/gateway as appropriate. The agent routes directly to code-server.

## Verify externally

- Port check (inside cluster with LoadBalancer IP): `curl -k https://<lb-ip>:8443/healthz`
- Browser: Open `https://<lb-ip>:8443/` and accept the self-signed certificate once.

## Security notes

- Always set `PASSWORD` or mount `/data` to persist the generated password.
- The container runs as non-root and binds code-server to `127.0.0.1`; only Caddy listens on `$PORT`.
- No admin endpoints are exposed and directory listings are disabled for health path.
