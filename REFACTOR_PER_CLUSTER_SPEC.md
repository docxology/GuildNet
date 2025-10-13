# REFACTOR_PER_CLUSTER_SPEC.md

Status: Draft
Date: 2025-10-13

## Overview

This document specifies the implementation plan, requirements, tests and verification steps for migrating global host-wide services and state in GuildNet to be per-cluster scoped. The goals are to improve multi-cluster isolation, reliability, onboarding, and to remove contention (DB locks, global k8s clients, port-forward clashing, proxy fallbacks) observed in the current architecture.

This specification uses RFC 2119 language (MUST, MUST NOT, SHOULD, SHOULD NOT, MAY) for requirements.

## Scope

This refactor applies to all hostapp and backend components that currently hold global singletons or host-wide state which should instead be created/owned per target Kubernetes cluster. At minimum, the following areas/components are in-scope:

- DB managers and local storage used by hostapp (localdb/sqlite, any cache files, and in-memory global caches tied to cluster state)
- RethinkDB connectors and per-cluster discovery/connection logic
- Kubernetes API clients and informers
- Port-forward managers and active port-forward sessions
- Proxy and API routing/fallback logic (k8s API proxy, kubectl proxy fallbacks, Tailscale/Headscale route fallbacks)
- Settings and configuration objects that vary by cluster (per-cluster settings, sync of .env/config.json/guildnet.config)
- Metrics collectors and registries that are cluster-scoped (avoid global metric collisions)
- Any background workers or goroutines that handle cluster events (watchers, changefeed processors)

Out of scope for this document (but should be reviewed during implementation):

- Node-level or OS-level utilities that must remain global (system certificate handling, privileged host scripts invoked outside hostapp scope, global agent images)
- UI static assets and purely client-side UI logic

## Definitions

- cluster: A logical Kubernetes cluster as identified by unique cluster ID / kubeconfig context / API endpoint.
- per-cluster instance: An instance of a service (DB manager, k8s client, port-forward manager, etc.) created and owned for a single cluster.
- host-wide/global instance: A single instance shared across multiple clusters or the whole host.

## High-level Principles

- Principle 1: Isolation. Components managing cluster-specific resources or data MUST be isolated per cluster so a failure/lock/resource leak in one cluster does not impact other clusters.
- Principle 2: Lifecycle tied to cluster registration. Per-cluster instances MUST be created when a cluster is registered/added and MUST be destroyed when the cluster is explicitly removed/unregistered (or after a configured expiry/garbage collection window if soft-delete is used).
- Principle 3: Consistency and determinism. APIs used by other packages MUST present a stable interface; migrating to per-cluster scope MUST preserve existing semantics except where noted and documented.
- Principle 4: Safe migration. Data migrations and topology changes MUST be reversible; a clear rollback plan MUST exist.

## MUST / SHOULD / MAY Requirements

### Global Requirements (applies to all components)

1. MUST ensure that for each recognized cluster there exists a distinct namespace of runtime objects (DB manager, k8s client, port-forward manager, proxy route entries, metric registries).
2. MUST NOT allow cross-cluster mutation of cluster-specific data without explicit, auditable API that declares cross-cluster intent.
3. MUST present a thread-safe, concurrency-safe factory mechanism to obtain per-cluster instances using the cluster identifier as the key.
4. SHOULD provide caching of per-cluster instances to avoid repeated expensive creation, with TTLs and LRU/GC semantics documented.
5. MUST provide deterministic teardown behavior for per-cluster instances (close DB handles, cancel goroutines, terminate port-forward sessions, deregister metrics) upon cluster removal.
6. MUST log lifecycle events (create/start/stop/destroy) for per-cluster instances at INFO level and errors at ERROR level.

### DB Manager and LocalDB (sqlite)

1. MUST create a per-cluster DB manager that opens a distinct SQLite database path per cluster (e.g. stateDir/<cluster-id>/local.db) rather than a single host-wide DB file.
2. MUST ensure that SQLite connections are not shared across clusters and that connection open/close is coordinated with the DB manager lifecycle.
3. MUST implement retry/backoff and health-check semantics for the DB manager's open operation; failures MUST be reported and not retried indefinitely without backoff.
4. SHOULD support an optional read-only global pooled DB for purely host-global state (if absolutely necessary), but this MUST be rare and documented.
5. MUST provide a migration mechanism to move data from the existing global DB layout to the per-cluster layout (migration tooling and tests are required). See Migration Plan section.
6. MUST NOT leave stale file locks or processes holding DB files when teardown occurs; file lock cleanup logic MUST be in the teardown path and tests MUST verify no stale locks remain.

### RethinkDB Connectors

1. MUST maintain per-cluster RethinkDB connectors (discovery + connection pool) aligned with cluster-specific RethinkDB service location (in-cluster service discovery via cluster kubeconfig + env overrides).
2. MUST NOT use a single global RethinkDB connection for multiple clusters.
3. SHOULD lazy-init connectors on first use, and SHOULD provide an explicit warm-up API used by readiness probes when desirable.
4. MUST provide clear error classification for connector errors (unreachable, auth failure, schema missing) so callers can decide transient vs. fatal.

### Kubernetes API Client / Informers

1. MUST instantiate per-cluster Kubernetes clients and informers; clients MUST use cluster-scoped kubeconfig/context and have independent rate-limiting and error handling.
2. MUST NOT use a global shared informer factory to watch multiple clusters.
3. SHOULD reuse code paths that create clients via a central factory function to keep creation logic consistent.
4. MUST ensure that port-forward and proxy operations always reference the per-cluster client used to establish the connection.

### Port-forward Manager

1. MUST be scoped to cluster; a port-forward manager instance MUST only manage forwards for pods/services in that cluster.
2. MUST ensure forwards are uniquely identified by cluster + namespace + pod/service + localPort so collisions across clusters do not occur.
3. MUST support programmatic teardown and graceful shutdown of active forwards during cluster removal.
4. SHOULD implement health-checks to detect stale dead forwards and auto-restart only within the cluster scope.

### Proxy Routing and API Fallback

1. MUST maintain per-cluster proxy routing tables and fallback preferences (e.g. direct API endpoint vs. kubectl-proxy vs. Tailscale route) stored in per-cluster settings.
2. MUST evaluate fallback modes per-cluster and prefer non-invasive fallbacks first (e.g., kubectl proxy) when direct routing fails.
3. SHOULD provide administrators a per-cluster override for routing strategy saved in cluster settings.
4. MUST NOT share fallback decisions or metrics across clusters without explicit cross-cluster aggregation logic.

### Settings, Config Synchronization

1. MUST store and load settings in a per-cluster scope. File-based settings (e.g., synced config.json, guildnet.config) MUST be namespaced per-cluster on disk.
2. MUST provide a deterministic sync path from host `.env` and global `config.json` into the per-cluster settings store; conflicts MUST be resolved by a documented precedence (CLI/env > cluster-config > global-config by default).
3. SHOULD provide tools/scripts to bootstrap per-cluster settings when onboarding a new cluster.

### Metrics and Observability

1. MUST provide per-cluster metric registries or prefix metric names with the cluster id to avoid metric collisions in a single Prometheus scrape target.
2. SHOULD provide cluster label on all exported metrics and logs to enable filtering.

### Background Workers and Changefeeds

1. MUST scope any background goroutine or changefeed subscription to the cluster and ensure its lifecycle is tied to the per-cluster manager.
2. MUST provide a safe cancellation pathway (context cancellation) to stop changefeeds and watchers when cluster instance is torn down.
3. SHOULD isolate errant or hot changefeeds to avoid resource exhaustion on the host (e.g., limit concurrent feeds per cluster; provide rate limiting).

## Implementation Plan (step-by-step)

NOTE: implement iteratively and keep all changes behind feature flags or gated by an integration test harness where possible.

1. Preparation & scaffolding
   - Add a per-cluster factory pattern and registry.
     - Implement `internal/cluster/registry.go` (or similar) providing thread-safe Get(clusterID) -> Instance factory, Create, Close functions.
     - Instances SHOULD implement an interface that exposes lifecycle methods: Start(), Stop(ctx), Status(), ID().
   - Add a cluster identifier abstraction (type ClusterID string) and a canonical hashing/normalization function for filesystem paths and registry keys.

2. DB manager migration
   - Implement `internal/localdb/manager.go` that is cluster-aware and holds the SQLite open/close logic for a single cluster path (stateDir/clusterID/local.db).
   - Wire callers of the old global localdb package to obtain a per-cluster DB manager via the registry.
   - Add unit tests for DB manager: open/close, concurrent gets, migrations, lock behavior.
   - Implement migration tooling: CLI command or function to move existing global DB to per-cluster layout for a target cluster (with dry-run and backup options).

3. Kubernetes client/informer migration
   - Implement per-cluster k8s client factory `internal/k8s/factory.go` returning clientset, rest.Config, and informer factories scoped to the cluster.
   - Replace global client usage sites with registry-backed per-cluster clients.
   - Add integration tests with kind/k3s test clusters or mocked client to verify informers and port-forwarding behavior.

4. Port-forward manager
   - Refactor `internal/k8s/portforward.go` to be instanciated per-cluster and to register forwards with cluster-scoped identifier.
   - Ensure active forwards are tracked on the cluster instance and torn down on Stop().
   - Add unit tests simulating start/stop, duplicate forward names, and teardown on cluster removal.

5. Proxy and fallback routing
   - Move per-cluster routing tables and fallback policies into `internal/settings` per-cluster settings.
   - Ensure `internal/api/router.go` uses the per-cluster proxy and health logic.
   - Add tests for fallback ordering: direct -> kubectl proxy -> tailscale route/proxy.

6. RethinkDB connectors and usage
   - Ensure per-cluster RethinkDB discovery runs using the cluster k8s client (service discovery) and that connectors are stored on the cluster instance.
   - Add connector lifecycle tests and error classification tests.

7. Metrics and logging
   - Ensure any metric registry created by the hostapp can be created per-cluster or annotated by cluster label.
   - Centralize log context to include cluster ID in per-cluster instances.

8. Background workers / changefeed subscriptions
   - Migrate any global changefeed/watchers to be created by per-cluster managers; ensure cancellation via context.
   - Add integration tests that create a cluster instance, subscribe to a feed, then tear down the instance and verify feed termination and resource release.

9. Gradual migration strategy
   - Implement a feature-flag early phase: create per-cluster factories but keep default behavior pointing to global instances. Add extensive logging when per-cluster factories are invoked.
   - Phase 1: DB managers and k8s clients switched to per-cluster as these have highest impact.
   - Phase 2: Port-forward, proxy, settings and metrics.
   - Phase 3: Background workers and changefeeds.

10. Cleanup & deprecation
   - After full migration and verification, deprecate global singletons and remove legacy code paths.

## Testing and Verification Plan

Testing MUST be multi-layered: unit tests, integration tests (in-process simulated clusters), and E2E tests (using local kind/k3s or CI-provided clusters). All tests MUST include failure and teardown verification.

1. Unit tests (fast, deterministic)
   - Test per-cluster registry basic operations (create, get, close), concurrency (parallel Get/Create), and memory/GC semantics.
   - DB manager unit tests: open/close, concurrent transactions, migration tooling with sample data, file lock tests that simulate stale lock conditions.
   - Port-forward unit tests: ensure unique forwarding keys, teardown semantics.
   - RethinkDB connector unit tests: validate error classification, lazy init, and mock reconnection behavior.

2. Integration tests (mock or lightweight clusters)
   - Use `envtest`, `client-go` fake clients, or lightweight k3s/kind clusters spawned in test harness to verify real client interactions, informer behavior, and port-forwarding using ephemeral pods.
   - Test cluster lifecycle: create cluster instance, perform operations (DB writes, port-forward, watch resources), then tear down and assert no goroutines leak and no file locks remain.

3. E2E tests (CI or local cluster(s))
   - Multi-cluster scenario: start 2+ clusters (kind/k3s), register them with hostapp, exercise actions in each cluster in parallel and assert isolation (writes in cluster A do not appear in cluster B local DB; port-forwards do not collide; metrics have cluster labels).
   - Important stress tests: open many changefeeds across clusters, create/destroy clusters repeatedly to detect resource leakage.

4. Regression tests
   - Re-run existing test suite (tests/ folder) and ensure all pass.
   - Add tests to reproduce past issues: BoltDB file locks, global-k8s-client timeouts, cross-cluster proxy leak.

5. Test tooling and CI
   - Add CI jobs that run the integration and E2E harnesses on PRs touching the refactor areas.
   - Provide a local developer script (`scripts/test-per-cluster.sh`) to run a curated subset of integration tests using kind/k3s.

## Migration Plan (Data & Operational)

1. Preparation
   - Add `scripts/migrate-global-db-to-per-cluster.sh` (or internal CLI `hostapp migrate-db --cluster <id> --backup <path> --dry-run`) that can move global DB contents into a new per-cluster DB layout.
   - Migration MUST support dry-run, backup of original global DB, and a deterministic mapping of keys -> cluster buckets. The mapping may be provided by the operator as a CSV or inferred by objective rules; inference MUST be tested and transparent.

2. Phased migration
   - Phase 0: Create per-cluster DB manager and mount an empty DB for new clusters only; do not alter existing global DB.
   - Phase 1: For a selected pilot cluster, run migration tool with `--dry-run` then `--commit` and validate application behavior and tests.
   - Phase 2: Gradually migrate other clusters during a maintenance window.

3. Validation after migration
   - Verify hostapp can open per-cluster DB and that writes/reads behave as before.
   - Run the integration/E2E smoke tests on the migrated cluster.

4. Rollback
   - If problems occur, rollback MUST be supported by:
     - Restoring backup DB files to the original global DB path.
     - Restarting hostapp in legacy mode (feature-flag to use global DB) to resume service.
   - Rollback steps MUST be automated and documented in `docs/migration-rollback.md`.

## Rollback and Safety

1. All changes MUST be behind a feature toggle or guarded by a configuration flag during the initial rollout.
2. Migration tools MUST not delete source data without explicit `--commit` confirmation.
3. All migrations MUST produce an audit log with files/operations performed, timestamps, and the operator who ran them.
4. Tests validating rollback MUST be part of the integration test suite.

## Observability, Monitoring, and Alerts

1. Logging
   - Per-cluster lifecycle events MUST be logged with cluster id and a stable event code.
   - Errors that cause cluster instance termination MUST be logged with stack traces and remediation hints.

2. Metrics
   - Export per-cluster metrics with label `cluster_id`. Metric names MUST avoid collisions (prefix with subsystem when appropriate).
   - Provide metrics for: active clusters, per-cluster DB open count, active port-forwards per cluster, active changefeeds per cluster, per-cluster k8s API error rate.

3. Alerts
   - Create alert rules for: rapid creation/destruction of clusters (possible misbehaving consumer), high DB error rates for a single cluster, port-forward failures ramp, leaked goroutines or file descriptors per host.

4. Dashboards
   - Provide a dashboard showing cluster instances, resource usage per cluster, and health checks.

## Acceptance Criteria

The refactor will be considered complete when all of the following are true:

1. Functional correctness
   - Hostapp can manage and operate on multiple clusters concurrently without cross-cluster leakage.
   - DB reads/writes operate correctly per-cluster and existing APIs behave as before.

2. Reliability
   - Known previous failure modes (BoltDB lock, global-client timeouts) are resolved under normal and stress tests.

3. Observability
   - Per-cluster lifecycle events, metrics, and logs are present and searchable.

4. Tests
   - Unit, integration and E2E tests described above pass in CI for PRs that touch refactor areas.

5. Rollback
   - Migration tooling supports reversible migration with documented rollback steps.

6. Performance
   - No significant regressions in hostapp startup time or memory/FD usage relative to baseline (a documented performance baseline should be captured prior to migration).

## Rollout Plan and Timeline (suggested)

- Week 1: Scaffolding and registry implementation; DB manager refactor and unit tests.
- Week 2: K8s client factory; port-forward refactor; unit/integration tests.
- Week 3: Proxy, settings migration; RethinkDB connector adjustment; integration tests.
- Week 4: Background workers and changefeeds; E2E tests; migration tooling.
- Week 5: Pilot migration, monitoring/dashboards, CI stabilization.
- Week 6: Broader rollout, cleanup, deprecations, docs.

Adjust timeline based on available engineering resources and blockers. Use feature-flagged rollout to reduce blast radius.

## Developer Guidance & Patterns

1. Factories and interfaces
   - Expose creation via interfaces: e.g. ClusterManagerFactory.Create(clusterID string) -> ClusterManager.
   - Avoid leaking internal state; prefer small interfaces for testability.

2. Filesystem layout
   - Use stateDir/<cluster-id>/ for any per-cluster persisted files. Normalize cluster-id into filesystem safe names.

3. Concurrency
   - Registry MUST be safe for concurrent Get/Create/Close operations. Prefer sync.RWMutex or sync.Map with careful teardown semantics.

4. Testing
   - Write table-driven tests for error paths. Use race detector (go test -race) regularly.

## Security & Access Control

1. Credentials for per-cluster clients (kubeconfigs, secrets) MUST be stored with appropriate permissions and not leak into logs.
2. Any migration or cluster management operation that affects cluster data MUST require explicit admin consent and auditing.

## Open Questions

1. How to map existing global DB records to clusters when records are not already cluster-tagged? (Requires operator guidance, heuristics, or interactive mapping tool.)
2. Are there any host-wide features that truly require shared state across clusters? If so, define minimal shared interface and central read-only data store.
3. What is the expected scale of clusters per host? This affects resource budgets and limits for concurrent port-forwards/changefeeds.

## Checklist (Developer Ready)

- [ ] Implement `internal/cluster/registry.go` with Get/Create/Close semantics.
- [ ] Implement per-cluster localdb manager and migration tool.
- [ ] Implement per-cluster k8s client factory.
- [ ] Refactor port-forward and proxy code to use per-cluster instances.
- [ ] Migrate RethinkDB connectors to cluster scope.
- [ ] Add unit/integration/E2E tests and CI jobs.
- [ ] Add migration scripts and rollback docs.
- [ ] Provide dashboards and alerts for per-cluster metrics.



-- End of spec --


