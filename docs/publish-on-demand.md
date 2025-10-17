Publish-on-demand port-forwarding and tsnet publish
=================================================

This notes the simple, short-term "publish-on-demand" approach implemented in the codebase.

Overview
--------
- When a request arrives for `/api/cluster/{id}/proxy/server/{name}/...`, the server will:
  1. Try the kube API service proxy path.
  2. If endpoints are missing or service proxy is unsuitable, attempt a Pod port-forward.
  3. If a port-forward is created, publish the forwarded local port into the tailnet via tsnet.Listen.
  4. Cache the published listener in-memory so subsequent requests reuse it.

Security considerations
-----------------------
- Publishing is powerful: only enable it for trusted clusters and users. Limit it with per-cluster setting and audit logs.
- Consider using tailscale ACLs or a short-lived token to restrict access to published endpoints.

Limitations
-----------
- Current implementation uses an in-memory cache. Restarting hostapp loses published mappings.
- Long-term solution should adopt an agent-based service registration model and persistent registry storage.
