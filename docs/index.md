# ⚔️ Hortator

**Kubernetes-native orchestration for autonomous AI agents**

*Named after the officer on Roman galleys who commanded the rowers — orchestrates agents without doing the thinking.*

---

## What is Hortator?

Hortator is a **Kubernetes operator** that lets AI agents spawn other AI agents — forming autonomous hierarchies to solve complex problems.

Think of it as **Kubernetes for AI workforces**: agents get isolated Pods, resource limits, network policies, budget caps, and health monitoring. They can spawn sub-agents, pass context, and report results — all orchestrated through K8s-native CRDs.

**Hortator doesn't care how your agents think.** It provides the infrastructure — isolation, spawning, governance, security. Build your agents with LangGraph, CrewAI, AutoGen, or plain Python. Hortator runs them.

## Why Hortator?

| Single container | Hortator |
|---|---|
| All agents share one process | Each agent gets its own Pod |
| One agent crashes → everything crashes | Isolated failures |
| No resource limits per agent | CPU/memory limits per agent |
| No security boundaries | NetworkPolicy + RBAC per agent |
| Static agent count | Dynamic spawning (agents create agents) |
| One big context window | Per-agent context + structured handoffs |

## Quick Install

```bash
helm repo add hortator https://charts.hortator.io
helm install hortator hortator/hortator \
  --namespace hortator-system --create-namespace \
  --set models.default.endpoint=https://api.anthropic.com/v1 \
  --set models.default.name=claude-sonnet \
  --set examples.enabled=true
```

→ [Full Quickstart Guide](getting-started/quickstart.md)

## The Roman Hierarchy

| Tier | Role | Storage | Model |
|------|------|---------|-------|
| **Consul** | Strategic leadership | PVC (persistent) | Expensive reasoning |
| **Centurion** | Coordinates a unit | PVC (persistent) | Mid-tier |
| **Legionary** | Executes a single task | EmptyDir (ephemeral) | Fast/cheap |

→ [Learn the Concepts](getting-started/concepts.md)

## Built-in Guardrails

- 🔒 **Security** — Per-agent NetworkPolicies, RBAC, capability inheritance
- 💰 **Budget** — Token/cost caps per task, LiteLLM price map integration
- 🛡️ **PII Detection** — Presidio sidecar for secrets and PII scanning
- 🏥 **Health Monitoring** — Behavioral stuck detection, auto-escalation
- 📊 **Observability** — OpenTelemetry traces + Prometheus metrics
- 💾 **Context Management** — Structured extraction, summarization, agent reincarnation

---

*"I don't row. I command the rowers."* — The Hortator
