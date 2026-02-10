# Design: AgentTask Retry Semantics

**Status:** Implemented  
**Date:** 2026-02-09 (implemented 2026-02-10)  
**Author:** Daemon + M

## Problem

Failed AgentTasks are terminal — there's no automatic recovery. Users must manually delete and recreate tasks. This is fine for logical failures (bad prompt, budget exceeded) but unacceptable for transient failures (pod crash, OOM, sidecar race condition, network blip).

## Decision: Hybrid Retry (Option C)

Transient (infrastructure) failures auto-retry with backoff. Logical (agent-reported) failures stay terminal and require human/parent intervention.

## Classification

| Failure Type | Source | Retry? | Examples |
|---|---|---|---|
| **Transient** | Pod exit code != 0, OOM, timeout | ✅ Auto-retry | Sidecar race, network error, OOMKilled |
| **Logical** | Agent writes `status: failed` in `result.json` | ❌ No retry | "I can't do this", bad prompt, API auth error |
| **Budget** | Cost/token limit exceeded | ❌ No retry | maxCostUsd hit |

### How the operator distinguishes them

1. Pod terminates with **non-zero exit code** AND no `result.json` → **transient** → retry
2. Pod terminates and `result.json` exists with `status: failed` → **logical** → no retry
3. Pod terminates and `result.json` exists with `status: completed` → **success** → done
4. Pod exceeds timeout → **transient** → retry (up to limit)
5. Budget exceeded (reported by runtime or operator) → **budget** → no retry

## CRD Changes

### spec.retry (new)

```yaml
spec:
  retry:
    maxAttempts: 3        # default: 0 (no retry)
    backoffSeconds: 30    # initial backoff, doubles each attempt
    maxBackoffSeconds: 300 # cap
```

### status additions

```yaml
status:
  phase: Failed | Running | Completed | Retrying
  attempts: 2
  lastFailureReason: "exit code 7: entrypoint crash"
  lastFailureTime: "2026-02-09T19:22:56Z"
  history:
    - attempt: 1
      startTime: "2026-02-09T19:20:00Z"
      endTime: "2026-02-09T19:22:56Z"
      exitCode: 7
      reason: "transient: pod failed without result.json"
    - attempt: 2
      startTime: "2026-02-09T19:23:30Z"
      endTime: "2026-02-09T19:25:00Z"
      exitCode: 0
      reason: "completed"
```

## Operator Logic

```
on reconcile(task):
  if task.status.phase == "Retrying":
    if now < task.status.nextRetryTime:
      requeue after (nextRetryTime - now)
      return
    # else: spawn new pod

  if pod.failed:
    if result_json_exists and result.status == "failed":
      task.status.phase = "Failed"  # logical failure, no retry
      return

    if task.status.attempts >= task.spec.retry.maxAttempts:
      task.status.phase = "Failed"  # exhausted retries
      return

    # transient failure — schedule retry
    backoff = min(
      spec.retry.backoffSeconds * 2^(attempts-1),
      spec.retry.maxBackoffSeconds
    )
    task.status.phase = "Retrying"
    task.status.attempts += 1
    task.status.nextRetryTime = now + backoff
    requeue after backoff
```

## Multi-Tier Interaction

In the Tribune → Centurion → Legionary hierarchy:

- **Legionary** tasks should have `retry.maxAttempts: 2-3` by default (cheap, fast, worth retrying)
- **Tribune** tasks should have `retry.maxAttempts: 0-1` (expensive, complex, better to fail fast and let a human decide)
- A parent task (Tribune/Centurion) can inspect child failure reasons and decide to:
  - Retry with a different prompt
  - Retry with a different model
  - Spawn a different agent
  - Report failure upward

## Defaults

The ClusterAgentRole or Helm values can set tier-based defaults:

```yaml
# helm values
agent:
  retry:
    legionary:
      maxAttempts: 3
      backoffSeconds: 15
    centurion:
      maxAttempts: 1
      backoffSeconds: 30
    tribune:
      maxAttempts: 0
```

## Open Questions

1. Should we retry on API auth errors? (Probably not — they won't self-heal)
2. Should retries reset the budget counter or continue accumulating?  
   → **Continue accumulating** — prevents infinite spend on retrying expensive tasks
3. Should we emit K8s Events on retry decisions for observability?  
   → **Yes** — `Normal/Retrying` and `Warning/RetryExhausted` events

## Implementation Notes (2026-02-10)

- `RetrySpec` implemented in `api/v1alpha1/agenttask_types.go` with `maxAttempts`, `backoffSeconds`, `maxBackoffSeconds`
- `AttemptRecord` tracks per-attempt history in `status.history`
- `Retrying` phase added to `AgentTaskPhase` enum
- Backoff computation includes ±25% random jitter to prevent thundering herd (added in code review fix)
- Retry logic tested in `internal/controller/retry_test.go` with table-driven tests covering edge cases
