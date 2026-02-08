# Agent Roles System

## Concept

Spawned agents need predefined roles that shape their behavior. A role is an archetype (e.g. Backend Dev, Frontend Dev, QA Engineer) with baked-in rules and constraints.

## Decisions

### 1. Where do roles live? → **CRDs (K8s-native)**

Roles are `AgentRole` CRDs — cluster-scoped, versioned in git, deployed via GitOps (Flux/ArgoCD). No file copying, no SCP, no hoping the right markdown ended up on the right node.

Composes naturally with `AgentTask` and `AgentPolicy` as the third pillar of the Hortator CRD family.

```yaml
apiVersion: hortator.io/v1alpha1
kind: ClusterAgentRole
metadata:
  name: backend-dev
spec:
  description: "Backend developer with TDD focus"
  rules:
    - "Always write tests before implementation"
    - "Follow API design patterns (REST, proper status codes)"
    - "Security best practices (input validation, auth checks)"
    - "Proper error handling with meaningful messages"
  antiPatterns:
    - "Never use `any` in TypeScript"
    - "Don't install new dependencies without checking existing ones first"
  tools:
    - shell
    - web-fetch
  defaultModel: sonnet
  references:
    - "https://internal-docs.example.com/api-guidelines"
```

### 2. How does per-task flavor compose with the role? → **Free-form append**

The role provides the base rules. The flavor is free-form text appended as extra context — like an addendum. No structured override mechanism at MVP. Can add field-level overrides later if a pattern emerges.

```yaml
apiVersion: hortator.io/v1alpha1
kind: AgentTask
metadata:
  name: fix-auth-bug-42
spec:
  role: backend-dev          # ← pulls in AgentRole CRD rules
  flavor: |                   # ← free-form addendum
    Use the existing Drizzle ORM schema.
    Don't touch migrations.
    The bug is in packages/api/src/routes/auth.ts line 47.
  prompt: "Fix the session cookie not being set on login response"
  tier: legionary             # ← tribune | centurion | legionary
  thinkingLevel: medium       # ← task-level, not role-level
  timeout: 600
```

The operator resolves the role → injects rules into agent context → appends flavor → starts the job.

### 3. How are roles injected into agents? → **Operator handles it**

The operator reads the `AgentRole` CRD referenced in the task, and injects the role's rules + flavor into the agent's startup context (e.g. as a ROLE.md mounted into the container, or prepended to the prompt). No agent-side logic needed.

### 4. Thinking level → **Task-level, not role-level**

`thinkingLevel` belongs on `AgentTask`, not `AgentRole`. The same backend-dev role might do a simple rename (low thinking) or design a new auth system (high thinking). The role sets `defaultModel` as a baseline; the task overrides model and thinking when needed.

### 5. Scope → **Dual-scoped (like cert-manager Issuer/ClusterIssuer)**

Two CRDs:

- **`ClusterAgentRole`** — cluster-scoped. Company-wide standard roles (`backend-dev`, `qa-engineer`). Defined by platform team, available to all namespaces.
- **`AgentRole`** — namespace-scoped. Team-owned variants. A team can define their own `backend-dev` in their namespace without polluting the global namespace or resorting to `backend-dev-2-final-edit-new`.

**Resolution:** When an `AgentTask` references a role by name, the operator looks up namespace-local `AgentRole` first, falls back to `ClusterAgentRole`. Namespace-local **fully replaces** the cluster role (no inheritance/merging at MVP — that's a rabbit hole). Teams can copy-paste the cluster role as a starting point and customize from there.

This mirrors the `Issuer`/`ClusterIssuer` pattern from cert-manager and `Role`/`ClusterRole` from K8s RBAC. Clean multi-tenancy without naming gymnastics.

### 6. Cardinality → **Single role per task**

One role per `AgentTask`. No multi-role composition. Reasons:
- Multiple roles require merge/conflict resolution logic (rabbit hole)
- Composing rules from two CRDs into one coherent prompt is hard to debug
- The flavor field already handles "I need a bit of X mixed in"
- Need a security-conscious backend dev? Make `backend-dev-secure` or use flavor.

## AgentRole CRD Spec

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Human-readable role description |
| `rules` | string[] | Behavioral rules injected into agent context |
| `antiPatterns` | string[] | Things the agent should explicitly NOT do |
| `tools` | string[] | Capabilities granted (shell, web-fetch, spawn, etc.) |
| `defaultModel` | string | Suggested model (task can override) |
| `references` | string[] | URLs or paths the agent should consult |

## Example Roles (will become actual CRDs)

- **backend-dev:** TDD, error handling, API design patterns, security best practices
- **frontend-dev:** Component architecture, accessibility, responsive design, UX patterns
- **qa-engineer:** Test coverage, edge cases, regression testing, acceptance criteria
- **devops:** Infrastructure, CI/CD, monitoring, deployment automation
