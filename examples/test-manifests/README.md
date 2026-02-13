# Test Manifests

Comprehensive YAML manifests for testing Hortator features. Each file targets a specific capability or combination of capabilities.

## Prerequisites

```bash
# Create test namespace
kubectl create namespace hortator-test

# Create API key secret
kubectl create secret generic anthropic-api-key \
  --namespace hortator-test \
  --from-literal=api-key=sk-ant-...

# (Optional) Create GitHub credentials for kitchen-sink test
kubectl create secret generic github-credentials \
  --namespace hortator-test \
  --from-literal=token=ghp_...

# (Optional) Create Milvus credentials for file-delivery RAG test
kubectl create secret generic milvus-credentials \
  --namespace hortator-test \
  --from-literal=api-key=...
```

## Manifests

| File | Feature | What It Tests |
|------|---------|---------------|
| `guardrails-pii.yaml` | Presidio PII scanning | Per-task Presidio overrides, PII detection in agent output, policy-enforced scanning |
| `storage-promotion.yaml` | Storage lifecycle | Ephemeral → retained PVC promotion via `hortator retain`, tag-based discovery by future tasks |
| `budget-limit.yaml` | Budget enforcement | Three-phase wind-down (warning → soft ceiling → hard ceiling), partial result preservation, task resume |
| `retry-backoff.yaml` | Retry semantics | Exponential backoff with jitter, transient vs logical failure classification, retry exhaustion |
| `result-cache.yaml` | Result cache | Content-addressable dedup (SHA-256 of prompt+role), cache hit/miss/skip behavior |
| `warm-pool.yaml` | Warm pod pool | Instant startup from pre-provisioned pods, pool exhaustion with graceful degradation |
| `file-delivery.yaml` | File delivery & RAG | Files delivered to `/inbox/`, vector store access via RAG capability and env vars |
| `multi-tier.yaml` | Task hierarchy | Tribune → Centurion → Legionary delegation, capability inheritance, budget aggregation |
| `policy-override.yaml` | Policy & override precedence | AgentPolicy enforcement, denied capabilities, tier/budget violations, three-tier override model |
| `kitchen-sink.yaml` | All features combined | Every feature in one manifest — role+flavor, budget, retry, storage, Presidio, health, resources, env vars |

## Running Tests

Apply individually:
```bash
kubectl apply -f guardrails-pii.yaml
kubectl get agenttasks -n hortator-test -w
```

Apply all at once (note: some tests have ordering dependencies):
```bash
kubectl apply -f .
```

Check results:
```bash
# List all test tasks
kubectl get agenttasks -n hortator-test -l hortator.ai/test-suite

# Check a specific test suite
kubectl get agenttasks -n hortator-test -l hortator.ai/test-suite=retry

# View task details
kubectl describe agenttask retry-backoff-test -n hortator-test

# Check retry history
kubectl get agenttask retry-backoff-test -n hortator-test -o jsonpath='{.status.history}'
```

## Ordering Dependencies

Some manifests contain multiple resources that should be applied sequentially:

1. **storage-promotion.yaml** — Apply `storage-promotion-producer` first, wait for completion, then apply `storage-promotion-consumer`
2. **result-cache.yaml** — Apply `result-cache-miss` first, wait for completion, then apply `result-cache-hit` and `result-cache-skip`
3. **budget-limit.yaml** — Apply `budget-limit-test` first, wait for BudgetExceeded, then optionally apply `budget-limit-resume`

## Cleanup

```bash
kubectl delete namespace hortator-test
```
