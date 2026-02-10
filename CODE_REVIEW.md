# Hortator Code Review

> Review date: 2026-02-10

---

## Table of Contents

- [Overview](#overview)
- [Component Breakdown](#component-breakdown)
  - [API Types](#1-api-types-apiv1alpha1)
  - [Operator Controller](#2-operator-controller-internalcontroller)
  - [CLI](#3-cli-cmdhortator)
  - [Gateway](#4-gateway-cmdgateway-internalgateway)
  - [Runtime](#5-runtime-runtime)
  - [Helm Chart](#6-helm-chart-chartshortator)
  - [CRDs](#7-crds-crds)
  - [CI/CD](#8-cicd-githubworkflows)
- [What's Good](#whats-good)
  - [Architecture & Design](#architecture--design)
  - [Code Quality](#code-quality)
- [What Could Be Improved](#what-could-be-improved)
  - [Critical Issues](#critical-issues)
  - [Moderate Issues](#moderate-issues)
  - [Minor Issues / Polish](#minor-issues--polish)
- [Summary](#summary)

---

## Overview

Hortator is a Kubernetes operator that orchestrates AI agents in a hierarchical structure inspired by Roman military ranks (Tribune > Centurion > Legionary). It lets agents spawn sub-agents as Kubernetes Pods with isolation, budget controls, security guardrails, and lifecycle management.

---

## Component Breakdown

### 1. API Types (`api/v1alpha1/`)

**`agenttask_types.go`** — The core CRD type definitions. Defines `AgentTask` (the main workload) and all supporting types (`ModelSpec`, `RetrySpec`, `BudgetSpec`, `StorageSpec`, `HealthSpec`, `PresidioSpec`). The status struct tracks phase, output, token usage, child tasks, retry history, and timing.

**`agentpolicy_types.go`** — Defines `AgentPolicy` for namespace-scoped constraints (capability allow/deny lists, image restrictions, budget caps, concurrency limits, egress allowlists).

### 2. Operator Controller (`internal/controller/`)

**`agenttask_controller.go`** (~1318 lines) — The heart of the system. A standard controller-runtime reconciler that implements:

- **Phase machine**: Pending → Running → Completed / Failed / TimedOut / BudgetExceeded / Cancelled, with a Retrying intermediate state.
- **`handlePending`**: Validates namespace labels, enforces capability inheritance (children can't escalate beyond parent), enforces `AgentPolicy` restrictions, creates PVCs, builds and creates Pods.
- **`handleRunning`**: Monitors pod status, handles success/failure/timeout, extracts results from logs or CRD status, manages retries for transient failures.
- **`handleRetrying`**: Waits for backoff timer, transitions back to Pending.
- **`handleTTLCleanup`**: Deletes terminal tasks after retention period expires.
- **`buildPod`**: Constructs the Pod spec with init container (writes `task.json`), agent container, volumes (`/inbox`, `/outbox`, `/workspace`, `/memory`), env vars (API keys, model config), and resource limits.
- **`enforcePolicy`**: Iterates all `AgentPolicy` objects in namespace, checking capabilities, images, budget, timeout, tier, and concurrency.
- **Observability**: Prometheus metrics (`hortator_tasks_total`, `hortator_tasks_active`, `hortator_task_duration_seconds`) and OpenTelemetry spans for lifecycle events.

### 3. CLI (`cmd/hortator/`)

A Cobra-based CLI designed to run inside agent pods. Commands:

| Command | Purpose |
|---------|---------|
| `spawn` | Creates AgentTask CRDs (agents spawning agents) |
| `status` | Checks task phase |
| `result` | Gets task output |
| `logs` | Streams pod logs |
| `cancel` | Sets phase to Cancelled |
| `list` | Lists tasks in namespace |
| `tree` | Visualizes parent/child hierarchy |
| `report` | Agents report results back to CRD status (primary reporting path) |
| `delete` | Removes tasks |
| `version` | Prints version info |

### 4. Gateway (`cmd/gateway/`, `internal/gateway/`)

An OpenAI-compatible HTTP API that translates standard chat completion requests into AgentTask CRDs:

- `POST /v1/chat/completions` — Creates an AgentTask, watches it, returns results in OpenAI format.
- `GET /v1/models` — Lists AgentRoles as available "models".
- Supports both blocking and SSE streaming responses.
- Bearer token authentication against a K8s Secret.
- Model resolution from AgentRole CRDs with intelligent defaults.

### 5. Runtime (`runtime/`)

A bash entrypoint script (`entrypoint.sh`) that runs inside agent pods:

- Reads `/inbox/task.json`.
- Scans prompt for PII via Presidio (if configured).
- Calls either Anthropic or OpenAI API based on available env vars (or runs in echo mode).
- Maps tiers to model names.
- Reports results via `hortator report` CLI (primary) or stdout markers (fallback).
- Writes `result.json` and `usage.json` to `/outbox/`.
- Handles SIGTERM gracefully.

### 6. Helm Chart (`charts/hortator/`)

Comprehensive Helm chart with templates for:

- Operator Deployment with health probes and metrics.
- Gateway Deployment (optional) with HTTPRoute support.
- Presidio Deployment (optional, centralized service).
- NetworkPolicies (capability-driven: `web-fetch`, `spawn`, `presidio`).
- RBAC (operator ClusterRole + worker namespace Role).
- ServiceMonitor for Prometheus.
- ConfigMap with full operator configuration.

### 7. CRDs (`crds/`)

Three CRD definitions: `AgentTask`, `AgentRole`/`ClusterAgentRole`, and `AgentPolicy`. Well-structured with validation, enums, defaults, and printer columns.

### 8. CI/CD (`.github/workflows/`)

Three workflows:

- **CI** (`ci.yaml`): lint (golangci-lint + Helm lint), test (envtest), build (Go + Docker), release (on tags).
- **Image builds** (`build-images.yml`): multi-arch Docker images pushed to `ghcr.io`.
- **PR checks** (`pr-check.yaml`): verifies `go.mod` tidy, generated code, CRD manifests.
- **Dependabot** (`dependabot.yml`): weekly updates for Go modules and GitHub Actions.

---

## What's Good

### Architecture & Design

1. **Strong conceptual model.** The Roman hierarchy metaphor maps cleanly to a real decomposition pattern (strategic → coordination → execution). The three-tier model is intuitive and the tier-based defaults (storage size, model selection) are well thought out.

2. **Kubernetes-native done right.** CRDs with status subresource, controller-runtime reconciler, owner references for garbage collection, finalizers for cleanup, proper RBAC markers. The phase-based state machine in the reconciler is clean and easy to follow.

3. **Security-first design.** Capability inheritance (children can't escalate beyond parent), per-namespace `AgentPolicy` enforcement, generated NetworkPolicies from capabilities, and namespace label restrictions are all solid guardrails for "agents spawning agents."

4. **Clean separation of concerns.** The operator, CLI, gateway, and runtime are cleanly separated. Agents interact via CLI (never YAML), platform engineers via Helm/CRDs, and external systems via the OpenAI-compatible gateway. Three distinct personas, three distinct interfaces.

5. **Presidio architecture pivot.** The decision to move from sidecar to centralized Deployment+Service was smart — it avoids the exit-code-137 problem and simplifies pod lifecycle.

6. **Result reporting dual path.** The primary `hortator report` CRD-patching path with stdout marker fallback is pragmatic. The system works even with older runtimes or if the CLI fails.

7. **The gateway is clever.** Exposing AgentRoles as OpenAI-compatible "models" means any OpenAI SDK or tool can drive Hortator without custom integration. SSE streaming with progress updates is a nice touch.

8. **Intentional PVC lifecycle design.** The three-stage storage funnel (hot PVC → mid-term tagged retention → cold S3/vector graduation) is a natural fit for agent workloads where you don't know upfront which artifacts matter. TTL cleanup acts as the garbage collector for untagged PVCs, while `hortator retain` and tags preserve important results for later graduation.

### Code Quality

9. **Well-structured Go.** Idiomatic Go, clean package layout, proper error handling in most places, good use of controller-runtime primitives. The reconciler methods are reasonably sized and logically separated.

10. **Good Prometheus metrics.** The three metrics (total, active gauge, duration histogram) cover the essential observability needs. The exponential histogram buckets are sensible for AI task durations.

11. **Retry logic is well-tested.** `retry_test.go` has thorough table-driven tests covering edge cases (nil spec, zero max, exhaustion, custom backoff, capping). This is the best-tested part of the codebase.

12. **Comprehensive Helm values.** The `values.yaml` is well-organized with sensible defaults for every configuration area.

---

## What Could Be Improved

### Critical Issues

**C1: ConfigMap reload on every reconciliation.**
`loadClusterDefaults` is called on every single `Reconcile()` invocation (controller line 149). This means a K8s API call to fetch the ConfigMap on every reconciliation event. At scale with many tasks this will create unnecessary API server load. Consider using an informer/cache or a periodic refresh with a timer (e.g., every 30 seconds).

**C2: `resource.MustParse` will panic.**
In `buildPod` (controller lines 934–964), invalid resource strings from user input or ConfigMap values will cause `MustParse` to panic, crashing the operator. Use `resource.ParseQuantity` and return errors instead:

```go
// Current (dangerous):
resources.Requests[corev1.ResourceCPU] = resource.MustParse(r.defaults.DefaultRequestsCPU)

// Should be:
qty, err := resource.ParseQuantity(r.defaults.DefaultRequestsCPU)
if err != nil {
    return nil, fmt.Errorf("invalid default CPU request %q: %w", r.defaults.DefaultRequestsCPU, err)
}
resources.Requests[corev1.ResourceCPU] = qty
```

**C3: No jitter in retry backoff.**
The `computeBackoff` function uses pure exponential backoff without jitter. When multiple tasks fail simultaneously (e.g., an API outage), they'll all retry at the exact same time, causing a thundering herd. Add random jitter:

```go
jitter := time.Duration(rand.Int63n(int64(backoff) / 4))
return time.Duration(backoff)*time.Second + jitter
```

**C4: Init container uses `busybox:latest`.**
The init container that writes `task.json` (controller line 982) uses `busybox:latest` — a mutable tag that could break reproducibility or be blocked by image policies. Pin to a specific version or digest.

### Moderate Issues

**M1: PVC owner reference vs. retention model.**
PVCs are created with `SetControllerReference(task, pvc, ...)` (controller line 837), which means K8s garbage collection will cascade-delete the PVC when the owning AgentTask is deleted. This is fine for the current MVP since `retain=true` prevents the AgentTask itself from being TTL-deleted. However, once S3/vector graduation is implemented, retained PVCs will need to outlive their AgentTasks. At that point, consider detaching the owner reference on retained PVCs or moving to a separate PVC lifecycle controller.

**M2: The reconciler file is too large** (1318 lines).
`agenttask_controller.go` contains all reconciliation logic, pod building, policy enforcement, log collection, token extraction, PVC management, and volume construction. Consider splitting into separate files: `pod_builder.go`, `policy.go`, `cleanup.go`, `metrics.go`.

**M3: Authentication on every gateway request.**
The gateway's `authenticate` method (handler.go line 41) fetches the K8s Secret on every HTTP request. This should be cached with a TTL (e.g., 60 seconds) to avoid hammering the API server under load.

**M4: No rate limiting on the gateway.**
The OpenAI-compatible gateway has no rate limiting. A misbehaving client could create unlimited AgentTasks. Consider adding per-client rate limiting, or at minimum leveraging `AgentPolicy.maxConcurrentTasks` at the gateway layer.

**M5: Shell interpolation in the init container.**
`buildPod` escapes single quotes for shell and pipes the task spec JSON through `echo '...' > /inbox/task.json` (controller line 976–982). If the task spec contains backticks, dollar signs, or other shell metacharacters, this could break or be a security issue. Use a ConfigMap volume or a proper binary init container that writes from stdin instead of shell interpolation.

**M6: Missing `--role` and `--tier` in the `spawn` CLI.**
The `spawn` command (spawn.go lines 84–95) creates an `AgentTask` but never sets `Role`, `Tier`, or `ParentTaskID`. These are critical fields for the hierarchy system. The README shows `--role` being used, but it's not implemented in the code.

**M7: `waitForTask` uses polling, not watches.**
The CLI's `waitForTask` (spawn.go line 125) polls every 2 seconds. For a K8s-native tool, using a Watch would be more efficient and responsive.

**M8: Missing terminal phase handling in CLI `waitForTask`.**
The `waitForTask` function only handles `Completed`, `Failed`, `Running`, and `Pending`. It ignores `BudgetExceeded`, `TimedOut`, `Cancelled`, and `Retrying` — these will cause infinite polling.

**M9: Policy enforcement is O(n*m).**
`enforcePolicy` lists all policies and all running tasks for concurrency checks. The concurrent task counting (controller lines 1162–1173) fetches ALL tasks in the namespace on every pending task reconciliation. At scale, use a cached counter or informer.

**M10: No validation webhook.**
The CRDs rely on kubebuilder validation markers, but there is no admission webhook to enforce complex constraints (e.g., "tier must match parent tier hierarchy", "budget must not exceed parent budget"). Invalid tasks are only caught at reconciliation time, consuming resources before failing.

### Minor Issues / Polish

**L1: Test coverage is very thin (~5–10%).**
The controller test is essentially the kubebuilder scaffold with a TODO comment. The retry tests are good but only cover helper functions. There are no integration tests for the actual reconciliation flow (`handlePending`, `handleRunning`, etc.), no tests for the gateway, no tests for the CLI, and no tests for policy enforcement.

**L2: Inconsistent license headers.**
The controller has Apache 2.0 headers, but the README states MIT. The `LICENSE` file should be checked for consistency.

**L3: Several "Coming soon" doc pages.**
About 40% of the documentation pages are placeholders: `crds.md`, `storage.md`, `telemetry.md`, `helm-values.md`, `agentrole.md`, `agenttask.md`, `budget.md`, `model-routing.md`, `presidio.md`, and the enterprise docs.

**L4: `Makefile` contains backlog, not make targets.**
The `Makefile` is actually a markdown document with project planning notes, not a functional Makefile. This means there are no `make build`, `make test`, etc. targets — a friction point for contributors.

**L5: Duplicate CRD files.**
CRD YAMLs exist in `crds/`, `charts/hortator/crds/`, and `config/crd/bases/`. There is no mechanism ensuring they stay in sync.

**L6: Entrypoint tier-to-model mapping is duplicated.**
The tier-to-model mapping logic exists twice in `entrypoint.sh`: first for OpenAI models (fast→gpt-4o-mini, think→gpt-4o) and then overridden for Anthropic (fast→sonnet, deep→opus). The initial mapping is dead code when Anthropic keys are present.

**L7: `AgentRole`/`ClusterAgentRole` aren't Go types.**
They appear only as CRD YAMLs but have no corresponding Go types in `api/v1alpha1/`. The gateway uses `unstructured.Unstructured` to read them. This means they can't be used in strongly-typed controller code and there's no generated deep-copy or validation.

**L8: No context timeout on `waitForTask`.**
The CLI's `waitForTask` uses `context.Background()` (from `runSpawn` line 69), meaning `--wait` will block forever if the task never completes and timeout enforcement fails. It should inherit a context with a timeout.

---

## Summary

Hortator is a well-architected project with a clear vision and solid Kubernetes-native foundations. The core design — agents-as-pods with hierarchy, capability inheritance, budget controls, and an OpenAI-compatible gateway — is genuinely compelling. The code is clean and idiomatic Go.

The main areas needing attention:

| Priority | Area | Impact |
|----------|------|--------|
| **Critical** | `MustParse` panics, no jitter in retries, ConfigMap reload per reconcile, mutable init image | Operator crashes or thundering herds in production |
| **High** | Test coverage (~5–10%) | Can't refactor or ship with confidence |
| **Medium** | `spawn` CLI missing `--role`/`--tier`/`--parent`, `waitForTask` incomplete | Core user-facing features broken |
| **Medium** | Shell interpolation in init container | Security risk and fragility |
| **Medium** | Gateway rate limiting and auth caching | Scalability under load |
| **Low** | Controller file size, duplicate CRDs, Makefile, doc placeholders | Developer experience and maintenance burden |

The foundation is strong. The improvements are mostly about hardening for production rather than architectural rework.
