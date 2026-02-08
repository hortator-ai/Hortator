# Choosing the Right Strategy

Hortator gives you options in three key areas. This guide helps you pick the right one for your use case.

---

## 1. Context Compression: How should agents manage their context window?

### Structured Extraction (default)

**Use when:** Agents do focused, trackable work — coding, testing, debugging, data processing.

The agent maintains a structured state file (`/memory/state.json`) that tracks what it's tried, what worked, and what's next. Token-efficient, prevents retry loops, survives agent reincarnation.

```yaml
runtime:
  contextManagement:
    strategy: structured
```

**Best for:** Legionaries. Most tasks. Start here.

### Summarization

**Use when:** Agents have long-running conversations where narrative context matters — coordination, planning, multi-step reasoning that builds on itself.

Older conversation turns are periodically compressed into a summary. The agent keeps "the gist" without burning tokens on verbatim history.

```yaml
runtime:
  contextManagement:
    strategy: summarize   # or hybrid (structured + summarize fallback)
    summarization:
      triggerPercent: 75
      keepRecentTurns: 10
```

**Best for:** Centurions coordinating multiple legionaries. Tasks where the *journey* matters, not just the current step.

**Watch out:** Summarization is lossy. Details get dropped. If an agent needs to remember exact file paths, error messages, or code snippets from 30 turns ago, structured extraction is better.

### Vector Retrieval

**Use when:** Agents need to search large knowledge bases, reference documentation, or recall specific details from a massive history.

Requires a vector database (pgvector, Milvus, Qdrant, Chroma) deployed in your cluster.

```yaml
runtime:
  contextManagement:
    vectorRetrieval:
      enabled: true
      backend: pgvector
      endpoint: postgres://...
```

**Best for:** Consul-tier agents managing long-lived projects. Research agents. Agents that need "institutional memory" spanning weeks/months.

**Don't use for:** Short-lived legionary tasks. The infrastructure overhead isn't worth it for a 5-minute coding task.

### Decision Matrix

| Scenario | Strategy | Why |
|----------|----------|-----|
| Fix a bug (legionary) | Structured | Track attempts, prevent retries, cheap |
| Coordinate 5 legionaries (centurion) | Hybrid | Structured state + summarized coordination history |
| Architect a system (consul) | Summarize + Vector | Long narrative + searchable past decisions |
| One-shot data transform | Structured | Probably won't even need compression |
| Multi-day research project | Vector | Need to recall specific findings across sessions |

---

## 2. Model Routing: Which LLM should agents use?

### Static Endpoint (default)

**Use when:** You have one LLM provider and just want things to work.

Every agent calls the same endpoint. Configure once in Helm values.

```yaml
models:
  default:
    endpoint: https://api.anthropic.com/v1
    name: claude-sonnet
```

**Best for:** Getting started. Small teams. Single-provider shops.

### LiteLLM Proxy Routing

**Use when:** You want to mix local and cloud models, need fallbacks, or want cost-based routing.

Deploy LiteLLM proxy in-cluster. Agents call model aliases (`fast`, `smart`, `reasoning`). LiteLLM routes to the optimal backend.

```yaml
models:
  presets:
    litellm:
      enabled: true
      endpoint: http://litellm.default.svc:4000/v1
```

**Best for:** Cost optimization. Hybrid local+cloud setups. High availability (cloud down → fallback to local).

### Task-Aware Routing (post-MVP)

**Use when:** You want the hierarchy tier to automatically determine model quality.

```yaml
routing:
  strategy: task-aware
  rules:
    - tier: legionary
      model: fast          # Cheap local model
    - tier: centurion
      model: smart         # Mid-tier cloud model
    - tier: consul
      model: reasoning     # Expensive reasoning model
```

**Best for:** Large-scale deployments where you want to optimize cost/quality automatically without configuring every task.

### Decision Matrix

| Scenario | Routing | Why |
|----------|---------|-----|
| Solo developer, one API key | Static | Simple, no infra needed |
| Team with budget constraints | LiteLLM | Cost tracking + cheaper routing |
| Air-gapped / private cluster | Static → Ollama | Local only, no external calls |
| Production with SLAs | LiteLLM | Fallbacks if cloud goes down |
| 100+ concurrent agents | LiteLLM + task-aware | Auto-optimize cost across tiers |

---

## 3. Budget Tracking: How accurate do you need cost data?

### Self-Reported (default, open source)

**Use when:** You want budget guardrails and approximate cost visibility.

Agents report their own token usage via `hortator report-usage`. Operator calculates cost from LiteLLM's community-maintained price map.

```yaml
budget:
  enabled: true
  priceSource: litellm
```

**Accuracy:** Best-effort estimate. ±10-20% due to cached tokens, batch discounts, and provider-side pricing variations.

**Best for:** Most users. Cost awareness and guardrails without extra infrastructure.

### LiteLLM Proxy (enterprise)

**Use when:** You need audit-grade cost tracking for chargebacks, compliance, or finance.

LiteLLM proxy intercepts all LLM calls and tracks exact token usage + cost.

```yaml
budget:
  litellmProxy:
    enabled: true
```

**Accuracy:** Authoritative. Proxy sees every request/response.

**Best for:** Enterprise. Multi-tenant deployments with chargebacks. Regulated industries.

### Decision Matrix

| Scenario | Tracking | Why |
|----------|----------|-----|
| Dev/staging environment | Self-reported | Good enough for guardrails |
| Production, single team | Self-reported | Cost awareness without complexity |
| Multi-tenant, chargebacks | LiteLLM proxy | Need exact per-team accounting |
| Compliance / audit requirements | LiteLLM proxy | Audit-grade accuracy |
| Local models only | Self-reported (tokens only) | No $ cost, but track token consumption |

---

## 4. PII Detection: When to enable Presidio?

### Disabled

**Use when:** Agents only work with public data, or you handle PII at the application layer.

```yaml
presidio:
  enabled: false
```

### Detect Only

**Use when:** You want visibility into what PII agents encounter, without blocking them.

Logs PII findings as OTel events. Doesn't modify agent output.

```yaml
presidio:
  enabled: true
  action: detect
```

**Best for:** Initial rollout. Understanding your PII exposure before enforcing redaction.

### Redact (default when enabled)

**Use when:** Agents must never leak PII in their output.

Replaces detected PII with placeholder tokens before output leaves the Pod.

```yaml
presidio:
  enabled: true
  action: redact
```

**Best for:** Production. Healthcare, finance, any regulated industry.

### Decision Matrix

| Scenario | Presidio | Why |
|----------|----------|-----|
| Internal tooling, no PII | Disabled | No overhead needed |
| Processing customer data | Redact | Compliance, prevent leaks |
| Evaluating PII exposure | Detect | Visibility without disruption |
| Air-gapped, internal docs only | Disabled or Detect | Low risk, but visibility is nice |

---

## 5. Storage Retention: What to keep, what to discard?

### Default TTL (just works)

**Use when:** You don't need to keep agent work products. Most legionary tasks.

PVCs auto-delete after TTL (7d completed, 2d failed, 1d cancelled).

```yaml
storage:
  cleanup:
    ttl:
      completed: 7d
```

### Retain + Tags

**Use when:** Agents produce valuable knowledge that future agents should access.

Agents mark important PVCs during execution. Operator matches tags to future tasks.

```yaml
# Agent calls during execution:
# hortator retain --reason "Auth architecture decisions" --tags "auth,backend"
```

**Best for:** Architecture decisions. Research findings. Anything you'd want to "remember" across tasks.

### Vector Graduation

**Use when:** You have long-lived retained PVCs eating storage, but the knowledge is still valuable.

Stale retained PVCs get indexed into vector storage, then the PVC is freed.

```yaml
storage:
  retained:
    graduation:
      vectorStore:
        enabled: true
```

**Best for:** Long-running projects with months of accumulated knowledge. Requires vector DB.

### Decision Matrix

| Scenario | Strategy | Why |
|----------|----------|-----|
| One-off tasks, no history needed | Default TTL | Auto-cleanup, zero maintenance |
| Building a product over weeks | Retain + Tags | Future agents learn from past work |
| Large-scale, storage-constrained | Retain + Vector graduation | Knowledge persists, storage is freed |
| Debugging failed tasks | Short TTL (2d) | Enough time to investigate, then clean up |

---

## Quick Reference: Recommended Defaults by Scale

### Solo / Small Team (1-10 agents)
```yaml
runtime:
  contextManagement:
    strategy: structured
models:
  default:
    endpoint: https://api.anthropic.com/v1    # One provider
budget:
  enabled: true
  priceSource: litellm
presidio:
  enabled: false
storage:
  cleanup:
    ttl:
      completed: 7d
```

### Medium Team (10-50 agents)
```yaml
runtime:
  contextManagement:
    strategy: hybrid              # Structured + summarization fallback
models:
  presets:
    litellm:
      enabled: true               # Cost-based routing
budget:
  enabled: true
presidio:
  enabled: true
  action: detect                  # Start with visibility
storage:
  retained:
    discovery: tags
```

### Enterprise (50+ agents, multi-tenant)
```yaml
enterprise:
  enabled: true
runtime:
  contextManagement:
    strategy: hybrid
    vectorRetrieval:
      enabled: true
budget:
  litellmProxy:
    enabled: true                 # Authoritative tracking
presidio:
  enabled: true
  action: redact
storage:
  retained:
    graduation:
      vectorStore:
        enabled: true
      objectStore:
        enabled: true
```
