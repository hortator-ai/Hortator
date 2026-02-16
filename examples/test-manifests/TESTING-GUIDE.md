# 🧪 AgentTask Test Manifests — Testing Guide

**Sandbox:** `ssh root@89.167.62.139`
**Namespace:** `hortator-test`
**Images:** `ghcr.io/hortator-ai/hortator/*:latest` (from CI)

## Pre-flight Checks

```bash
# Operator running?
kubectl get pods -n hortator-system -l app.kubernetes.io/name=hortator

# Secrets in place?
kubectl get secrets -n hortator-test

# CRDs registered?
kubectl get crd | grep hortator
```

---

## Test Execution Order

Tests are grouped by independence. Run in this order to avoid conflicts.

### Phase 1 — Independent Tests (run in parallel)

These have no dependencies on each other:

#### 1. Guardrails PII (`guardrails-pii.yaml`)
```bash
kubectl apply -f examples/test-manifests/guardrails-pii.yaml
```
**What to verify:**
- AgentPolicy `pii-required-policy` is created
- Task `guardrails-pii-test` runs with Presidio scanning enabled
- Check operator logs for Presidio scan events: `kubectl logs -n hortator-system -l app.kubernetes.io/name=hortator | grep -i presidio`
- Task should complete (or have output scanned) — look for `presidio` annotations on the task

**Expected outcome:** Task completes. If the agent outputs PII, Presidio should detect and flag it based on `scoreThreshold: 0.7`.

---

#### 2. Budget Limit (`budget-limit.yaml`)
```bash
kubectl apply -f examples/test-manifests/budget-limit.yaml
```
**What to verify:**
- Task `budget-limit-test` starts with a very low budget ($0.02)
- Watch for phase transitions: `kubectl get agenttask budget-limit-test -n hortator-test -w`
- Task should eventually hit `BudgetExceeded` phase
- Check status: `kubectl get agenttask budget-limit-test -n hortator-test -o jsonpath='{.status.phase}'`
- Verify partial results are preserved in `/outbox/`

**Expected outcome:** Phase → `BudgetExceeded`. The three-phase wind-down (warning at 80%, soft ceiling at 95%, hard kill at 100%) should be visible in operator logs.

---

#### 3. Retry Backoff (`retry-backoff.yaml`)
```bash
kubectl apply -f examples/test-manifests/retry-backoff.yaml
```
**What to verify:**
- Three tasks: `retry-transient-test`, `retry-exhaustion-test`, `retry-success-no-retry`
- `retry-transient-test`: Agent exits 1 on first attempt (marker file trick), operator retries, succeeds on attempt 2
  - Check: `kubectl get agenttask retry-transient-test -n hortator-test -o jsonpath='{.status.attempts}'` → should be 2
  - Phase should end as `Completed`
- `retry-exhaustion-test`: References non-existent secret → pod fails every time → exhausts retries → `Failed`
  - Check: `kubectl get agenttask retry-exhaustion-test -n hortator-test -o jsonpath='{.status.phase}'` → `Failed`
- `retry-success-no-retry`: Normal task with retry config → completes on first attempt, no retries triggered
  - Check: attempts = 1, phase = `Completed`

**Expected outcome:** One retry+recovery, one exhaustion→Failed, one clean success.

---

#### 4. Warm Pool (`warm-pool.yaml`)
```bash
kubectl apply -f examples/test-manifests/warm-pool.yaml
```
**What to verify:**
- Task `warm-pool-test` should claim a warm pod (if pool exists)
- Task `warm-pool-burst-*` tests should test pool exhaustion
- Compare startup time vs a cold-start task
- Check annotations for warm pool claim: `kubectl get agenttask warm-pool-test -n hortator-test -o yaml | grep warm`

**Expected outcome:** Fast startup if warm pool is provisioned. Graceful fallback to cold start if pool is empty.

---

### ⚠️ Cleanup Between Phases

AgentPolicies are namespace-scoped and affect ALL tasks. Clean up before moving to the next phase:

```bash
# Remove all policies and tasks from Phase 1
kubectl delete agentpolicies --all -n hortator-test
kubectl delete agenttasks --all -n hortator-test
```

### Phase 2 — Sequential Tests

#### 5. Storage Promotion (`storage-promotion.yaml`)
```bash
# Step 1: Apply producer
kubectl apply -f examples/test-manifests/storage-promotion.yaml
# Wait for producer to complete
kubectl wait --for=jsonpath='{.status.phase}'=Completed agenttask/storage-promotion-producer -n hortator-test --timeout=300s

# Step 2: Check that PVC was retained
kubectl get pvc -n hortator-test -l hortator.ai/retained=true
```
**What to verify:**
- Producer task completes and calls `hortator retain`
- PVC gets `hortator.ai/retained=true` label
- Consumer task (if present) can discover the retained PVC via label selector

**Expected outcome:** PVC persists after task completion instead of being garbage collected.

---

#### 6. Result Cache (`result-cache.yaml`)
```bash
# Step 1: Cache miss (first run)
kubectl apply -f examples/test-manifests/result-cache.yaml
# Wait for first task
kubectl wait --for=jsonpath='{.status.phase}'=Completed agenttask/result-cache-miss -n hortator-test --timeout=300s

# Step 2: Check cache hit and skip tasks
kubectl get agenttask result-cache-hit -n hortator-test -o jsonpath='{.status.phase}'
kubectl get agenttask result-cache-skip -n hortator-test -o jsonpath='{.status.phase}'
```
**What to verify:**
- `result-cache-miss`: Runs normally, result gets cached (SHA-256 of prompt+role)
- `result-cache-hit`: Same prompt+role — should return cached result instantly (check if it spawns a pod at all)
- `result-cache-skip`: Has `hortator.ai/cache: skip` annotation — should run fresh even with cache available

**Expected outcome:** Cache hit task completes near-instantly without spawning a pod. Cache skip task runs normally.

---

### ⚠️ Cleanup Before Phase 3

```bash
kubectl delete agentpolicies --all -n hortator-test
kubectl delete agenttasks --all -n hortator-test
```

### Phase 3 — Complex Tests

#### 7. File Delivery (`file-delivery.yaml`)
```bash
kubectl apply -f examples/test-manifests/file-delivery.yaml
```
**What to verify:**
- ConfigMap with input files is created
- Task mounts files to `/inbox/`
- Agent can read delivered files
- RAG env vars are set (if configured)

**Expected outcome:** Task completes and references the delivered files in its output.

---

#### 8. Multi-Tier (`multi-tier.yaml`)
```bash
kubectl apply -f examples/test-manifests/multi-tier.yaml
```
**What to verify:**
- ClusterAgentRoles are created (tribune, centurion, legionary, specialist)
- Tribune task starts and spawns centurions
- Centurions spawn legionaries
- Full hierarchy visible: `kubectl get agenttasks -n hortator-test -l hortator.ai/test-suite=multi-tier`
- Check parent-child relationships: `kubectl get agenttask <name> -n hortator-test -o jsonpath='{.status.parentTask}'`
- Capability inheritance: children should NOT have more capabilities than parents

**Expected outcome:** Full task tree emerges. Tribune → Centurion → Legionary delegation works. Results flow back up.

---

#### 9. Policy Override (`policy-override.yaml`)
```bash
kubectl apply -f examples/test-manifests/policy-override.yaml
```
**What to verify:**
- AgentPolicy `restrictive-policy` is created
- `policy-compliant-test`: Should succeed (within policy bounds)
- `policy-tier-violation-test`: Should be rejected (tier not allowed)
- `policy-capability-violation-test`: Should be rejected (capability not allowed)
- `policy-budget-violation-test`: Should be rejected (budget exceeds policy max)
- Check rejection reasons: `kubectl describe agenttask policy-tier-violation-test -n hortator-test`

**Expected outcome:** 1 pass, 3 rejections. Policy enforcement works at admission time.

---

#### 10. Kitchen Sink (`kitchen-sink.yaml`)
```bash
kubectl apply -f examples/test-manifests/kitchen-sink.yaml
```
**What to verify:**
- Every feature in one task: role, flavor, budget, retry, storage retention, resources, health detection, Presidio, env vars
- Task starts and runs with all configurations applied
- Check the full spec: `kubectl get agenttask kitchen-sink-test -n hortator-test -o yaml`
- Verify resource limits are applied to the pod: `kubectl get pod -l hortator.ai/task=kitchen-sink-test -n hortator-test -o jsonpath='{.spec.containers[0].resources}'`

**Expected outcome:** Task runs with all features simultaneously. No conflicts between configurations.

---

#### 11. Delegation Scenarios (`delegation-scenarios.yaml`)
```bash
kubectl apply -f examples/test-manifests/delegation-scenarios.yaml -n hortator-test
```
**What to verify:**

**Scenario A — Complex (should delegate):**
- `delegation-complex` tribune spawns 2-3 children with **distinct** scopes
- Look for: backend-engineer, frontend-engineer, qa-engineer legionaries
- Each child prompt should be focused on ONE component (not the whole task)
- **Red flag:** Two children with overlapping prompts = prompt regression
- Check: `kubectl get agenttasks -n hortator-test -l hortator.ai/test-suite=delegation`

**Scenario B — Simple (should NOT delegate):**
- `delegation-simple` tribune completes with 0 child tasks
- The script should be written directly by the tribune
- Check: `kubectl get agenttasks -n hortator-test -l hortator.ai/test-case=should-not-delegate`
- **Red flag:** Any child tasks spawned = tribune over-delegating

**Expected outcome:** Complex task produces 2-3 well-scoped children. Simple task produces zero children.

---

## Observability During Tests

```bash
# Watch all tasks in real-time
kubectl get agenttasks -n hortator-test -w

# Operator logs (in another terminal)
kubectl logs -f -n hortator-system -l app.kubernetes.io/name=hortator

# Task-specific pod logs
kubectl logs -n hortator-test -l hortator.ai/task=<task-name> -c agent

# Hortator TUI (if installed)
hortator watch -n hortator-test
```

## Cleanup

```bash
# Remove all test resources
kubectl delete namespace hortator-test

# Or selectively
kubectl delete agenttasks -n hortator-test -l hortator.ai/test-suite
kubectl delete agentpolicies -n hortator-test --all
kubectl delete clusteragentroles -l hortator.ai/test-suite=multi-tier
```
