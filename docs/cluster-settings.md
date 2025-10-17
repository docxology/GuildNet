Cluster-level workspace exposure setting

This document explains the per-cluster setting that controls the operator's default exposure
mode for Workspaces when a Workspace has no explicit `exposure` section.

Key concept
- The cluster-level setting is persisted by the Host App and written into the target
  Kubernetes cluster as a ConfigMap named `guildnet-cluster-settings` in the
  `guildnet-system` namespace.
- The ConfigMap data key is `workspace_lb_enabled` with a value of `"true"` or
  `"false"` (strings). The Host App converts its internal boolean into this
  string and applies the ConfigMap whenever a cluster's settings are updated.

Operator behavior
- The operator prefers to read the in-cluster ConfigMap `guildnet-cluster-settings`
  and uses `workspace_lb_enabled` to determine whether Workspaces without an
  explicit exposure should be created as `ServiceType=LoadBalancer`.
- If the ConfigMap is not present or does not contain the key, the operator falls
  back to the environment variable `WORKSPACE_LB_DEFAULT` (values `1`, `true`,
  or `yes` are considered true).
- To reduce API load, the operator caches the current value and updates the cache
  whenever the ConfigMap changes. When the flag flips, the operator triggers a
  reconcile of existing Workspaces so they can be converted if appropriate.

How to change the setting manually

- To set load-balancer-by-default for a cluster named "my-cluster":

```bash
kubectl -n guildnet-system create configmap guildnet-cluster-settings \
  --from-literal=workspace_lb_enabled=true --dry-run=client -o yaml | kubectl apply -f -
```

- To disable:

```bash
kubectl -n guildnet-system create configmap guildnet-cluster-settings \
  --from-literal=workspace_lb_enabled=false --dry-run=client -o yaml | kubectl apply -f -
```

Host App
- The Host App exposes the cluster settings API at `PUT /api/settings/cluster/:id`.
  When the cluster settings (the `workspace_lb_enabled` boolean) are updated via
  the API, the Host App writes/updates the ConfigMap in the target cluster so the
  operator can consume it.

Notes and caveats
- Changing the default affects Workspaces that are created after the change and
  can also cause existing Workspaces to be reconciled; whether an existing
  Service is converted depends on cluster policies and LoadBalancer availability
  (and may require manual exposure updates for in-use workloads).
- In kind/test environments, MetalLB should be installed and configured to allocate
  IPs for LoadBalancer Services.
