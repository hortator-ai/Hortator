# Remaining Work & Testing Guide

_Last updated: 2026-02-11_

---

## Project Status: ~90% Complete

All core features are implemented. The remaining work is E2E validation, hardening, and documentation polish.

---

## 1. Blocking — Must-Do Before Go-to-Market

### 1.1 CRD Regeneration

**Effort:** 15 minutes

The `AgentRole` Go type has a `.Spec.Health` field for per-role stuck detection overrides, but the CRD YAML in `crds/` hasn't been regenerated to include it.

```bash
# On your dev machine (needs controller-gen):
make generate
make manifests
make sync-crds
make verify-crds
```

### 1.2 Full E2E Validation of Tribune Orchestration

**Effort:** 2–4 hours (manual testing + fixing any issues found)

This is the single most important validation. The tribune → centurion → legionary hierarchy is the product's core value prop. It has never been proven end-to-end on a real cluster.

**What to validate (see Section 4 for step-by-step instructions):**

1. Tribune pod starts with the `agent-agentic` image (not the bash runtime)
2. Tribune's agentic runtime calls `spawn_task` tool to create child tasks
3. Child tasks run and complete
4. Child results are injected into tribune's PVC at `/inbox/child-results/`
5. Tribune resumes (reincarnation), consolidates child results, and completes
6. Full result is available via `hortator result` and the gateway API

---

## 2. Medium Priority — Post-Launch Hardening

### 2.1 Code Review Items (from CODE_REVIEW.md)

| ID  | Issue | Effort |
|-----|-------|--------|
| M1  | Warm pool Pods/PVCs have no owner reference (orphan risk on operator restart) | 2h |
| M3  | Policy enforcement is O(n×m) for concurrency checks — fine at scale <100, revisit later | 1h |
| M4  | No admission/validation webhook for AgentTask CRD | 4h |
| M7  | No rate limiting on the API gateway | 2h |
| M8  | Result cache key doesn't include model or tier (potential false cache hits) | 30m |

### 2.2 Test Coverage Gaps

| Area | Current | Target | Work needed |
|------|---------|--------|-------------|
| Controller unit tests | ~44% | 60%+ | Integration tests for full reconcile loop |
| Gateway unit tests | ~59% | 70%+ | Streaming endpoint tests, session tests |
| E2E tests | Basic | Comprehensive | Multi-tier flow, retries, warm pool, policy violations |
| SDK tests in CI | Not run | Run in CI | Add Python pytest + TypeScript jest to ci.yaml |
| Performance | None | Basic | Stress test with 50+ concurrent tasks |

### 2.3 Documentation Gaps

- ~30% of docs in `docs/` are placeholders or thin
- Key gaps: `docs/getting-started/quickstart.md`, `docs/guides/` guides need fleshing out
- Need a recorded demo (terminal recording of `hortator watch` during a multi-tier task)

---

## 3. Future — Post-Launch Roadmap

| Feature | Priority | Effort |
|---------|----------|--------|
| Multi-tenancy (cross-namespace policies) | High | 1 week |
| Gateway session continuity (Level 1: PVC reuse) | Medium | 3 days |
| Object storage archival for artifacts | Medium | 3 days |
| Validation webhook for CRDs | Medium | 1 day |
| Gateway rate limiting | Medium | 1 day |
| RAG integration (vector store capability) | Low | 1 week |
| Webhook callbacks on task completion | Low | 2 days |
| OIDC/SSO (Enterprise) | Low | 1 week |
| Web dashboard | Low | 2 weeks |
| Go SDK | Low | 3 days |

---

## 4. Testing Guide — How to Test on a VM with K8s

### Prerequisites

Your VM needs:
- Kubernetes 1.28+ (already installed)
- Helm 3.x
- kubectl
- A default StorageClass (check with `kubectl get storageclass`)
- An LLM API key (Anthropic recommended for best tool-calling support)

### Step 0: Pull the Latest Code

```bash
cd /path/to/Hortator
git pull origin main
```

### Step 1: Build and Push Images

Since CI builds and pushes to GHCR on every merge to `main`, you can skip building locally. But if CI hasn't run yet on these latest changes, build locally:

```bash
# Build all three images
docker build -t ghcr.io/michael-niemand/hortator/operator:dev .
docker build -t ghcr.io/michael-niemand/hortator/agent:dev -f runtime/Dockerfile .
docker build -t ghcr.io/michael-niemand/hortator/agent-agentic:dev -f runtime/agentic/Dockerfile .
```

If using Kind:
```bash
kind load docker-image ghcr.io/michael-niemand/hortator/operator:dev
kind load docker-image ghcr.io/michael-niemand/hortator/agent:dev
kind load docker-image ghcr.io/michael-niemand/hortator/agent-agentic:dev
```

If using a real cluster with a registry, push to your registry and adjust `--set` values below.

### Step 2: Install Default StorageClass (if needed)

```bash
# Check if you have a default StorageClass
kubectl get storageclass

# If none is marked (default), install local-path-provisioner:
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml
kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

### Step 3: Install CRDs

```bash
kubectl apply -f crds/
```

### Step 4: Install Hortator via Helm

```bash
kubectl create namespace hortator-system 2>/dev/null || true

# For locally-built images:
helm upgrade --install hortator charts/hortator \
  --namespace hortator-system \
  --set operator.image.repository=ghcr.io/michael-niemand/hortator/operator \
  --set operator.image.tag=dev \
  --set operator.image.pullPolicy=IfNotPresent \
  --set agent.image=ghcr.io/michael-niemand/hortator/agent:dev \
  --set agent.agenticImage=ghcr.io/michael-niemand/hortator/agent-agentic:dev \
  --set presidio.enabled=true \
  --wait --timeout 120s

# For GHCR images (after CI runs):
# helm upgrade --install hortator charts/hortator \
#   --namespace hortator-system \
#   --set presidio.enabled=true \
#   --wait --timeout 120s
```

Verify:
```bash
kubectl get pods -n hortator-system
# Should see: hortator-operator-xxx (Running), hortator-presidio-xxx (Running)
```

### Step 5: Create Demo Namespace and API Key Secret

```bash
kubectl create namespace hortator-demo

# Anthropic
kubectl create secret generic anthropic-api-key \
  --namespace hortator-demo \
  --from-literal=api-key=sk-ant-YOUR-KEY-HERE

# OR OpenAI
# kubectl create secret generic openai-api-key \
#   --namespace hortator-demo \
#   --from-literal=api-key=sk-YOUR-KEY-HERE
```

### Step 6: Test — Single-Tier (Legionary)

```bash
kubectl apply -f examples/quickstart/hello-world.yaml
kubectl get agenttasks -n hortator-demo -w

# Wait for Completed, then:
kubectl get agenttask hello-world -n hortator-demo -o jsonpath='{.status.output}'
kubectl logs -n hortator-demo -l hortator.ai/task=hello-world -c agent
```

**Expected:** Task completes in 10–30s with an LLM response. Check logs for `[hortator-runtime] Result reported to CRD`.

### Step 7: Test — Multi-Tier (Tribune → Centurion → Legionary)

This is the critical test.

```bash
kubectl apply -f examples/advanced/multi-tier.yaml

# Watch the task tree unfold:
hortator watch -n hortator-demo
# OR:
kubectl get agenttasks -n hortator-demo -w
```

**Expected behavior:**
1. `build-rest-api` (tribune) starts → phase `Running`
2. Tribune spawns centurion child tasks → new AgentTasks appear
3. Centurions spawn legionary children → more AgentTasks appear
4. Legionaries complete → results flow up
5. Tribune may enter `Waiting` phase (checkpoint), then resume when children complete
6. Eventually `build-rest-api` → phase `Completed`

**What to check if it doesn't work:**
```bash
# Check which image the tribune pod is using:
kubectl get pod -n hortator-demo -l hortator.ai/task=build-rest-api -o jsonpath='{.items[0].spec.containers[0].image}'
# Should be the agent-agentic image, NOT the bash agent image

# Check pod labels:
kubectl get pod -n hortator-demo -l hortator.ai/task=build-rest-api --show-labels
# Should include: hortator.ai/managed=true, hortator.ai/cap-spawn=true

# Check tribune logs:
kubectl logs -n hortator-demo -l hortator.ai/task=build-rest-api -c agent

# Check for children:
kubectl get agenttasks -n hortator-demo
```

### Step 8: Test — Budget Enforcement

```bash
cat <<'EOF' | kubectl apply -n hortator-demo -f -
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: budget-test
spec:
  prompt: "Write a very detailed 10000-word essay about the history of computing."
  tier: legionary
  model:
    name: claude-sonnet-4-20250514
    endpoint: https://api.anthropic.com/v1
    apiKeyRef:
      secretName: anthropic-api-key
      key: api-key
  budget:
    maxTokens: 100
  timeout: 120
EOF

kubectl get agenttask budget-test -n hortator-demo -w
# Expected: BudgetExceeded phase
```

### Step 9: Test — PII Detection (Presidio)

```bash
cat <<'EOF' | kubectl apply -n hortator-demo -f -
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: pii-test
spec:
  prompt: "My name is John Smith, my SSN is 123-45-6789, and my email is john@example.com. Just repeat this back."
  tier: legionary
  model:
    name: claude-sonnet-4-20250514
    endpoint: https://api.anthropic.com/v1
    apiKeyRef:
      secretName: anthropic-api-key
      key: api-key
  timeout: 120
EOF

# Check logs for PII detection:
kubectl logs -n hortator-demo -l hortator.ai/task=pii-test -c agent | grep -i "PII"
# Expected: "PII detected: PERSON", "PII detected: US_SSN", etc.
```

### Step 10: Test — `hortator` CLI & TUI

```bash
# List all tasks
hortator list -n hortator-demo

# Show task tree
hortator tree build-rest-api -n hortator-demo

# Get task result
hortator result hello-world -n hortator-demo

# Launch TUI dashboard
hortator watch -n hortator-demo
```

### Step 11: Test — API Gateway (Optional)

If you enabled the gateway (`gateway.enabled=true` in Helm values):

```bash
# Port-forward the gateway
kubectl port-forward -n hortator-system svc/hortator-gateway 8080:80 &

# Send a request
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR-GATEWAY-KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tech-lead",
    "messages": [{"role": "user", "content": "Say hello from the gateway"}]
  }'
```

### Step 12: Cleanup

```bash
kubectl delete agenttasks --all -n hortator-demo
kubectl delete namespace hortator-demo
helm uninstall hortator -n hortator-system
kubectl delete namespace hortator-system
kubectl delete -f crds/
```

---

## 5. Automated Test Suite

```bash
# Unit tests (no cluster needed)
go test ./internal/... -count=1 -short

# Full unit tests with envtest (needs setup-envtest):
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path)"
go test ./internal/... -count=1

# E2E tests (needs a running cluster):
export USE_EXISTING_CLUSTER=true
go test ./test/e2e/ -tags=e2e -v -timeout 15m

# Python SDK tests:
cd sdk/python && pip install -e ".[dev]" && pytest

# TypeScript SDK tests:
cd sdk/typescript && npm install && npm test

# Helm lint:
helm lint charts/hortator
```
