# Core Concepts

## The Roman Hierarchy

Hortator is named after the officer on Roman galleys who commanded the rowers. The hierarchy follows the Roman military theme:

### Consul

The strategic leader. Receives a complex problem, breaks it down, and delegates to centurions.

- **Storage:** Persistent Volume Claim (PVC) — survives across turns
- **Model:** Expensive reasoning models (Opus, GPT-4)
- **Lifespan:** Long-lived, persists until the mission is complete

### Centurion

The unit commander. Coordinates a group of legionaries, collects their results, and reports to the consul.

- **Storage:** Persistent Volume Claim (PVC) — survives across turns
- **Model:** Mid-tier models (Sonnet, GPT-4o)
- **Lifespan:** Medium, active for a phase of work

### Legionary

The soldier. Executes a single, focused task. Expendable — spawned for a job, terminated when done.

- **Storage:** EmptyDir — deleted with the Pod
- **Model:** Fast/cheap models (Haiku, GPT-4o-mini, local Ollama)
- **Lifespan:** Short, typically minutes

```
         ┌──────────┐
         │  Consul   │  "Redesign the auth system"
         └────┬─────┘
              │
    ┌─────────┼─────────┐
    │         │         │
┌───▼──┐ ┌───▼──┐ ┌───▼──┐
│Centur.│ │Centur.│ │Centur.│  "Handle backend" / "Handle frontend" / "Handle tests"
└───┬──┘ └───┬──┘ └───┬──┘
    │         │         │
  ┌─▼─┐   ┌─▼─┐   ┌─▼─┐
  │Leg.│   │Leg.│   │Leg.│    "Fix session.ts:47" / "Update login form" / "Write e2e test"
  └───┘   └───┘   └───┘
```

## CRDs

Hortator extends Kubernetes with three Custom Resource Definitions:

### AgentTask

The core resource. Defines a task for an agent to execute.

```yaml
apiVersion: hortator.io/v1alpha1
kind: AgentTask
metadata:
  name: fix-auth-bug
spec:
  prompt: "Fix the session cookie not being set on login"
  role: backend-dev
  tier: legionary
  timeout: 600
```

### AgentRole (namespaced)

Defines behavioral rules for a type of agent. Scoped to a namespace — teams can customize without affecting others.

### ClusterAgentRole (cluster-wide)

Same as AgentRole but cluster-scoped. Shared standard roles across all namespaces.

**Resolution:** When a task references a role by name, the operator looks up the namespace-local `AgentRole` first, then falls back to `ClusterAgentRole`.

## Agent Communication

Agents don't talk to each other directly. The **operator is the broker**:

1. Legionary completes → writes `/outbox/result.json`
2. Operator detects completion
3. Operator copies result to parent centurion's `/inbox/`
4. Operator spawns the centurion's next turn (new Job, same PVC)

## The Filesystem Contract

Every agent Pod has four mount points:

| Path | Who writes | Who reads | Purpose |
|------|-----------|-----------|---------|
| `/inbox/` | Operator | Agent | Task definition, context, prior work |
| `/outbox/` | Agent | Operator | Results, artifacts, usage report |
| `/memory/` | Agent | Agent | Persistent state across turns |
| `/workspace/` | Agent | Agent | Scratch space for temporary files |

## Three-Tier Override

Configuration flows through three levels (most specific wins):

```
Helm values.yaml → AgentRole CRD → AgentTask CRD
  (cluster defaults)  (role norms)    (task specifics)
```

This applies to: model selection, health thresholds, Presidio config, budget limits, and capabilities.

## The Hortator CLI

The `hortator` CLI ships **inside the runtime container**. It's for agents to use, not humans:

```bash
hortator spawn --prompt "..." --role backend-dev --wait
hortator status <task-id>
hortator result <task-id>
hortator retain --reason "Important findings" --tags "auth"
hortator budget-remaining
```

Humans use `kubectl` and `helm`. Agents use `hortator`.
