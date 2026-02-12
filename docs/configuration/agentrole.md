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
  description: "Backend developer with TDD focus"
  rules:
    - "Always write tests before implementation"
    - "Security best practices (input validation, auth checks)"
    - "Proper error handling with meaningful messages"
  antiPatterns:
    - "Never use `any` in TypeScript"
    - "Don't install new dependencies without checking existing ones"
  tools: [shell, web-fetch]
  defaultModel: claude-sonnet
  references:
    - "https://internal-docs.example.com/api-guidelines"
```

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Human-readable role description |
| `rules` | []string | Behavioral rules injected into the agent's prompt |
| `antiPatterns` | []string | Things the agent should avoid |
| `tools` | []string | Default capabilities granted to tasks using this role |
| `defaultModel` | string | Default model name |
| `references` | []string | URLs for reference documentation |
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
