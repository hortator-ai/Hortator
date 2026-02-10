# AgentRole Configuration

AgentRoles define behavioral archetypes for agents. They are referenced by name in `AgentTask.spec.role`.

**CRD YAML:** [`crds/agentrole.yaml`](https://github.com/michael-niemand/Hortator/blob/main/crds/agentrole.yaml)

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

## Gateway Integration

The OpenAI-compatible gateway exposes AgentRoles as "models". When a client requests `model: "hortator/backend-dev"`, the gateway resolves the role and creates an AgentTask with the role's configuration.
