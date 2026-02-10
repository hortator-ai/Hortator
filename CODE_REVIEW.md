# Hortator Code Review

> Review date: 2026-02-10 (revision 2 — full re-review after recent changes)

---

## Table of Contents

- [Overview](#overview)
- [What Changed Since Last Review](#what-changed-since-last-review)
- [Component Breakdown](#component-breakdown)
  - [API Types](#1-api-types-apiv1alpha1)
  - [Operator Controller](#2-operator-controller-internalcontroller)
  - [CLI](#3-cli-cmdhortator)
  - [Gateway](#4-gateway-cmdgateway-internalgateway)
  - [Runtime](#5-runtime-runtime)
  - [SDKs](#6-sdks-sdk)
  - [Helm Chart](#7-helm-chart-chartshortator)
  - [CRDs](#8-crds-crds)
  - [CI/CD](#9-cicd-githubworkflows)
  - [Documentation](#10-documentation-docs)
- [What's Good](#whats-good)
  - [Architecture & Design](#architecture--design)
  - [Code Quality](#code-quality)
- [What Could Be Improved](#what-could-be-improved)
  - [Medium Issues](#medium-issues)
  - [Minor Issues / Polish](#minor-issues--polish)
- [Previously Reported Issues — Now Resolved](#previously-reported-issues--now-resolved)
- [Summary](#summary)

---

## Overview

Hortator is a Kubernetes operator that orchestrates AI agents in a hierarchical structure inspired by Roman military ranks (Tribune > Centurion > Legionary). It lets agents spawn sub-agents as Kubernetes Pods with isolation, budget controls, security guardrails, and lifecycle management.

---

## What Changed Since Last Review

Significant improvements were made across the board addressing most Critical and Moderate issues from the first review:

| Area | Change |
|------|--------|
| **Controller refactor** | Monolithic 1318-line `agenttask_controller.go` split into 7 focused files: `agenttask_controller.go` (701 lines), `helpers.go`, `metrics.go`, `pod_builder.go`, `policy.go`, `result_cache.go`, `warm_pool.go` |
| **ConfigMap caching** | `loadClusterDefaults` now cached with 30s TTL via `refreshDefaultsIfStale()` and `sync.RWMutex` — no longer hits K8s API every reconcile |
| **`MustParse` panics fixed** | All `resource.MustParse` calls replaced with `parseQuantity()` helper that returns clean errors |
| **Retry jitter added** | `computeBackoff()` now adds ±25% jitter to prevent thundering herd |
| **Init container pinned** | `busybox:latest` replaced with `busybox:1.37.0` |
| **Shell interpolation fixed** | Init container now uses `printf '%s' "$TASK_JSON"` via env var instead of shell string interpolation |
| **CLI `--role`/`--tier`/`--parent` added** | `spawn` command now sets `Role`, `Tier`, and `ParentTaskID` |
| **CLI terminal phases fixed** | `waitForTask` handles all phases: `BudgetExceeded`, `TimedOut`, `Cancelled`, `Retrying` |
| **CLI wait timeout** | `waitForTask` now uses `context.WithTimeout` via `--wait-timeout` flag (default 1h) |
| **Gateway auth caching** | `authenticate` now caches K8s Secret keys with 60s TTL |
| **License headers** | All files now consistently use MIT SPDX headers |
| **Makefile** | Now a proper kubebuilder Makefile with `make build`, `make test`, `make sync-crds`, `make verify-crds` |
| **CRD sync** | `make sync-crds` and `make verify-crds` keep CRDs in sync across `config/crd/bases/`, `crds/`, and `charts/hortator/crds/`. CI enforces sync. Removed stale `agentpolicy.yaml` and `agenttask.yaml` from `crds/` and `charts/` |
| **Warm Pod pool** | New opt-in feature for sub-second task assignment with pre-provisioned idle Pods |
| **Result cache** | New opt-in content-addressable cache with SHA-256 keying, LRU eviction, TTL expiry |
| **Test coverage** | Added `controller_unit_test.go`, `helpers_test.go`, `pod_builder_test.go`, `policy_test.go`, `result_cache_test.go`, `warm_pool_test.go`, `gateway_handler_test.go`, `gateway_test.go`, plus SDK tests |
| **Python SDK** | Sync/async client, SSE streaming, LangChain + CrewAI integrations, tests, examples |
| **TypeScript SDK** | Zero-dependency client, streaming, LangChain.js integration, tests |
| **Documentation** | Previously placeholder pages (`crds.md`, `storage.md`, `telemetry.md`) now have real content. New `warm-pool.md` design doc. |

---

## Component Breakdown

### 1. API Types (`api/v1alpha1/`)

**`agenttask_types.go`** — Core CRD type definitions for `AgentTask` and all supporting types (`ModelSpec`, `RetrySpec`, `BudgetSpec`, `StorageSpec`, `HealthSpec`, `PresidioSpec`, `EnvVar`, `ResourceRequirements`). The status struct tracks phase, output, token usage, child tasks, retry history, and timing.

**`agentpolicy_types.go`** — Defines `AgentPolicy` for namespace-scoped governance constraints (capability allow/deny lists, image restrictions, budget caps, concurrency limits, egress allowlists).

### 2. Operator Controller (`internal/controller/`)

Now split into 7 files with clear responsibilities:

| File | Lines | Responsibility |
|------|-------|----------------|
| `agenttask_controller.go` | 701 | Main reconciler, phase machine, retry logic, SetupWithManager |
| `helpers.go` | 256 | Config caching, duration parsing, log collection, parent notification, token/result extraction |
| `pod_builder.go` | 329 | Pod and PVC construction, resource parsing, volume setup |
| `policy.go` | 147 | AgentPolicy enforcement (capabilities, images, budget, tier, concurrency) |
| `metrics.go` | 68 | Prometheus metrics and OpenTelemetry tracing |
| `result_cache.go` | 167 | Content-addressable result cache with SHA-256 keying and LRU eviction |
| `warm_pool.go` | 336 | Warm Pod pool management (reconcile, replenish, build, claim, inject) |

**Phase machine flow:** `Pending` → policy checks → cache check → warm pool or cold Pod → `Running` → pod monitoring → `Completed`/`Failed`/`TimedOut` → TTL cleanup.

### 3. CLI (`cmd/hortator/`)

Cobra-based CLI with all critical flags now implemented:

| Command | Purpose |
|---------|---------|
| `spawn` | Creates AgentTask CRDs with `--role`, `--tier`, `--parent`, `--wait`, `--wait-timeout` |
| `status` | Checks task phase |
| `result` | Gets task output |
| `logs` | Streams pod logs |
| `cancel` | Sets phase to Cancelled |
| `list` | Lists tasks in namespace |
| `tree` | Visualizes parent/child hierarchy |
| `report` | Agents report results back to CRD status |
| `delete` | Removes tasks |
| `version` | Prints version info |

### 4. Gateway (`cmd/gateway/`, `internal/gateway/`)

OpenAI-compatible HTTP API that translates chat completion requests into AgentTask CRDs:

- `POST /v1/chat/completions` — Creates an AgentTask, watches it, returns results in OpenAI format
- `GET /v1/models` — Lists AgentRoles as available "models"
- Both blocking and SSE streaming responses
- Bearer token authentication with cached K8s Secret (60s TTL)
- Model resolution from AgentRole CRDs with intelligent defaults

### 5. Runtime (`runtime/`)

Bash entrypoint script running inside agent pods:

- Reads `/inbox/task.json`, calls Anthropic or OpenAI API (or echo mode)
- PII scanning via centralized Presidio service
- Reports results via `hortator report` CLI (primary) or stdout markers (fallback)
- Writes `result.json` and `usage.json` to `/outbox/`
- Graceful SIGTERM handling

### 6. SDKs (`sdk/`)

#### Python SDK (`sdk/python/`)

- **`hortator` package**: Sync `HortatorClient` and async `AsyncHortatorClient`
- `run()` for single prompts, `chat()` for multi-turn, `stream()` for SSE
- `list_models()` to discover available roles
- Custom exceptions: `HortatorError`, `AuthenticationError`, `RateLimitError`
- **Integrations**: `HortatorLangChainLLM` (LangChain), `HortatorCrewAITool` (CrewAI)
- **Tests**: `test_client.py`, `test_models.py`, `test_streaming.py`
- **Examples**: basic usage, streaming, LangChain tool, CrewAI delegation
- Built on `httpx` for both sync and async

#### TypeScript SDK (`sdk/typescript/`)

- **`@hortator/sdk`**: Zero-dependency client using native `fetch`
- `run()`, `chat()`, `stream()` (returns `AsyncIterableIterator<StreamChunk>`)
- `listModels()` for role discovery
- Custom `HortatorError` with status code and error type
- **Integration**: `HortatorLangChainLLM` for LangChain.js
- **Tests**: `client.test.ts`, `streaming.test.ts`
- Built with tsup, targets ESM and CJS

### 7. Helm Chart (`charts/hortator/`)

Comprehensive Helm chart with templates for operator, gateway, Presidio, NetworkPolicies, RBAC, ServiceMonitor, and ConfigMap. New configuration sections for warm pool and result cache.

### 8. CRDs (`crds/`)

Three CRD definitions synced via `make sync-crds`: `AgentTask` (generated), `AgentPolicy` (generated), `AgentRole`/`ClusterAgentRole` (hand-maintained). CI enforces sync across all three locations.

### 9. CI/CD (`.github/workflows/`)

- **CI** (`ci.yaml`): lint, test (with coverage), build, release (on tags)
- **Image builds** (`build-images.yml`): multi-arch Docker images
- **PR checks** (`pr-check.yaml`): verifies go.mod tidy, generated code, CRD sync

### 10. Documentation (`docs/`)

Previously placeholder pages are now populated:
- `architecture/crds.md` — Full CRD reference with field tables
- `architecture/storage.md` — Storage model, PVC lifecycle, retention, quotas
- `architecture/telemetry.md` — Prometheus metrics and OpenTelemetry events
- `architecture/warm-pool.md` — Comprehensive design doc with decisions, alternatives, limitations

Still placeholder: `configuration/agentrole.md`, `configuration/agenttask.md`, `configuration/helm-values.md`, `guides/budget.md`, `guides/model-routing.md`, `guides/presidio.md`, `enterprise/overview.md`.

---

## What's Good

### Architecture & Design

1. **Strong conceptual model.** The Roman hierarchy maps cleanly to task decomposition (strategic → coordination → execution). Tier-based defaults for storage, model selection, and PVC sizing are well thought out.

2. **Kubernetes-native done right.** CRDs with status subresource, controller-runtime reconciler, owner references, finalizers, proper RBAC markers. The phase-based state machine is clean and well-separated across files.

3. **Security-first design.** Capability inheritance, per-namespace `AgentPolicy`, generated NetworkPolicies, and namespace label restrictions provide defense in depth for autonomous agent hierarchies.

4. **Clean separation of concerns.** Operator, CLI, gateway, runtime, and SDKs are cleanly separated. Three personas (platform engineers, agents, external clients) each have a dedicated interface.

5. **Intentional PVC lifecycle design.** The three-stage storage funnel (hot PVC → tagged retention → cold S3/vector graduation) is natural for agent workloads. TTL cleanup is the garbage collector for untagged PVCs; `hortator retain` preserves important results.

6. **Warm Pod pool is well-designed.** One-shot consumption avoids state leakage. Generic pools avoid fragmentation. `kubectl exec` for injection is pragmatic. Graceful degradation means the feature can't break the happy path. The design doc in `docs/architecture/warm-pool.md` is thorough with clear decision rationale.

7. **Result cache is appropriately scoped.** In-memory, SHA-256 keyed on prompt+role, LRU eviction, TTL expiry, opt-out annotation. Correctly only caches successes. The cache is a pure optimization — system works identically without it.

8. **Controller refactor is well-executed.** The split into `pod_builder.go`, `policy.go`, `helpers.go`, `metrics.go`, `result_cache.go`, `warm_pool.go` creates clear boundaries. Each file has a focused responsibility and can be tested independently.

9. **The gateway is clever.** Exposing AgentRoles as OpenAI "models" means any OpenAI SDK can drive Hortator. SSE streaming with progress updates is a nice touch. Auth caching avoids API server pressure.

10. **SDKs are production-quality.** Both Python and TypeScript SDKs have clean APIs (sync/async, streaming, context managers), proper error handling, framework integrations (LangChain, CrewAI), and tests. The Python SDK uses `httpx` for both sync and async, avoiding two HTTP libraries.

### Code Quality

11. **Comprehensive test coverage.** Unit tests now cover: resource parsing, pod building, policy enforcement, retry logic, result cache, warm pool, gateway handlers, gateway helpers, SDK clients, and SDK streaming. Table-driven tests throughout.

12. **Proper error handling.** `parseQuantity()` replaces all `MustParse` calls. The warm pool degrades gracefully. Auth caching falls back to fresh fetch. The result cache is opt-in and skip-safe.

13. **Thread safety.** ConfigMap defaults, auth keys, and warm pool state all use `sync.RWMutex`. The result cache has proper locking for concurrent reads and writes.

14. **CRD sync workflow.** `make sync-crds` → `make verify-crds` → CI enforcement eliminates the duplicate CRD drift problem. The CRD docs now explain the source-of-truth workflow.

15. **Good Prometheus metrics.** Three core metrics (total, active gauge, duration histogram) with OTel attributes that properly correlate to the task hierarchy.

---

## What Could Be Improved

### Medium Issues

**M1: Warm pool PVCs and Pods have no owner reference — orphan risk.**
Warm pool Pods and PVCs (lines 168–243 in `warm_pool.go`) are created without owner references and not tied to any CRD. If the operator crashes or is redeployed, orphaned warm Pods/PVCs may accumulate. The PVC is owned by the task *after* claiming (`claimWarmPod` sets the owner ref), but unclaimed warm resources have no owner. Consider a cleanup mechanism — either a label-based sweeper on startup or a TTL annotation on idle warm Pods.

**M2: Warm pool `replenishWarmPool` uses `resource.ParseQuantity` directly.**
In `buildWarmPod()` (lines 122–141 of `warm_pool.go`), resource strings are parsed with `resource.ParseQuantity` which is fine but inconsistent with the rest of the codebase that now uses `parseQuantity()`. If an invalid default value sneaks in, this won't panic (ParseQuantity returns an error) but the error is silently swallowed with `if err == nil`. Consider using `parseQuantity()` consistently and returning the error.

**M3: Policy enforcement is still O(n*m) for concurrency checks.**
`enforcePolicy` in `policy.go` (lines 129–141) fetches ALL tasks in the namespace on every pending task reconciliation to count running tasks. At scale (hundreds of tasks per namespace), this is expensive. Consider maintaining a running-task counter in the reconciler or using an informer-based count.

**M4: No validation webhook.**
CRDs rely on kubebuilder validation markers, but there is no admission webhook for complex cross-field constraints (e.g., "child budget must not exceed parent budget", "tier must be <= parent tier"). Invalid tasks are only caught at reconciliation time, consuming an API write and reconcile cycle before failing.

**M5: `AgentRole`/`ClusterAgentRole` still lack Go types.**
The gateway uses `unstructured.Unstructured` to read AgentRole CRDs (handler.go lines 422–453). This means no generated deep-copy, no compile-time field validation, and no ability to use them in typed controller code. The CRD docs (crds.md line 13) acknowledge this as a known gap. Adding Go types would enable typed watches and a potential AgentRole controller.

**M6: PVC owner reference vs. retention model tension.**
PVCs are created with `SetControllerReference(task, pvc, ...)` (pod_builder.go line 79). K8s garbage collection will cascade-delete the PVC when the AgentTask is deleted. The `retain=true` check prevents the AgentTask from TTL deletion, but if an AgentTask is deleted by any other means, the PVC goes with it. When S3/vector graduation is implemented, retained PVCs will need to outlive their AgentTasks — consider detaching the owner reference when retention is set.

**M7: No rate limiting on the gateway.**
The gateway has no rate limiting. A misbehaving client could create unlimited AgentTasks. Consider adding per-client rate limiting, or at minimum checking `AgentPolicy.maxConcurrentTasks` at the gateway layer before creating the CRD.

**M8: Cache key doesn't include model or tier.**
`CacheKey` in `result_cache.go` (line 65) hashes only `prompt + role`. Two tasks with the same prompt and role but different models or tiers would share the same cache entry. If the same prompt sent to `claude-opus` vs. `gpt-4o-mini` should produce different outputs, the cache key should include the model name and tier.

### Minor Issues / Polish

**L1: Remaining "Coming soon" documentation pages.**
About 30% of docs are still placeholders: `configuration/agentrole.md`, `configuration/agenttask.md`, `configuration/helm-values.md`, `guides/budget.md`, `guides/model-routing.md`, `guides/presidio.md`, `enterprise/overview.md`. The core architecture docs are now solid.

**L2: `waitForTask` still uses polling, not watches.**
The CLI's `waitForTask` (spawn.go line 134) polls every 2 seconds. For a K8s-native tool, a Watch would be more efficient and responsive. This is acceptable for CLI usage but adds unnecessary latency for short tasks.

**L3: Entrypoint tier-to-model mapping is duplicated.**
`entrypoint.sh` maps tiers to OpenAI models (fast→gpt-4o-mini, think→gpt-4o, deep→gpt-4o) then overrides with Anthropic models when `ANTHROPIC_API_KEY` is set (fast→sonnet, think→sonnet, deep→opus). The initial OpenAI mapping is dead code when Anthropic keys are present.

**L4: Warm pool Pods always provision in operator namespace.**
`replenishWarmPool` and `buildWarmPod` use `r.Namespace` (the operator namespace), but tasks may be in different namespaces. If a task in namespace `ai-team` tries to claim a warm Pod, the warm Pod is in `hortator-system` — the task and Pod are in different namespaces. The `claimWarmPod` also lists in `r.Namespace`. This means warm pool only works for tasks in the operator's namespace. The warm-pool.md doc notes this as a limitation ("per-operator-namespace"), but this should be more prominently documented or enforced.

**L5: Result cache `order` slice grows unbounded with expired entries.**
In `result_cache.go`, the `order` slice (used for LRU eviction) tracks insertion order. When entries are lazily removed on TTL expiry (line 98), the corresponding key stays in `order` until it's eventually skipped by `evictOldest`. With high churn and long TTLs, this slice could accumulate stale keys. This is minor but could be addressed with periodic compaction.

**L6: The `stream.Close()` return value is now properly handled.**
`helpers.go` line 182 uses `defer func() { _ = stream.Close() }()` which is correct. Good fix from the previous review.

**L7: SDK `_check_response` is called inside streaming context.**
In the Python SDK `client.py` line 122, `_check_response(resp)` is called inside `self._client.stream(...)` context manager. For httpx streaming responses, the status code is available but the body may not be fully read yet. If the server returns a 401 with a JSON error body, `resp.text` inside a streaming context may not return the full body. Consider reading the body before raising, or using `resp.raise_for_status()` with a custom handler.

---

## Previously Reported Issues — Now Resolved

The following issues from the first review have been addressed:

| ID | Issue | Resolution |
|----|-------|------------|
| C1 | ConfigMap reload on every reconciliation | Cached with 30s TTL via `refreshDefaultsIfStale()` + `sync.RWMutex` |
| C2 | `resource.MustParse` panics | Replaced with `parseQuantity()` that returns errors |
| C3 | No jitter in retry backoff | `computeBackoff()` now adds ±25% jitter |
| C4 | Init container uses `busybox:latest` | Pinned to `busybox:1.37.0` |
| M2 | Reconciler file too large (1318 lines) | Split into 7 focused files |
| M3 | Auth on every gateway request | Cached with 60s TTL via `getAuthKeys()` + `sync.RWMutex` |
| M5 | Shell interpolation in init container | Now uses env var: `printf '%s' "$TASK_JSON"` |
| M6 | Missing `--role`/`--tier` in spawn CLI | Added `--role`, `--tier`, `--parent` flags |
| M8 | Missing terminal phases in `waitForTask` | All phases now handled: `BudgetExceeded`, `TimedOut`, `Cancelled`, `Retrying` |
| L1 | Test coverage ~5-10% | Comprehensive unit tests across controller, gateway, and SDKs |
| L2 | Inconsistent license headers | All files now use MIT SPDX headers |
| L3 | Placeholder docs | Core architecture docs (`crds.md`, `storage.md`, `telemetry.md`) now populated |
| L4 | Makefile was backlog markdown | Now a proper kubebuilder Makefile with build/test/sync targets |
| L5 | Duplicate CRDs with no sync | `make sync-crds` + `make verify-crds` + CI enforcement |
| L8 | No context timeout on `waitForTask` | `--wait-timeout` flag with `context.WithTimeout` |

---

## Summary

This revision of the codebase shows substantial improvement. All Critical issues and most Moderate issues from the first review have been addressed. The codebase has evolved from a solid MVP into a production-hardening phase with:

- Clean architecture (7-file controller split, proper caching, thread safety)
- Two new features (warm Pod pool, result cache) with thorough design documentation
- Two SDKs (Python + TypeScript) with framework integrations
- Significantly improved test coverage across all components
- Proper build tooling (Makefile, CRD sync, CI enforcement)

**Remaining priority areas:**

| Priority | Area | Impact |
|----------|------|--------|
| **Medium** | Orphan warm pool resources, cache key scope, policy O(n*m) | Production reliability at scale |
| **Medium** | PVC owner ref vs. retention, no rate limiting | Preparation for graduation pipeline + security |
| **Medium** | AgentRole Go types, validation webhook | Developer experience and early error detection |
| **Low** | Remaining doc placeholders, CLI polling, entrypoint duplication | Polish and developer experience |

The foundation is production-ready for moderate scale. The improvement trajectory is strong — the codebase went from ~5% test coverage to comprehensive tests, addressed all crash risks, and added two significant features with clean design.
