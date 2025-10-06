# GuildNet Host App (MVP)

A small Go service that exposes a local and Tailscale-backed HTTP interface to reach in-cluster services via a Tailscale subnet router. Uses tsnet (embedded tailscaled) and persists all state/config under `~/.guildnet/`.

## Features

- `tsnet.Server`-backed connectivity; no external tailscaled.
- Routes TCP via a Tailscale subnet router advertising Pod/Service CIDRs.
- Endpoints:
  - `GET /healthz` – liveness.
  - `GET /api/ping?addr=<host-or-ip>:<port>` – TCP dial RTT.
  - `GET /proxy?to=<ip:port>&path=/...` – minimal reverse proxy to in-cluster HTTP.
  - `WS /ws/echo` – WebSocket echo.
- Serves on both a local TCP address and on its Tailscale listener.
- JSON logging with request IDs, graceful shutdown.

## Prereqs

- A Headscale (or Tailscale) control server you can reach.
- A node acting as a Tailscale subnet router advertising your cluster's Pod/Service CIDRs.

## Install & Run

1. Build

```sh
make tidy
make build
```

2. First-time config

```sh
./bin/hostapp init
```

This creates `~/.guildnet/config.json` and `~/.guildnet/state/`.

3. Serve

```sh
./bin/hostapp serve
```

On startup, the app logs its Tailscale IP and MagicDNS name. It listens on the configured local address and on the tsnet listener.

## Usage

- Health:

```sh
curl http://127.0.0.1:8080/healthz
```

- Ping:

```sh
curl 'http://127.0.0.1:8080/api/ping?addr=10.96.0.1:443'
```

- Proxy:

```sh
curl 'http://127.0.0.1:8080/proxy?to=10.96.0.1:443&path=/'
```

- WebSocket echo:

Use `wscat` or browser to connect to `ws://127.0.0.1:8080/ws/echo`.

## Container

```sh
docker build -t guildnet/hostapp:dev .
# Mount ~/.guildnet to persist identity if desired
# docker run --rm -p 8080:8080 -v $HOME/.guildnet:/home/nonroot/.guildnet guildnet/hostapp:dev
```

## Paths

- Config: `~/.guildnet/config.json`
- State: `~/.guildnet/state/`

## Notes

- The app dials IPs directly reachable via Tailscale routes; it doesn't install anything in Kubernetes.
- Allowlist is required for `/api/ping` and `/proxy`. If empty, access is denied.
