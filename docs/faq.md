# FAQ

## How is Hortator different from Argo Workflows / Tekton?

Argo and Tekton define **static DAGs** — you declare the entire workflow graph upfront, then execute it. Every step, dependency, and branch is known before the pipeline runs.

Hortator tasks are **dynamically spawned by agents at runtime**. An agent decides *during execution* that it needs help, spawns a sub-agent with a natural language prompt, and gets results back. The task tree **emerges from the work** — it's not predetermined.

Think of it this way: Argo is a conveyor belt (fixed path, known steps). Hortator is a team of people (they decide what to do next based on what they find).

**Use Argo/Tekton when:** You know the workflow ahead of time (CI/CD, data pipelines, ETL).

**Use Hortator when:** AI agents need to make runtime decisions about what sub-tasks to spawn.

## Isn't this just Kubernetes Jobs with extra steps?

At the lowest level, yes — Hortator creates Pods. But the value isn't in creating the Pod. It's everything the operator does around it:

- **Result brokering** — automatically copies child results to parent agent's `/inbox/`
- **PVC lifecycle** — TTL cleanup, retention labels, tag-based knowledge discovery, vector storage graduation
- **Budget enforcement** — token/cost tracking per task with automatic kill when exceeded
- **Stuck detection** — behavioral analysis (tool diversity, prompt repetition) catches looping agents
- **Context management** — graceful degradation when context windows fill up, agent reincarnation
- **Security** — capability inheritance, NetworkPolicy mapping, RBAC per agent

It's the same argument for any Kubernetes operator. You *could* hand-manage StatefulSets for PostgreSQL, but you'd use a Postgres operator because the lifecycle management is the hard part.

## Can agents really be trusted to spawn other agents? Isn't that dangerous?

This is exactly why Hortator exists — to make agent spawning **safe**. Without guardrails, agents are already spawning Docker containers, SSH-ing into machines, and running arbitrary code. That's dangerous.

Hortator provides the guardrails:

- **Capability model** — agents can only use explicitly granted capabilities (`shell`, `web-fetch`, `spawn`)
- **Inheritance** — children inherit parent capabilities and **cannot escalate** beyond them
- **Resource limits** — CPU/memory limits enforced per Pod
- **Budget caps** — token/cost limits per task, automatic termination when exceeded
- **NetworkPolicies** — capabilities map to egress rules (no `web-fetch` = no internet access)
- **Namespace isolation** — agents can only spawn tasks in their own namespace
- **PII detection** — optional centralized Presidio service scans output for secrets and sensitive data

The alternative (agents running unsupervised on bare metal) is what's actually terrifying.

## Does Hortator make my agents smarter?

No. Hortator is infrastructure, not an AI framework. It doesn't improve your prompts, your agent logic, or your model selection.

What it does is **stop good agents from failing for infrastructure reasons**: context window exhaustion, runaway costs, no isolation between agents, no way to detect stuck agents, no result handoff between parent and child tasks.

Build your agents with LangGraph, CrewAI, AutoGen, or plain Python. Hortator runs them safely at scale.

## Do I need to write YAML to use Hortator?

**Platform engineers** use YAML/Helm to set up the operator, define roles, and configure policies. This is the same as any K8s operator.

**Agents** use the `hortator` CLI inside their Pod:

```bash
hortator spawn --prompt "Fix the login bug" --role backend-dev --wait
hortator result <task-id>
hortator budget-remaining
```

Agents never touch YAML. The CLI handles CRD creation under the hood.

If you're an AI developer writing agent logic, you interact with Hortator through the CLI, the [Python SDK](https://pypi.org/project/hortator/) (`pip install hortator`), or the [TypeScript SDK](https://www.npmjs.com/package/@hortator/sdk) (`npm install @hortator/sdk`). Both SDKs support the OpenAI-compatible gateway API with streaming, and include integrations for LangChain, LangGraph, and CrewAI.

## What LLM providers does Hortator support?

All of them. Hortator is **model-agnostic**. Agents call whatever LLM endpoint you configure:

- **Cloud APIs** — OpenAI, Anthropic, Google, etc.
- **Local models** — Ollama, vLLM, any OpenAI-compatible endpoint
- **Hybrid** — use LiteLLM proxy to route between local and cloud based on cost/availability

Hortator doesn't make LLM calls itself. Your agent code calls the LLM. Hortator provides the environment variable (`LLM_BASE_URL`) and the budget tracking.

## What's the overhead of running Hortator?

The operator itself is lightweight:

- **CPU:** ~100m request, ~500m limit
- **Memory:** ~128Mi request, ~256Mi limit
- **One Pod** for the operator (+ optional OTel Collector and Presidio service)

Each agent task runs as a standard K8s Pod. Resource usage depends on your agent workload, not Hortator.

## Can I use Hortator with a single agent (no hierarchy)?

Yes. A single `AgentTask` with `tier: legionary` works fine — it's just a managed K8s Job with budget tracking, stuck detection, and structured output. You don't need tribunes or centurions until you're ready for multi-agent orchestration.

## What's the difference between open source and enterprise?

**Open source (MIT):** Everything you need to run autonomous agents with guardrails — CRDs, operator, CLI, runtime, budget tracking, health monitoring, OTel events.

**Enterprise (separate license):** Governance and compliance features — AgentPolicy CRD, LiteLLM proxy integration, retained PVC → vector storage graduation, object storage archival, cross-namespace policies, OIDC/SSO.

> **Note:** Presidio PII detection is available in open-source via `presidio.enabled` Helm value. It is not an enterprise-only feature.

See [Enterprise Overview](enterprise/overview.md) for details.

## Why the Roman theme?

Hortator is named after the officer on Roman galleys who set the rowing rhythm — orchestrating workers without doing the rowing. The hierarchy follows naturally:

- **Tribune** — strategic commander (like Maximus in Gladiator 🎬)
- **Centurion** — unit coordinator
- **Legionary** — the soldier executing the task

Plus, `tier: legionary` in a YAML file is just cool.
