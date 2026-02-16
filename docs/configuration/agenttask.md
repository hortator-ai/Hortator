# AgentTask Configuration

For the full field reference, see the [CRDs documentation](../architecture/crds.md#agenttask-corehortatoraiv1alpha1).

**Go types:** [`api/v1alpha1/agenttask_types.go`](https://github.com/hortator-ai/Hortator/blob/main/api/v1alpha1/agenttask_types.go)

## Example

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: fix-auth-bug
  namespace: ai-team
spec:
  prompt: "Fix the session cookie not being set on login response"
  tier: legionary
  role: backend-dev
  parentTaskId: feature-auth-refactor
  timeout: 600
  capabilities: [shell, web-fetch]
  thinkingLevel: low
  budget:
    maxTokens: 100000
    maxCostUsd: "0.50"
  model:
    name: claude-sonnet
  storage:
    size: 1Gi
    storageClass: fast-ssd
    retain: false
  retry:
    maxAttempts: 3
    backoffSeconds: 30
    maxBackoffSeconds: 300
  env:
    - name: ANTHROPIC_API_KEY
      valueFrom:
        secretKeyRef:
          secretName: llm-keys
          key: anthropic
  resources:
    requests:
      cpu: "100m"
      memory: 128Mi
    limits:
      cpu: "1"
      memory: 1Gi
```

## Status Phases

| Phase | Meaning |
|-------|---------|
| `Pending` | Task created, waiting for Pod |
| `Running` | Agent Pod is executing |
| `Waiting` | Agent checkpointed, waiting for children to complete |
| `Completed` | Agent finished successfully |
| `Failed` | Agent reported failure or exhausted retries |
| `BudgetExceeded` | Token or cost budget exceeded |
| `TimedOut` | Timeout elapsed |
| `Cancelled` | Manually cancelled |
| `Retrying` | Transient failure, waiting for backoff before next attempt |
