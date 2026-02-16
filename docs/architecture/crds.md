# CRDs Reference

Hortator extends Kubernetes with three Custom Resource Definitions.

## CRD Source of Truth

CRD YAMLs are maintained in a single-source-of-truth workflow:

| CRD | Source | Generated from |
|-----|--------|----------------|
| AgentTask | `config/crd/bases/` | Go types in `api/v1alpha1/agenttask_types.go` via controller-gen |
| AgentPolicy | `config/crd/bases/` | Go types in `api/v1alpha1/agentpolicy_types.go` via controller-gen |
| AgentRole / ClusterAgentRole | `config/crd/bases/` | Go types in `api/v1alpha1/agentrole_types.go` via controller-gen |

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
| `inputFiles` | []InputFile | | Files delivered to `/inbox/` via init container (base64-encoded, ~1MB total limit) |
| `hierarchyBudget` | BudgetSpec | | Shared budget pool across the entire task tree (only meaningful on root tasks) |

### Status Phases

`Pending` → `Running` → `Waiting` → `Completed` | `Failed` | `BudgetExceeded` | `TimedOut` | `Cancelled`

With retry enabled: `Failed` → `Retrying` → `Pending` (up to `maxAttempts`)

The `Waiting` phase indicates the agent has checkpointed and is waiting for child tasks to complete (tribune/centurion reincarnation model).

## AgentRole / ClusterAgentRole (`core.hortator.ai/v1alpha1`)

Behavioral archetypes for agents. `AgentRole` is namespace-scoped, `ClusterAgentRole` is cluster-wide. Namespace-local takes precedence over cluster-wide for the same name.

**Go types:** [`api/v1alpha1/agentrole_types.go`](https://github.com/hortator-ai/Hortator/blob/main/api/v1alpha1/agentrole_types.go)
**CRD YAML:** [`crds/agentrole.yaml`](https://github.com/hortator-ai/Hortator/blob/main/crds/agentrole.yaml) (generated — do not edit directly)

### Key Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `defaultModel` | string | Default model name (e.g. `claude-sonnet-4-20250514`) |
| `defaultEndpoint` | string | Base URL for the LLM API |
| `apiKeyRef` | SecretKeyRef | Reference to a K8s Secret containing the API key |
| `tools` | []string | Default capabilities |
| `health` | HealthSpec | Per-role health/stuck detection overrides |

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
| `allowedShellCommands` | []string | Restrict which base commands agents can execute (first word checked). Empty = all allowed. |
| `deniedShellCommands` | []string | Block specific command prefixes (applied after allow list) |
| `readOnlyWorkspace` | bool | Makes `/workspace` read-only for analysis-only tasks |
