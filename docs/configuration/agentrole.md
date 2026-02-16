# AgentRole Configuration

AgentRoles define behavioral archetypes for agents. They are referenced by name in `AgentTask.spec.role`.

**CRD YAML:** [`crds/agentrole.yaml`](https://github.com/hortator-ai/Hortator/blob/main/crds/agentrole.yaml)

## Scoping

- **AgentRole** — namespace-scoped. Teams can customize without affecting others.
- **ClusterAgentRole** — cluster-wide. Shared standard roles across all namespaces.

**Resolution:** When a task references a role by name, the operator looks up the namespace-local `AgentRole` first, then falls back to `ClusterAgentRole`.

## Example

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: ClusterAgentRole
metadata:
  name: backend-dev
spec:
  tools: [shell, web-fetch]
  defaultModel: claude-sonnet
  defaultEndpoint: https://api.anthropic.com/v1
  apiKeyRef:
    secretName: anthropic-key
    key: api-key
```

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `defaultModel` | string | Default model name (e.g. `claude-sonnet-4-20250514`) |
| `defaultEndpoint` | string | Base URL for the LLM API |
| `apiKeyRef` | SecretKeyRef | Reference to a K8s Secret containing the API key (`secretName` + `key`) |
| `tools` | []string | Default capabilities granted to tasks using this role |
| `health` | HealthSpec | Per-role health/stuck detection overrides (see below) |

## Per-Role Health Overrides

The `health` field allows role-specific stuck detection configuration. This sits between cluster defaults (ConfigMap) and per-task overrides in the resolution cascade:

**ConfigMap defaults -> AgentRole -> AgentTask** (most specific wins)

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: ClusterAgentRole
metadata:
  name: qa-engineer
spec:
  defaultModel: claude-sonnet-4-20250514
  tools: [shell, web-fetch]
  health:
    stuckDetection:
      # QA engineers naturally repeat tests, so allow more prompt repetition
      maxRepeatedPrompts: 8
      toolDiversityMin: 0.15
      action: warn
```

### Health Fields

| Field | Type | Description |
|-------|------|-------------|
| `health.stuckDetection.toolDiversityMin` | float64 | Minimum tool diversity ratio (0-1). Lower = more lenient. |
| `health.stuckDetection.maxRepeatedPrompts` | int | Max identical prompts before flagging as stuck. |
| `health.stuckDetection.statusStaleMinutes` | int | Minutes without progress before staleness penalty. |
| `health.stuckDetection.action` | string | Action on stuck: `warn`, `kill`, or `escalate`. |

## Gateway Integration

The OpenAI-compatible gateway exposes AgentRoles as "models". When a client requests `model: "hortator/backend-dev"`, the gateway resolves the role and creates an AgentTask with the role's configuration.
