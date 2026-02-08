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
  <a href="#roadmap">Roadmap</a>
</p>

---

## What is Hortator?

Hortator is a **Kubernetes operator** that lets AI agents spawn other AI agents — forming autonomous hierarchies to solve complex problems.

Think of it as **Kubernetes for AI workforces**: agents get isolated Pods, resource limits, network policies, budget caps, and health monitoring. They can spawn sub-agents, pass context, and report results — all orchestrated through K8s-native CRDs.

**Hortator doesn't care how your agents think.** It provides the infrastructure — isolation, spawning, governance, security. Build your agents with LangGraph, CrewAI, AutoGen, or plain Python. Hortator runs them.

### Why not just run agents in a single container?

| Single container | Hortator |
|---|---|
| All agents share one process | Each agent gets its own Pod |
| One agent crashes → everything crashes | Isolated failures |
| No resource limits per agent | CPU/memory limits per agent |
| No security boundaries | NetworkPolicy + RBAC per agent |
| Static agent count | Dynamic spawning (agents create agents) |
| One big context window | Per-agent context + structured handoffs |

## Quickstart

```bash
# Install the operator
helm repo add hortator https://charts.hortator.io
helm install hortator hortator/hortator \
  --namespace hortator-system --create-namespace \
  --set models.default.endpoint=https://api.anthropic.com/v1 \
  --set models.default.name=claude-sonnet \
  --set examples.enabled=true   # Install sample roles + hello-world task

# Watch your first agent run
kubectl get agenttasks -n hortator-demo -w
```

Or install examples separately:

```bash
helm install hortator hortator/hortator -n hortator-system --create-namespace
kubectl apply -f https://raw.githubusercontent.com/hortator/hortator/main/examples/quickstart/
```

## How It Works

### The Roman Hierarchy

Hortator uses a Roman military hierarchy — because the Hortator was a Roman galley officer, and because `tier: legionary` in YAML is just cool.

| Tier | Role | Storage | Model | Lifespan |
|------|------|---------|-------|----------|
| **Consul** | Strategic leadership. Decomposes complex problems. | PVC (persistent) | Expensive reasoning | Long-lived |
| **Centurion** | Coordinates a unit. Delegates to legionaries, collects results. | PVC (persistent) | Mid-tier | Medium |
| **Legionary** | Executes a single focused task. | EmptyDir (ephemeral) | Fast/cheap | Short-lived |

```
         ┌──────────┐
         │  Consul  │  "Redesign the auth system"
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

1. A **Consul** receives a complex task via `AgentTask` CRD
2. It uses the `hortator` CLI inside its Pod to spawn **Centurions**
3. Each Centurion spawns **Legionaries** for specific subtasks
4. Legionaries write results to `/outbox/result.json`
5. Operator copies results to parent's `/inbox/` and triggers the next turn
6. Results flow up the chain: Legionary → Centurion → Consul → done

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
│                    Kubernetes Cluster                  │
│                                                        │
│  ┌──────────────┐    watches   ┌───────────────────┐   │
│  │   Hortator   │◄────────────►│  AgentTask CRDs   │   │
│  │   Operator   │              │  AgentRole CRDs   │   │
│  └──────┬───────┘              └───────────────────┘   │
│         │ creates                                      │
│         ▼                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │  Agent Pod   │  │  Agent Pod   │  │  Agent Pod   │  │
│  │  (Consul)    │  │  (Centurion) │  │  (Legionary) │  │
│  │  ┌────────┐  │  │  ┌────────┐  │  │  ┌────────┐  │  │
│  │  │Runtime │  │  │  │Runtime │  │  │  │Runtime │  │  │
│  │  │+ CLI   │  │  │  │+ CLI   │  │  │  │+ CLI   │  │  │
│  │  └────────┘  │  │  └────────┘  │  │  └────────┘  │  │
│  │  ┌────────┐  │  │  ┌────────┐  │  │              │  │
│  │  │  PVC   │  │  │  │  PVC   │  │  │  (EmptyDir)  │  │
│  │  └────────┘  │  │  └────────┘  │  │              │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                        │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Optional: Presidio │ OTel Collector │ LiteLLM   │  │
│  └──────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────┘
```

**Written in Go** — first-class K8s ecosystem support.

## CRDs

### AgentTask

The core resource. Defines a task for an agent to execute.

```yaml
apiVersion: hortator.io/v1alpha1
kind: AgentTask
metadata:
  name: fix-auth-bug
  namespace: ai-team
spec:
  prompt: "Fix the session cookie not being set on login response"
  role: backend-dev
  flavor: "Use Drizzle ORM. Don't touch migrations. Bug is in session.ts:47."
  tier: legionary
  parentTaskId: feature-auth-refactor
  thinkingLevel: medium
  timeout: 600
  capabilities: [shell, web-fetch]
  budget:
    maxCostUsd: "0.50"
  resources:
    limits:
      cpu: "1"
      memory: 1Gi
```

### AgentRole / ClusterAgentRole

Defines behavioral archetypes for agents. Namespace-scoped (`AgentRole`) or cluster-wide (`ClusterAgentRole`).

```yaml
apiVersion: hortator.io/v1alpha1
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

See [`helm/values.yaml`](helm/values.yaml) for the full reference with comments.

## Built-in Guardrails

Hortator is designed for **autonomous agents with guardrails**:

- 🔒 **Security**: Per-agent NetworkPolicies, RBAC, capability inheritance (legionaries can't escalate beyond parent)
- 💰 **Budget**: Token/cost caps per task, powered by LiteLLM price map. Optional LiteLLM proxy for authoritative tracking.
- 🛡️ **PII Detection**: Presidio sidecar scans agent output for secrets, API keys, PII. Configurable action (redact/detect/hash/mask).
- 🏥 **Health Monitoring**: Behavioral stuck detection (tool diversity, prompt repetition, state staleness). Auto-kill or escalate stuck agents.
- 📊 **Observability**: Full OpenTelemetry integration. Task hierarchy = distributed trace. Budget + health metrics via Prometheus.
- 💾 **Context Management**: Structured extraction + summarization fallback. Graceful degradation when context window fills up ("agent reincarnation").

## CLI (for agents)

The `hortator` CLI ships inside the runtime container. Agents use it to spawn sub-agents and manage tasks:

```bash
hortator spawn --prompt "Fix the login bug" --role backend-dev --wait
hortator status <task-id>
hortator result <task-id>
hortator logs <task-id>
hortator cancel <task-id>
hortator list
hortator tree <task-id>            # Visualize task hierarchy
hortator retain --reason "..." --tags "auth,backend"  # Mark PVC for retention
hortator budget-remaining          # Check remaining budget
hortator progress --status "..."   # Self-report progress (for stuck detection)
```

## Roadmap

### MVP (P0)
- AgentTask CRD + basic operator (watch → create Job → track status)
- CLI: `spawn`, `status`, `result`, `spawn --wait`
- Default runtime container with standard filesystem layout
- Helm chart with sane defaults
- PVC provisioning for persistent tiers

### Next (P1)
- Task hierarchy (consul → centurion → legionary chains)
- TTL-based PVC cleanup + retention labels
- Prometheus metrics
- Security: NetworkPolicies from capabilities, RBAC
- Capability inheritance

### Future (P2)
- Presidio PII detection sidecar
- OpenTelemetry distributed tracing
- Budget enforcement with LiteLLM integration
- Stuck detection + auto-escalation
- Retained PVC knowledge discovery (tag matching → vector graduation)
- Multi-tenancy (namespace isolation, cross-namespace policies)
- Enterprise: AgentPolicy CRD, egress allowlists, OIDC/SSO

## Contributing

*(Coming soon)*

## License

MIT (core) — Enterprise features under separate license.

---

<p align="center">
  <em>"I don't row. I command the rowers."</em> — The Hortator
</p>
