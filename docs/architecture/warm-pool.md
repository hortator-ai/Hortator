# Warm Pod Pool

## Overview

The warm Pod pool pre-provisions idle agent Pods that accept tasks immediately, eliminating image pull and container startup latency. Instead of waiting 5–30 seconds for a new Pod to schedule, pull its image, and start, a warm Pod is ready in under a second.

## How It Works

```
┌─────────────────┐     ┌──────────────┐     ┌──────────────┐
│  Warm Pod (idle) │     │  Warm Pod    │     │  Warm Pod    │
│  waiting for     │     │  (idle)      │     │  (idle)      │
│  task.json...    │     │              │     │              │
└────────┬────────┘     └──────────────┘     └──────────────┘
         │
         │  AgentTask arrives
         ▼
┌─────────────────┐
│  Operator claims │──► exec: write /inbox/task.json
│  warm Pod        │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────┐
│  Pod starts work │     │  New warm Pod│◄── Operator replenishes pool
│  immediately     │     │  (idle)      │
└─────────────────┘     └──────────────┘
```

1. Operator maintains a pool of **N idle Pods** per namespace (configurable, default 2).
2. Each warm Pod runs the agent image with a wait loop: it polls for `/inbox/task.json` every 500ms.
3. When an `AgentTask` arrives, `handlePending()` checks for an available warm Pod **before** creating a new one.
4. If found: the operator **claims** the Pod (patches labels), **execs** into it to write `task.json`, and the agent starts immediately.
5. If not found: falls back to normal Pod creation (existing flow — no degradation).
6. Consumed Pods are **not returned to the pool**. The operator creates a replacement warm Pod to maintain pool size.

## Configuration

```yaml
# values.yaml
warmPool:
  enabled: false   # opt-in
  size: 2          # idle Pods per namespace
```

Surfaces in the `hortator-config` ConfigMap as flat keys:

```yaml
warmPoolEnabled: "true"
warmPoolSize: "2"
```

## Design Decisions

### One-Shot Consumption, Not Reusable

**Decision:** Each warm Pod handles exactly one task, then is replaced.

**Why not reuse?** A reusable pool would return Pods after task completion for the next task. This is faster (no replenishment delay) but introduces serious risks:

- **State leakage:** Agent workspaces, environment, filesystem state from one task could leak to the next. Even with cleanup, it's hard to guarantee a pristine environment.
- **Security:** API keys, prompt content, and intermediate results from Task A should never be visible to Task B. Containers share a process namespace — a reusable model requires paranoid cleanup.
- **Complexity:** Resetting volumes, clearing environment, restarting the entrypoint — all fragile and error-prone.

One-shot is the Kubernetes-native pattern: Pods are cattle, not pets. The replenishment delay (~5s for image-cached nodes) is negligible compared to the risk.

### `kubectl exec` for Task Injection

**Decision:** The operator writes `task.json` into the warm Pod via SPDY exec (`cat > /inbox/task.json`).

**Alternatives considered:**

| Approach | Rejected Because |
|----------|-----------------|
| **ConfigMap mount** | Can't add new ConfigMap references to a running Pod. Projected volumes have propagation delay. |
| **Init container signaling** | Init containers run before the main container. Can't "release" an init container on a running Pod. |
| **Sidecar writer** | Adds complexity, another container to manage, coordination overhead. |
| **Helper Job that writes to PVC** | Defeats the purpose — we'd be creating a Pod (Job) to avoid creating a Pod. |
| **HTTP endpoint in Pod** | Requires adding an HTTP server to the agent runtime. Over-engineered for writing one file. |

Exec is the simplest approach: one API call, writes directly to the emptyDir, no extra containers or infrastructure. The operator already has Pod exec RBAC (`pods/exec`).

### Generic Pool, Not Per-Role

**Decision:** Warm Pods use the default agent image and namespace-default API keys. They're not pre-configured for specific roles.

**Why:** Per-role pools fragment capacity. If you have 5 roles with 2 warm Pods each, that's 10 idle Pods — most sitting unused. A generic pool of 2–3 covers the common case where any task needs fast startup.

The task prompt, role, and model configuration are all written to `task.json` at claim time. The only thing that can't change is the container image and mounted secrets. Since most tasks in a namespace use the same image and API keys, generic pools work well.

**Future:** If per-role pools prove necessary (e.g., different images per role), we can add `warmPool.roles` configuration. The infrastructure supports it — `buildWarmPod()` just needs role-specific parameters.

### EmptyDir for /inbox, PVC for Storage

**Decision:** `/inbox` uses EmptyDir (operator writes via exec), while `/outbox`, `/workspace`, `/memory` use a pre-provisioned PVC.

**Why EmptyDir for inbox:** The operator needs to write `task.json` into the Pod after it starts. EmptyDir is immediately writable. A PVC-backed inbox would require the operator to mount the PVC elsewhere to write to it — adding complexity.

**Why PVC for storage:** Agent work products (results, artifacts, state) need to survive Pod termination for result collection. The PVC is pre-provisioned with the warm Pod, so there's no provisioning delay at claim time.

### Warm Pool Reconciliation Cooldown

**Decision:** Pool reconciliation runs at most every 30 seconds, piggybacking on the AgentTask reconcile loop.

**Why not a separate controller?** The warm pool doesn't have its own CRD — it's configuration-driven. A separate controller watching Pods would work but adds wiring complexity. Piggybacking on the existing reconcile loop with a cooldown timer is simpler and sufficient.

**Why 30 seconds?** Matches the ConfigMap cache TTL. Fast enough to replace consumed Pods within a minute, slow enough to avoid API spam when reconciling many tasks.

### Replenishment in Background Goroutine

**Decision:** After claiming a warm Pod, replenishment runs in a `go func()` to avoid blocking the task's reconcile path.

The claimed task should transition to Running immediately. Creating the replacement warm Pod involves PVC provisioning and Pod creation — we don't want that latency in the critical path. If replenishment fails, the next reconcile cycle catches it.

### Policy Enforcement Before Warm Pool

**Decision:** Capability checks and AgentPolicy validation run **before** attempting to claim a warm Pod.

If a task violates policy, it should fail fast — don't waste a warm Pod on a task that will be rejected. The flow is: namespace check → capability inheritance → policy enforcement → warm pool claim → (fallback) normal Pod creation.

### Graceful Degradation

**Decision:** If warm pool claim fails (no idle Pods, exec error, any transient issue), the operator falls back to normal Pod creation with a log warning. No task ever fails because of the warm pool.

The warm pool is a pure optimization. The system must work identically with `warmPool.enabled: false`. This also means the feature can be toggled on/off without risk.

## Labels

Warm Pods and PVCs use these labels for identification and lifecycle management:

| Label | On | Values | Purpose |
|-------|------|--------|---------|
| `hortator.ai/warm-pool` | Pod, PVC | `"true"` | Identifies warm pool resources |
| `hortator.ai/warm-status` | Pod | `"idle"`, `"claimed"` | Current status |
| `hortator.ai/warm-pod` | PVC | `<pod-name>` | Links PVC to its warm Pod |
| `hortator.ai/task` | Pod, PVC | `<task-name>` | Set on claim, links to AgentTask |

## Latency Comparison

| Scenario | Cold Pod | Warm Pod |
|----------|----------|----------|
| Image cached on node | ~2–5s | **<1s** |
| Image pull required | ~10–30s | **<1s** |
| With PVC provisioning | ~5–15s | **<1s** (pre-provisioned) |

## Limitations

- **Image changes require pool rotation.** If you update the agent image, existing warm Pods use the old image. They'll be consumed and replaced with new-image Pods naturally, but there's a brief window of mixed versions.
- **Secrets are fixed at Pod creation.** If API keys rotate, idle warm Pods still have the old keys. They'll fail on the first LLM call and get replaced. For fast key rotation, reduce pool size or manually delete warm Pods.
- **Pool size is per-operator-namespace**, not per-task-namespace. Multi-namespace pools are a future enhancement.
- **No autoscaling.** Pool size is static. Future: scale based on task arrival rate.

## Future Enhancements

- **Per-role pools:** Different warm Pod specs per AgentRole (different images, secrets, resource limits).
- **Autoscaling:** Adjust pool size based on task queue depth and arrival patterns.
- **Reusable mode (opt-in):** For trusted, stateless workloads where cleanup between tasks is acceptable.
- **Cross-namespace pools:** Shared warm Pods across namespaces with RBAC controls.
