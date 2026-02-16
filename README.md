<p align="center">
  <h1 align="center">⚔️ Hortator</h1>
  <p align="center"><strong>A Kubernetes operator that lets AI agents spawn AI agents.</strong></p>
</p>

<p align="center">
  <a href="#the-problem">The Problem</a> •
  <a href="#what-hortator-does">What Hortator Does</a> •
  <a href="#quickstart">Quickstart</a> •
  <a href="#how-it-works">How It Works</a> •
  <a href="#guardrails">Guardrails</a> •
  <a href="#crds">CRDs</a> •
  <a href="#faq">FAQ</a> •
  <a href="#roadmap">Roadmap</a>
</p>

<p align="center">
  <a href="https://github.com/hortator-ai/Hortator/actions/workflows/ci.yaml"><img src="https://github.com/hortator-ai/Hortator/actions/workflows/ci.yaml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://github.com/hortator-ai/Hortator/releases"><img src="https://img.shields.io/github/v/release/hortator-ai/Hortator?style=flat-square&color=blue" alt="Release"></a>
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License: MIT">
  <img src="https://img.shields.io/badge/language-Go-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/kubernetes-1.28+-326CE5?style=flat-square&logo=kubernetes&logoColor=white" alt="Kubernetes 1.28+">
</p>

---

## The Problem

AI agents today run in one of two modes: **sandboxed toys** (single container, no real autonomy) or **terrifying cowboys** (SSH into prod, spawn Docker containers, `curl | bash` whatever they want).

There's no middle ground. No infrastructure that says: *"Yes, you can spawn sub-agents, decompose problems, and work autonomously - but within boundaries I define."*

Hortator is that middle ground.

## What Hortator Does

Hortator is a **Kubernetes operator** that gives AI agents the ability to create other AI agents at runtime - forming dynamic task hierarchies to solve complex problems. Each agent runs in its own Pod with its own context, budget, and security boundary.

You define the guardrails. Agents do the thinking.

```
        ┌───────────┐
        │  Tribune  │  "Redesign the auth system"
        └─────┬─────┘
              │
    ┌─────────┼─────────┐
    │         │         │
┌───▼───┐ ┌───▼───┐ ┌───▼───┐
│Centur.│ │Centur.│ │Centur.│  "Handle backend" / "Handle frontend" / "Handle tests"
└───┬───┘ └───┬───┘ └───┬───┘
    │         │         │
 ┌──▼──┐   ┌──▼──┐   ┌──▼──┐
 │ Leg.│   │ Leg.│   │ Leg.│    "Fix session.ts:47" / "Update login form" / ...
 └─────┘   └─────┘   └─────┘
```

The task tree isn't defined upfront - it **emerges** from the work. A Tribune decides it needs three Centurions. A Centurion decides it needs five Legionaries. Hortator manages the lifecycle, result passing, and cleanup.

**Key idea:** Agents never see YAML. They use a CLI (`hortator spawn`, `hortator result`) inside their Pod. The operator handles everything else.

### What makes this different from [Argo / Tekton / CrewAI / LangGraph]?

- **Argo/Tekton** define static DAGs upfront. Hortator task trees are dynamic - agents decide the structure at runtime.
- **CrewAI/LangGraph** are Python frameworks that run agents in-process. Hortator gives each agent its own Pod, PVC, network policy, and budget. They're complementary - you can run CrewAI *inside* a Hortator agent.
- **Raw Kubernetes Jobs** are the primitive Hortator builds on. The value is everything around the Job: result brokering between parent/child, PVC lifecycle, budget enforcement, stuck detection, PII redaction, security policies.

## Quickstart

```bash
# Install the operator
helm install hortator oci://ghcr.io/hortator-ai/hortator/charts/hortator \
  --namespace hortator-system --create-namespace \
  --set models.default.endpoint=https://api.anthropic.com/v1 \
  --set models.default.name=claude-sonnet-4-20250514

# Create a namespace and API key secret
kubectl create namespace hortator-demo
kubectl create secret generic llm-api-key \
  --namespace hortator-demo \
  --from-literal=api-key=sk-ant-...

# Run your first task
kubectl apply -f examples/quickstart/hello-world.yaml

# Watch it work
kubectl get agenttasks -n hortator-demo -w
```

That's it. The operator creates a Pod, injects your prompt, runs the agent, collects the result, and updates the CRD status.

For multi-tier examples (Tribune → Centurion → Legionary chains), see [`examples/advanced/`](examples/advanced/).

## How It Works

### The Roman Hierarchy

*Named after the officer on Roman galleys who commanded the rowers - Hortator orchestrates agents without doing the thinking.*

Three tiers, inspired by the Roman military:

| Tier | Role | Think of it as... |
|------|------|-------------------|
| **Tribune** | Strategic leadership. Decomposes problems, coordinates Centurions. | The architect |
| **Centurion** | Mid-level coordination. Delegates to Legionaries, aggregates results. | The tech lead |
| **Legionary** | Executes a single focused task. Fast, cheap, disposable. | The developer |

Tiers determine defaults (model, storage, timeout) but aren't rigid - a Legionary can use GPT-4 if you want. They're conventions, not constraints.

### Agent Communication

Agents don't talk to each other. The operator is the message broker:

```
Legionary writes /outbox/result.json
  → Operator detects completion
  → Copies result to parent Centurion's /inbox/
  → Centurion wakes up with new context
  → Repeat until Tribune has all results
```

Each agent Pod gets four mount points:

| Path | Purpose |
|------|---------|
| `/inbox/` | Task definition + results from children |
| `/outbox/` | Results + artifacts for parent |
| `/memory/` | Persistent state across agent "turns" |
| `/workspace/` | Scratch space |

### Agent Reincarnation

When an agent's context window fills up, it doesn't crash - it **checkpoints** its state to `/memory/`, gets killed, and respawns with a fresh context window and its checkpoint. The agent picks up where it left off. We call this reincarnation.

### The CLI

Agents interact with Hortator through a CLI, not YAML:

```bash
# Spawn a sub-agent and wait for the result
hortator spawn --prompt "Fix the login bug" --role backend-dev --wait

# Spawn with specific capabilities
hortator spawn --prompt "Scrape the API docs" --cap shell,web-fetch --tier legionary

# Check on your children
hortator tree my-task              # Visualize task hierarchy
hortator status child-task-id      # Check phase
hortator result child-task-id      # Get output

# Budget awareness
hortator budget-remaining          # "You have 42,000 tokens left"

# Self-report (feeds into stuck detection)
hortator progress --status "Analyzing auth module, found 3 issues"
```

## Guardrails

The whole point of Hortator is **autonomous agents with boundaries**. Here's what's built in:

### 🔒 Security
- **Pod isolation** - each agent is its own Pod with its own ServiceAccount
- **NetworkPolicies** - auto-generated from declared capabilities. `web-fetch` opens specific egress. `shell` stays isolated. No capability = no network.
- **Capability inheritance** - children cannot escalate beyond their parent. A Legionary spawned by a Centurion with `[shell]` cannot request `[shell, web-fetch]`.
- **Per-capability RBAC** - agents get minimal ServiceAccount permissions based on their declared capabilities

### 💰 Budget
- **Token and cost caps** - per-task and per-hierarchy (shared budget across an entire task tree)
- **LiteLLM price map** - automatic cost tracking across providers
- **`BudgetExceeded` phase** - tasks stop cleanly when limits are hit, not mid-stream

### 🛡️ PII Redaction
- **Presidio sidecar** - scans agent input and output for PII, secrets, API keys
- **Input redaction** - prompts are scrubbed before reaching the LLM (configurable)
- **Output redaction** - results are scrubbed before being passed to parent agents

### 🏥 Health Monitoring
- **Behavioral stuck detection** - not just "is the process alive" but "is the agent making progress?" Monitors tool diversity, prompt repetition, state staleness.
- **Auto-kill or escalate** - stuck agents can be terminated or flagged for human review

### 📊 Observability
- **Prometheus metrics** - `hortator_tasks_total`, `hortator_tasks_active`, `hortator_task_duration_seconds`
- **OpenTelemetry traces** - task hierarchy maps directly to distributed traces. Open in Jaeger/Tempo and see the full agent tree.

## CRDs

### AgentTask

The core resource. One task = one agent = one Pod.

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
  budget:
    maxTokens: 100000
    maxCostUsd: "0.50"
  model:
    name: claude-sonnet
  env:
    - name: ANTHROPIC_API_KEY
      valueFrom:
        secretKeyRef:
          name: llm-keys
          key: anthropic
```

**Status phases:** `Pending` → `Running` → `Waiting` → `Completed` | `Failed` | `BudgetExceeded` | `TimedOut` | `Cancelled`

### AgentRole / ClusterAgentRole

Behavioral archetypes. Define what an agent *is* - its rules, anti-patterns, and default tools.

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
  antiPatterns:
    - "Never use `any` in TypeScript"
  tools: [shell, web-fetch]
  defaultModel: claude-sonnet
```

### AgentPolicy

Namespace-scoped governance. The guardrails that individual agents can't override.

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: production-policy
  namespace: ai-team
spec:
  maxTier: centurion                     # No tribunes allowed
  maxConcurrentTasks: 10
  allowedCapabilities: [shell, web-fetch]
  deniedCapabilities: [spawn]            # No recursive agent spawning
  requirePresidio: true                  # PII redaction mandatory
  maxBudget:
    maxCostUsd: "5.00"
  egressAllowlist:
    - host: api.anthropic.com
      ports: [443]
```

## Configuration

Everything lives in Helm values - GitOps-friendly, no custom images needed.

**Three-tier override:** Helm defaults → AgentRole → AgentTask (most specific wins).

See [`charts/hortator/values.yaml`](charts/hortator/values.yaml) for the full reference.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Kubernetes Cluster                    │
│                                                         │
│  ┌──────────────┐   watches    ┌────────────────────┐   │
│  │   Operator   │◄────────────►│  AgentTask CRDs    │   │
│  │   (Go)       │              │  AgentRole CRDs    │   │
│  └──────┬───────┘              │  AgentPolicy CRDs  │   │
│         │ creates Pods         └────────────────────┘   │
│         ▼                                               │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐                 │
│  │ Tribune │  │Centurion│  │Legionary│  (each a Pod)   │
│  │ + PVC   │  │ + PVC   │  │ + PVC   │                 │
│  └─────────┘  └─────────┘  └─────────┘                 │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │ Optional: Presidio · OTel Collector · Qdrant      │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**Written in Go.** Operator pattern, controller-runtime, Helm chart. The boring infrastructure choices, on purpose.

**Two personas:**

| Who | Interface | What they do |
|-----|-----------|--------------|
| **Platform engineers** | Helm, YAML | Configure operator, define roles/policies, set budgets |
| **Agents** | `hortator` CLI | Spawn sub-agents, check status, report results |

**SDKs:** [Python](sdk/python/) (sync/async, LangChain + CrewAI integrations) and [TypeScript](sdk/typescript/) (zero deps, LangChain.js integration).

### Bring Your Own Agent

Hortator ships with a **reference runtime** (`runtime/agentic/`) — a Python-based agent that demonstrates multi-tier delegation, planning loops, and tool usage. It's a starting point, not the only option.

**Hortator is runtime-agnostic.** Any agent that can run in a container and call the `hortator` CLI works:

```bash
# Inside your agent's container — that's the entire integration surface
hortator spawn --prompt "Analyse this dataset" --role data-analyst --wait
hortator result child-task-id
hortator progress --status "Phase 2 complete"
hortator budget-remaining
```

The operator doesn't care what language your agent is written in, what LLM it calls, or how it makes decisions. It cares that your agent stays within its budget, capabilities, and network boundary. Write your agent in Python, Go, TypeScript, Rust, or bash — as long as it reads from `/inbox/`, writes to `/outbox/artifacts/`, and uses the CLI to spawn children, Hortator handles the rest.

The reference runtime is there to get you started and to demonstrate what's possible. Replace it whenever you're ready.

## FAQ

**"Agents spawning agents sounds terrifying."**
That's the point. Agents are *already* doing this - spawning Docker containers, SSH-ing into machines, running arbitrary code. Hortator adds the guardrails: capability inheritance, network isolation, budget caps, PII redaction, stuck detection. The question isn't whether agents will spawn agents. It's whether they'll do it with or without guardrails.

**"This is just Kubernetes Jobs with extra steps."**
At the lowest level, yes - like how Kubernetes is just Linux processes with extra steps. The value is the lifecycle management: result brokering between parent and child agents, PVC lifecycle, budget enforcement, stuck detection, PII redaction, security policies, observability. The same argument applies to any operator.

**"Why not just use CrewAI / LangGraph / AutoGen?"**
Those are agent frameworks. Hortator is agent infrastructure. They solve different problems at different layers. You can run CrewAI *inside* a Hortator agent - and now your CrewAI crew has pod isolation, budget enforcement, and network policies. They're complementary.

**"Why Kubernetes?"**
Because Kubernetes already solved pod isolation, resource limits, networking, storage, RBAC, and scheduling. Building agent orchestration on top of K8s means inheriting all of that for free. If your agents don't need isolation or you're running on a laptop, Hortator is probably overkill - and that's fine.

**"Does this make my agents smarter?"**
No. Hortator is infrastructure, not intelligence. It prevents good agents from failing for infrastructure reasons: context window exhaustion, runaway costs, no isolation, no monitoring, no cleanup. It's the difference between running a web app on bare metal vs. in Kubernetes.

## Roadmap

**Done:** CRD-driven task lifecycle, CLI, Helm chart, multi-tier hierarchies, PVC management, Presidio PII redaction, OpenTelemetry tracing, Prometheus metrics, NetworkPolicies, capability inheritance, warm pod pool, result caching, agent reincarnation, Python + TypeScript SDKs, OpenAI-compatible API gateway.

**Current (v0.2):** Hierarchy-wide budgets, shell command filtering, per-capability RBAC, PII input redaction, pluggable vector store (Qdrant).

**Next:** Validation webhooks, multi-tenancy, gateway session continuity, object storage archival for artifacts, web dashboard.

See [`docs/roadmap.md`](docs/roadmap.md) for the full breakdown.

## Contributing

We're just getting started and welcome contributions. See [CONTRIBUTING.md](CONTRIBUTING.md) *(coming soon)*.

## License

[MIT](LICENSE) - core operator, CLI, SDKs, Helm chart.

Enterprise features (AgentPolicy, Presidio integration) available under separate license.

---

<p align="center">
  <em>Named after the officer on Roman galleys who commanded the rowers.<br>Hortator orchestrates agents without doing the thinking.</em>
</p>
