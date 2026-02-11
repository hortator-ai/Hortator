<p align="center">
  <h1 align="center">⚔️ Hortator</h1>
  <p align="center"><strong>Kubernetes-native orchestration for autonomous AI agents</strong></p>
  <p align="center">
    <em>Named after the officer on Roman galleys who commanded the rowers — orchestrates agents without doing the thinking.</em>
  </p>
</p>

<p align="center">
  <a href="#quickstart">Quickstart</a> •
  <a href="#how-it-works">How It Works</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#crds">CRDs</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#faq">FAQ</a> •
  <a href="#roadmap">Roadmap</a>
</p>

---

## What is Hortator?

Hortator is a **Kubernetes operator** that lets AI agents spawn other AI agents — forming autonomous hierarchies with guardrails to solve complex problems.

It provides the **infrastructure layer** — isolation, lifecycle management, budget enforcement, security, and health monitoring — so agents can focus on thinking. Build your agents with LangGraph, CrewAI, AutoGen, or plain Python. Hortator runs them safely.

### Why not just run agents in a single container?

| Single container | Hortator |
|---|---|
| All agents share one process | Each agent gets its own Pod |
| One agent crashes → everything crashes | Isolated failures |
| No resource limits per agent | CPU/memory limits per agent |
| No security boundaries | NetworkPolicy + RBAC per agent |
| Static agent count | Dynamic spawning (agents create agents) |
| One big context window | Per-agent context + structured handoffs |

## Prerequisites

- **Kubernetes** 1.28+ (RKE2, K3s, EKS, GKE, etc.)
- **Helm** 3.x
- **Default StorageClass** — required for tribune/centurion tiers. RKE2/K3s don't ship one by default; install [local-path-provisioner](https://github.com/rancher/local-path-provisioner):
  ```bash
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **LLM API key** — Anthropic, OpenAI, or any OpenAI-compatible endpoint

## Quickstart

```bash
# Install the operator
helm install hortator oci://ghcr.io/michael-niemand/hortator/charts/hortator \
  --namespace hortator-system --create-namespace \
  --set models.default.endpoint=https://api.anthropic.com/v1 \
  --set models.default.name=claude-sonnet-4-20250514

# Create a secret with your API key
kubectl create namespace hortator-demo
kubectl create secret generic anthropic-api-key \
  --namespace hortator-demo \
  --from-literal=api-key=sk-ant-...

# Run your first task
kubectl apply -f examples/quickstart/hello-world.yaml

# Watch it
kubectl get agenttasks -n hortator-demo -w
kubectl logs -n hortator-demo -l hortator.ai/task=hello-world -c agent
```

## How It Works

### The Roman Hierarchy

Hortator uses a Roman military hierarchy — because the Hortator was a Roman galley officer, and because `tier: legionary` in YAML is just cool.

| Tier | Role | Storage | Model | Lifespan |
|------|------|---------|-------|----------|
| **Tribune** | Strategic leadership. Decomposes complex problems. | PVC (persistent) | Expensive reasoning | Long-lived |
| **Centurion** | Coordinates a unit. Delegates to legionaries, collects results. | PVC (persistent) | Mid-tier | Medium |
| **Legionary** | Executes a single focused task. | PVC (256Mi default) | Fast/cheap | Short-lived |

```
         ┌──────────┐
         │  Tribune  │  "Redesign the auth system"
         └────┬─────┘
              │
    ┌─────────┼─────────┐
    │         │         │
┌───▼───┐ ┌───▼───┐ ┌───▼───┐
│Centur.│ │Centur.│ │Centur.│  "Handle backend" / "Handle frontend" / "Handle tests"
└───┬───┘ └───┬───┘ └───┬───┘
    │         │         │
 ┌──▼──┐   ┌──▼──┐   ┌──▼──┐
 │ Leg.│   │ Leg.│   │ Leg.│    Focused tasks: "Fix session.ts:47" / "Update login form" / ...
 └─────┘   └─────┘   └─────┘
```

### The Flow

1. A **Tribune** receives a complex task via `AgentTask` CRD
2. It uses the `hortator` CLI inside its Pod to spawn **Centurions**
3. Each Centurion spawns **Legionaries** for specific subtasks
4. Legionaries write results to `/outbox/result.json`
5. Operator copies results to parent's `/inbox/` and triggers the next turn
6. Results flow up the chain: Legionary → Centurion → Tribune → done

### Agent Communication

Agents don't talk to each other directly. The **operator is the broker**:

```
Legionary completes
  → Writes /outbox/result.json
  → Operator detects completion
  → Copies result to parent Centurion's /inbox/
  → Spawns parent Centurion's next turn (new Job, same PVC)
```

Each agent Pod has four mount points:

| Path | Purpose |
|------|---------|
| `/inbox/` | Task definition + context from parent (operator writes) |
| `/outbox/` | Results + artifacts for parent (agent writes) |
| `/memory/` | Persistent state across turns (agent reads/writes) |
| `/workspace/` | Scratch space for temporary files |

## Architecture

```
┌────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                   │
│                                                        │
│  ┌──────────────┐    watches   ┌───────────────────┐   │
│  │   Hortator   │◄────────────►│  AgentTask CRDs   │   │
│  │   Operator   │              │  AgentRole CRDs   │   │
│  └──────┬───────┘              │  AgentPolicy CRDs │   │
│         │ creates              └───────────────────┘   │
│         ▼                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │  Agent Pod   │  │  Agent Pod   │  │  Agent Pod   │  │
│  │  (Tribune)   │  │  (Centurion) │  │  (Legionary) │  │
│  │  ┌────────┐  │  │  ┌────────┐  │  │  ┌────────┐  │  │
│  │  │Runtime │  │  │  │Runtime │  │  │  │Runtime │  │  │
│  │  │+ CLI   │  │  │  │+ CLI   │  │  │  │+ CLI   │  │  │
│  │  └────────┘  │  │  └────────┘  │  │  └────────┘  │  │
│  │  ┌────────┐  │  │  ┌────────┐  │  │              │  │
│  │  │  PVC   │  │  │  │  PVC   │  │  │  │  PVC   │  │  │
│  │  └────────┘  │  │  └────────┘  │  │              │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                        │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Optional: Presidio │ OTel Collector │ LiteLLM   │  │
│  └──────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────┘
```

**Written in Go** — first-class K8s ecosystem support.

**Two personas interact with Hortator:**

| Who | Interface | What they do |
|-----|-----------|--------------|
| **Platform engineers** | Helm values, YAML CRDs | Configure operator, define roles/policies, set budgets |
| **Agents** | `hortator` CLI inside Pods | Spawn sub-agents, check status, report results |

Agents never touch YAML. The CLI creates CRDs under the hood.

**Security layers:**
- **NetworkPolicies** — automatically generated from agent capabilities (e.g., `web-fetch` opens egress, `shell` stays isolated)
- **RBAC** — agents get minimal permissions (only create AgentTasks in their own namespace); capability inheritance prevents escalation
- **Namespace restrictions** — optionally require `core.hortator.ai/enabled=true` label on namespaces (via `enforceNamespaceLabels` in Helm values)

**Observability:**
- **Prometheus metrics** — `hortator_tasks_total`, `hortator_tasks_active`, `hortator_task_duration_seconds`
- **OpenTelemetry** — task hierarchy maps to distributed traces; audit events emitted as OTel spans for Jaeger/Tempo/Datadog

## CRDs

### AgentTask

The core resource. Defines a task for an agent to execute.

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: fix-auth-bug
  namespace: ai-team
spec:
  prompt: "Fix the session cookie not being set on login response"
  tier: legionary
  parentTaskId: feature-auth-refactor
  timeout: 600
  capabilities: [shell, web-fetch]
  thinkingLevel: low            # Reasoning depth hint (low/medium/high)
  budget:
    maxTokens: 100000
    maxCostUsd: "0.50"
  model:
    name: claude-sonnet
  storage:
    size: 1Gi
    storageClass: fast-ssd
    retain: false
  env:
    - name: ANTHROPIC_API_KEY
      valueFrom:
        secretKeyRef:
          name: llm-keys
          key: anthropic
  resources:
    requests:
      cpu: "100m"
      memory: 128Mi
    limits:
      cpu: "1"
      memory: 1Gi
```

**Key fields:**
- `tier` — tribune / centurion / legionary (determines storage and default model)
- `capabilities` — `shell`, `web-fetch`, `spawn` (maps to NetworkPolicies)
- `thinkingLevel` — per-task reasoning depth hint (`low` / `medium` / `high`)
- `flavor` — free-form addendum appended to the role's rules (task-specific context without a new role)
- `image` — custom container image (defaults to Hortator runtime from Helm values)
- `parentTaskId` — establishes hierarchy; children inherit and cannot escalate beyond parent capabilities

**Status phases:** `Pending` → `Running` → `Waiting` → `Completed` | `Failed` | `BudgetExceeded` | `TimedOut` | `Cancelled`

### AgentRole / ClusterAgentRole

Defines behavioral archetypes for agents. Namespace-scoped (`AgentRole`) or cluster-wide (`ClusterAgentRole`).

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

**Resolution:** Namespace-local `AgentRole` takes precedence over `ClusterAgentRole` with the same name.

### AgentPolicy *(Enterprise)*

Namespace-scoped governance constraints. Enforces limits that individual tasks cannot override.

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: production-restrictions
  namespace: ai-team
spec:
  maxTier: centurion                    # No tribunes in this namespace
  maxTimeout: 1800                      # 30 min max
  maxConcurrentTasks: 10
  allowedCapabilities: [shell, web-fetch]
  deniedCapabilities: [spawn]           # Overrides allowed
  allowedImages: ["ghcr.io/hortator/*"]
  requirePresidio: true
  maxBudget:
    maxCostUsd: "5.00"
  egressAllowlist:
    - host: api.anthropic.com
      ports: [443]
```

## Configuration

All configuration lives in Helm `values.yaml` — transparent, GitOps-friendly, no custom images needed.

**Three-tier override:** Helm defaults → AgentRole → AgentTask (most specific wins).

Key configuration areas:

| Area | What it controls |
|------|-----------------|
| `models.*` | LLM endpoint, model name, API keys, presets (Ollama/vLLM/LiteLLM) |
| `budget.*` | Cost tracking, price source (LiteLLM map), per-task limits |
| `presidio.*` | PII detection sidecar, recognizers, scan thresholds |
| `telemetry.*` | OpenTelemetry audit events, distributed traces, Prometheus metrics |
| `health.*` | Stuck detection thresholds, behavioral analysis, per-role overrides |
| `storage.*` | PVC cleanup TTLs, retention, knowledge discovery, quotas |
| `security.*` | Capabilities → NetworkPolicy mapping, RBAC |
| `examples.*` | Install quickstart examples (off by default) |

See [`charts/hortator/values.yaml`](charts/hortator/values.yaml) for the full reference with comments.

## Built-in Guardrails

Hortator is designed for **autonomous agents with guardrails**:

- 🔒 **Security**: Per-agent NetworkPolicies, RBAC, capability inheritance (children can't escalate beyond parent)
- 💰 **Budget**: Token/cost caps per task, powered by LiteLLM price map. Optional LiteLLM proxy for authoritative tracking.
- 🛡️ **PII Detection** *(Enterprise)*: Centralized Presidio service scans agent output for secrets, API keys, PII. Configurable action (redact/detect/hash/mask).
- 🏥 **Health Monitoring**: Behavioral stuck detection (tool diversity, prompt repetition, state staleness). Auto-kill or escalate stuck agents.
- 📊 **Observability**: Full OpenTelemetry integration. Task hierarchy = distributed trace. Budget + health metrics via Prometheus.
- 💾 **Context Management**: Structured extraction + summarization fallback. Graceful degradation when context windows fill up ("agent reincarnation").

## CLI (for agents)

The `hortator` CLI ships inside the runtime container. Agents use it to interact with the operator — they never write YAML.

```bash
hortator spawn --prompt "Fix the login bug" --role backend-dev --wait
hortator spawn --prompt "..." --cap shell,web-fetch --tier legionary
hortator status <task-id>              # Check task phase
hortator status <task-id> -o json      # JSON output for scripting
hortator result <task-id>              # Get task output
hortator logs <task-id>                # Stream/fetch worker logs
hortator cancel <task-id>              # Terminate a running task
hortator list                          # List tasks in namespace
hortator list -o json                  # JSON output for automation
hortator tree <task-id>                # Visualize task hierarchy (parent/children)
hortator retain --reason "..." --tags "auth,backend"  # Mark PVC for retention
hortator budget-remaining              # Check remaining budget
hortator progress --status "..."       # Self-report progress (for stuck detection)
```

## FAQ

**"This is just Kubernetes Jobs with extra steps."** — At the lowest level, yes. The value is the lifecycle management around the Job: result brokering, PVC lifecycle, budget enforcement, stuck detection, context management, security. Same argument for any K8s operator.

**"Why not Argo Workflows / Tekton?"** — Argo defines static DAGs upfront. Hortator tasks are dynamically spawned by agents at runtime. The task tree emerges from the work, it's not predetermined. Argo is CI/CD infrastructure. Hortator is agent infrastructure.

**"Agents spawning agents is terrifying."** — This is exactly why Hortator exists. Without guardrails, agents are already spawning Docker containers and SSH-ing into machines. Hortator adds capability inheritance, NetworkPolicies, budget caps, and namespace isolation.

**"This doesn't make my agents smarter."** — Correct. Hortator is infrastructure, not an AI framework. It stops good agents from failing for infrastructure reasons: context exhaustion, runaway costs, no isolation, no monitoring.

See [full FAQ](docs/faq.md) for more.

## Roadmap

### ✅ Done
- AgentTask CRD + operator (watch → create Pod → track status)
- CLI: `spawn`, `status`, `result`, `logs`, `cancel`, `list`, `tree`
- Default runtime container with standard filesystem layout
- Helm chart with sane defaults
- Task hierarchy (tribune → centurion → legionary chains)
- TTL-based PVC cleanup + retention labels
- Prometheus metrics
- Security: NetworkPolicies from capabilities, RBAC, capability inheritance
- OpenTelemetry distributed tracing
- Namespace restrictions (`enforceNamespaceLabels`)
- JSON output for all CLI commands
- Retry semantics with jitter (transient vs logical failure classification)
- AgentPolicy CRD *(Enterprise)*
- Presidio PII detection (centralized Deployment+Service) *(Enterprise)*
- CI/CD pipeline (GitHub Actions, linting, Dependabot)
- OpenAI-compatible API gateway (`/v1/chat/completions`, SSE streaming)
- Controller refactor (split into focused files: pod builder, policy, helpers, metrics)
- Comprehensive unit tests (controller, gateway, helpers, pod builder, policy, warm pool, result cache)
- Warm Pod pool for sub-second task assignment (opt-in)
- Content-addressable result cache with LRU eviction (opt-in)
- Python SDK (`hortator` package — sync/async, streaming, LangChain + CrewAI integrations)
- TypeScript SDK (`@hortator/sdk` — zero deps, streaming, LangChain.js integration)

### Next
- Python agentic runtime for tribune/centurion tiers (tool-calling loop, checkpoint/restore)
- Reincarnation model (event-driven Tribune lifecycle with `Waiting` phase)
- Artifact download endpoint (`GET /api/v1/tasks/{id}/artifacts`)
- Async task submission (`X-Hortator-Async` header)
- Budget enforcement with LiteLLM integration
- Stuck detection + auto-escalation
- Retained PVC knowledge discovery (tag matching → vector graduation)
- Multi-tenancy (cross-namespace policies)
- `hortator watch` TUI for live task tree visualization

### Future
- Object storage archival for completed task artifacts
- OIDC/SSO *(Enterprise)*
- Web dashboard for task hierarchy visualization
- Go SDK

## Contributing

*(Coming soon)*

## License

MIT (core) — Enterprise features under separate license.

---

<p align="center">
  <em>"I don't row. I command the rowers."</em> — The Hortator
</p>
