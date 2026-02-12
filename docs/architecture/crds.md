# CRDs Reference

Hortator extends Kubernetes with three Custom Resource Definitions.

## CRD Source of Truth

CRD YAMLs are maintained in a single-source-of-truth workflow:

| CRD | Source | Generated from |
|-----|--------|----------------|
| AgentTask | `config/crd/bases/` | Go types in `api/v1alpha1/agenttask_types.go` via controller-gen |
| AgentPolicy | `config/crd/bases/` | Go types in `api/v1alpha1/agentpolicy_types.go` via controller-gen |
| AgentRole / ClusterAgentRole | `crds/agentrole.yaml` | Hand-maintained (no Go types yet — see backlog L7) |

**`crds/`** aggregates all CRD YAMLs (generated + hand-written) and **`charts/hortator/crds/`** mirrors `crds/` for Helm installs.

Run `make sync-crds` to regenerate and sync across all locations. Run `make verify-crds` to check for drift. CI enforces sync on every pull request.

## AgentTask (`core.hortator.ai/v1alpha1`)

The core workload resource. Defines a task for an agent to execute.

**Go types:** [`api/v1alpha1/agenttask_types.go`](https://github.com/hortator-ai/Hortator/blob/main/api/v1alpha1/agenttask_types.go)
**CRD YAML:** [`crds/core.hortator.ai_agenttasks.yaml`](https://github.com/hortator-ai/Hortator/blob/main/crds/core.hortator.ai_agenttasks.yaml) (generated — do not edit directly)

### Key Spec Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `prompt` | string | *(required)* | Task instruction for the agent |
| `role` | string | | Reference to AgentRole/ClusterAgentRole by name |
| `tier` | enum | `legionary` | `tribune`, `centurion`, or `legionary` |
| `parentTaskId` | string | | Establishes hierarchy (children inherit capabilities) |
| `model` | ModelSpec | | LLM endpoint, model name, API key ref |
| `thinkingLevel` | enum | | Reasoning depth hint: `low`, `medium`, `high` |
| `flavor` | string | | Free-form addendum appended to role rules |
| `image` | string | | Custom container image (defaults to Helm `agent.image`) |
| `capabilities` | []string | | Permissions: `shell`, `web-fetch`, `spawn` |
| `timeout` | int | `600` | Timeout in seconds |
| `budget` | BudgetSpec | | `maxTokens` and/or `maxCostUsd` |
| `retry` | RetrySpec | | `maxAttempts`, `backoffSeconds`, `maxBackoffSeconds` |
| `resources` | ResourceRequirements | | CPU/memory requests and limits |
| `storage` | StorageSpec | | PVC size, storageClass, retain flag |
| `health` | HealthSpec | | Stuck detection overrides |
| `presidio` | PresidioSpec | | PII detection overrides |
| `env` | []EnvVar | | Environment variables (supports secretKeyRef) |

### Status Phases

`Pending` → `Running` → `Completed` | `Failed` | `BudgetExceeded` | `TimedOut` | `Cancelled`

With retry enabled: `Failed` → `Retrying` → `Pending` (up to `maxAttempts`)

## AgentRole / ClusterAgentRole (`core.hortator.ai/v1alpha1`)

Behavioral archetypes for agents. `AgentRole` is namespace-scoped, `ClusterAgentRole` is cluster-wide. Namespace-local takes precedence over cluster-wide for the same name.

**CRD YAML:** [`crds/agentrole.yaml`](https://github.com/hortator-ai/Hortator/blob/main/crds/agentrole.yaml) (hand-maintained — edit directly in `crds/`, then run `make sync-crds`)

> **Note:** Go types for AgentRole/ClusterAgentRole are not yet generated — the gateway uses `unstructured.Unstructured`. See backlog item L7.

### Key Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Human-readable role description |
| `rules` | []string | Behavioral rules injected into agent prompt |
| `antiPatterns` | []string | Things the agent should avoid |
| `tools` | []string | Default capabilities |
| `defaultModel` | string | Default model for this role |
| `references` | []string | URLs for reference documentation |

## AgentPolicy (`core.hortator.ai/v1alpha1`) *(Enterprise)*

Namespace-scoped governance constraints. Tasks must comply with all matching policies.

**Go types:** [`api/v1alpha1/agentpolicy_types.go`](https://github.com/hortator-ai/Hortator/blob/main/api/v1alpha1/agentpolicy_types.go)
**CRD YAML:** [`crds/core.hortator.ai_agentpolicies.yaml`](https://github.com/hortator-ai/Hortator/blob/main/crds/core.hortator.ai_agentpolicies.yaml) (generated — do not edit directly)

### Key Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `maxTier` | enum | Highest tier allowed (`legionary`, `centurion`, `tribune`) |
| `maxTimeout` | int | Maximum timeout in seconds |
| `maxConcurrentTasks` | int | Active task limit per namespace |
| `allowedCapabilities` | []string | Capability whitelist |
| `deniedCapabilities` | []string | Capability blacklist (overrides allowed) |
| `allowedImages` | []string | Glob patterns for permitted container images |
| `maxBudget` | BudgetSpec | Maximum budget any task can request |
| `requirePresidio` | bool | Force PII scanning on all tasks |
| `egressAllowlist` | []EgressRule | Outbound network restrictions (host + ports) |
| `namespaceSelector` | LabelSelector | Which namespaces this policy applies to |
