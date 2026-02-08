# Hortator Backlog

Personas: **Platform Engineer** (sets up Hortator), **AI Developer** (builds agent apps), **Agent** (the AI spawning workers), **Cluster Admin** (manages policies/tenants)

---

## MVP Scope

1. AgentTask CRD definition
2. Basic Go operator (watch CRD → create Job → track status → cleanup)
3. CLI: `hortator spawn`, `status`, `logs`, `result`, `cancel`, `list`
4. Default runtime container (OpenClaw + hortator CLI + common tools)
5. Helm chart for easy install
6. README with vision, quickstart, examples

---

## 📐 EPIC: Design & Documentation

- [x] **P0** Finalize CRD schemas — Lock down AgentTask, AgentRole, ClusterAgentRole YAML specs incorporating all research decisions ✅ 2026-02-08
- [x] **P0** Consolidate Helm `values.yaml` skeleton — Single coherent values file from all 9 research items ✅ 2026-02-08
- [x] **P0** Bootstrapping decision — Just `helm install`? CLI with GitOps integration? Hybrid? ✅ 2026-02-08
- [x] **P0** README rewrite — Proper open-source README (elevator pitch, quickstart, architecture diagram, contributing guide). ✅ 2026-02-08
- [x] **P1** Architecture diagram — Mermaid diagrams: system overview, task lifecycle, override precedence, storage lifecycle ✅ 2026-02-08
- [x] **P1** License decision — Formalize MIT core vs proprietary enterprise boundary ✅ 2026-02-08
- [x] **P2** "When to use which" docs — Context compression, model routing, budget, Presidio, storage retention ✅ 2026-02-08

### ✅ Decision: License & Enforcement (2026-02-08)

**Decision: Build tags + license notice (LiteLLM model).**

- Single repo, enterprise code in `enterprise/` directory with separate LICENSE
- Go build tags: `//go:build enterprise` — OSS binary genuinely excludes enterprise code
- Two image tags: `operator:latest` (MIT) / `operator:enterprise` (commercial)
- Helm toggle: `enterprise.enabled: false` → pulls OSS image by default
- No license key at MVP. Add runtime enforcement later if revenue justifies it.

**MIT (open source):**
- AgentTask + AgentRole/ClusterAgentRole CRDs
- Operator core (watch, create Jobs, track status, cleanup)
- CLI (spawn, status, result, logs, cancel, list, tree)
- Default runtime container + filesystem conventions
- Helm chart with all sane defaults
- Storage (PVC provisioning, TTL cleanup, retention labels)
- Basic security (RBAC, capability inheritance)
- Budget tracking (self-reported + LiteLLM price map)
- Health metrics + stuck detection
- OTel event emission

**Enterprise (separate license, `enterprise/` directory):**
- AgentPolicy CRD (fine-grained capability restrictions)
- Egress allowlists
- Presidio sidecar integration
- LiteLLM proxy sub-chart (authoritative cost tracking)
- Retained PVC → vector storage graduation
- Retained PVC → object storage archival
- Cross-namespace task spawning with policy controls
- OIDC/SSO
- Multi-cluster federation

**Principle:** Everything needed to run autonomous agents with basic guardrails = free. Enterprise = governance, compliance, advanced cost control.

---

### ✅ Decision: Bootstrapping Strategy (2026-02-08)

**Decision: Helm install + optional examples flag. No custom CLI for bootstrapping.**

Two install paths, same manifests:
```bash
# Path 1: Helm flag (convenience — demo/eval)
helm install hortator hortator/hortator --set examples.enabled=true

# Path 2: kubectl apply (explicit — production)
helm install hortator hortator/hortator
kubectl apply -f examples/quickstart/
```

- `examples.enabled=false` by default (production clean)
- Examples install to separate namespace (`hortator-demo`) for easy cleanup
- Example manifests live in repo AND Helm templates (one source of truth)
- CLI (`hortator spawn/status/logs`) ships inside runtime container only — it's for agents, not humans
- Day-2: standard `helm upgrade`. No custom tooling.

**Why not a CLI bootstrap?** Maintenance trap. Goes stale when Helm chart evolves. Duplicates what Helm already does. Our users live in kubectl/helm — meet them there.

**Repo structure:**
```
examples/
  quickstart/            # kubectl apply -f this whole dir
    roles.yaml           # Sample ClusterAgentRoles
    hello-world.yaml     # Simple AgentTask
  advanced/
    multi-tier.yaml      # Tribune → Centurion → Legionary
    with-presidio.yaml   # PII detection
    with-budget.yaml     # Cost-capped task
```

---

## 🎯 EPIC: Operator Core

- [ ] **P0** As a platform engineer, I want the operator to watch AgentTask CRDs and create Pods so agents can spawn workers
- [ ] **P0** As a platform engineer, I want to install Hortator via Helm chart so I can add agent orchestration to my cluster
- [ ] **P0** As a platform engineer, I want the operator to enforce resource limits defined in AgentTask specs
- [ ] **P1** As a platform engineer, I want completed/failed tasks to be cleaned up after a configurable retention period
- [ ] **P1** As a platform engineer, I want operator metrics (tasks created/completed/failed, active workers) exposed for Prometheus

## 📋 EPIC: AgentTask CRD

- [ ] **P0** As an AI developer, I want to define an AgentTask with prompt, capabilities, and resource limits
- [ ] **P0** As an AI developer, I want task status to reflect Pending/Running/Completed/Failed states
- [ ] **P0** As an AI developer, I want to set a timeout so runaway tasks get terminated automatically
- [ ] **P0** As an AI developer, I want to specify environment variables (API keys etc.) via secretRef
- [ ] **P1** As an AI developer, I want to specify a parent task ID to establish hierarchy (tribune → centurion → legionary)
- [ ] **P1** As an AI developer, I want to specify which container image to use (or default to Hortator runtime)
- [ ] **P1** As an AI developer, I want task results stored in status.output so I can retrieve them

## ⌨️ EPIC: CLI (hortator)

- [ ] **P0** As an agent, I want to run `hortator spawn --prompt "..."` to create a worker task and get its ID
- [ ] **P0** As an agent, I want to run `hortator spawn --wait` to block until the worker completes and return the result
- [ ] **P0** As an agent, I want to run `hortator status <task-id>` to check if a task is pending/running/completed/failed
- [ ] **P0** As an agent, I want to run `hortator result <task-id>` to get the final output of a completed task
- [ ] **P1** As an agent, I want to run `hortator spawn --cap shell,web-fetch` to grant specific capabilities to the worker
- [ ] **P1** As an agent, I want to run `hortator logs <task-id>` to stream or fetch worker output logs
- [ ] **P1** As an agent, I want to run `hortator cancel <task-id>` to terminate a running task
- [ ] **P1** As an AI developer, I want to run `hortator list` to see all tasks in my namespace with their states
- [ ] **P2** As an AI developer, I want to run `hortator tree <task-id>` to visualize the task hierarchy (parent/children)
- [ ] **P2** As an AI developer, I want `--output json` flag on all commands for scripting/automation

## 📦 EPIC: Default Runtime

- [ ] **P0** As a platform engineer, I want a default container image that runs agent tasks out of the box
- [ ] **P0** As an AI developer, I want the runtime to read task prompt from /inbox/task.json on startup
- [ ] **P0** As an AI developer, I want the runtime to write results to /outbox/result.json on completion
- [ ] **P0** As an AI developer, I want the runtime to support configurable LLM backends via env vars (OPENAI_API_KEY, ANTHROPIC_API_KEY)
- [ ] **P1** As an agent, I want the hortator CLI pre-installed in the runtime so I can spawn sub-workers
- [ ] **P1** As an AI developer, I want the runtime to have common tools pre-installed (git, curl, python, node)
- [ ] **P2** As an AI developer, I want to specify a custom runtime image in the AgentTask spec

## 💾 EPIC: Storage

- [ ] **P0** As a platform engineer, I want PVCs automatically provisioned for persistent agents (tribune, centurion tiers)
- [ ] **P0** As an agent, I want to read my task from /inbox/task.json when I start
- [ ] **P0** As an agent, I want to write my results to /outbox/result.json when I complete
- [ ] **P0** As an agent, I want /workspace for temporary files during task execution
- [ ] **P1** As an agent, I want /memory to persist my long-term context across task restarts
- [ ] **P1** As a platform engineer, I want the operator to copy worker results to parent agent inbox automatically
- [ ] **P2** As an AI developer, I want to configure storage class and size for PVCs via CRD spec
- [ ] **P2** (Future) As an AI developer, I want pluggable memory backends (Milvus, Postgres, Redis)

## 🔒 EPIC: Security & Guardrails

- [ ] **P1** As a platform engineer, I want capabilities (shell, web-fetch, spawn) to map to NetworkPolicies automatically
- [ ] **P1** As a platform engineer, I want to set cluster-wide default resource limits for all tasks
- [ ] **P1** As a platform engineer, I want workers to run with minimal RBAC (only create tasks in own namespace)
- [ ] **P1** As a platform engineer, I want tasks to inherit parent capabilities (workers can't escalate beyond parent)
- [ ] **P2** As a platform engineer, I want to restrict which namespaces can spawn tasks via label selectors
- [ ] **P2** As a cluster admin, I want audit logs of all task spawns, completions, and failures
- [ ] **P2** (Enterprise) As a cluster admin, I want AgentPolicy CRD to define fine-grained capability restrictions per namespace
- [ ] **P2** (Enterprise) As a cluster admin, I want egress allowlists to control which external APIs agents can call

## 🏢 EPIC: Multi-tenancy

- [ ] **P2** Define namespace isolation model (ResourceQuotas, NetworkPolicies, RBAC per tenant)
- [ ] **P2** Support one cluster = many AI companies, or dedicated clusters
- [ ] **P2** (Enterprise) Cross-namespace task spawning with policy controls

---

## Research Tasks

- [x] Presidio performance — latency/throughput at scale, resource requirements, custom pattern training ✅ 2026-02-08
- [x] Audit event schema — OpenTelemetry vs CEF vs custom JSON? What do SIEMs (Splunk, Datadog) expect? ✅ 2026-02-08
- [x] Budget controls implementation — where does token counting happen? Operator-level? Sidecar? Proxy? ✅ 2026-02-08
- [x] Agent health metrics — what matters? (token burn rate, task progress, error rate, idle time, stuck detection) ✅ 2026-02-08
- [x] Competitive analysis — how do LangGraph, CrewAI, AutoGen handle K8s deployment today? ✅ 2026-02-08 (full write-up in Notion)
- [x] Local model routing — ollama/vLLM integration patterns, model selection logic ✅ 2026-02-08
- [x] Context compression — summarization vs vector retrieval vs structured extraction. What ships in runtime? ✅ 2026-02-08
- [x] PVC cleanup strategies — explicit deletion, TTL-based GC, size quotas. What does the operator manage? ✅ 2026-02-08
- [x] Memory folder structure — context.md, decisions.log, workers/ archive. Standardize or leave to agent? ✅ 2026-02-08

---

## ✅ Research Findings

### Presidio Performance & Configuration (2026-02-08)

**Latency (per ~1,000 tokens):**
- Regex-only recognizers: sub-millisecond
- `en_core_web_sm` (small spaCy): <10ms
- `en_core_web_lg` (large spaCy): ~20-50ms
- Transformer models (`en_core_web_trf` / HuggingFace / Flair): 100ms+ (GPU recommended)

**Resource requirements by model:**
| Model | Size | RAM | Notes |
|-------|------|-----|-------|
| `en_core_web_sm` | ~15MB | ~200-300MB | CPU only, fast |
| `en_core_web_lg` | ~560MB | ~800MB-1GB | CPU only, better NER |
| `en_core_web_trf` | ~500MB+ | 1-2GB+ | Benefits from GPU |

**Custom recognizers (three tiers):**
1. **PatternRecognizer** — regex + deny-lists, YAML or Python, zero ML
2. **Custom EntityRecognizer** — subclass with arbitrary logic
3. **Custom NER models** — train spaCy/HuggingFace, plug in

**No official benchmarks:** Microsoft intentionally doesn't publish them — "Presidio is a framework, not a solution." Every deployment differs based on recognizers/models chosen. See `presidio-research` repo for evaluation notebooks.

#### Architecture Decision: Helm-Driven Configuration

**Sane defaults live in Helm `values.yaml`** — not baked into the image. This keeps everything K8s-native, transparent (`helm show values`), and overridable without custom image builds.

```yaml
# values.yaml (Hortator Helm chart)
presidio:
  enabled: true
  image: mcr.microsoft.com/presidio-analyzer:latest
  model: en_core_web_sm
  scoreThreshold: 0.5
  action: redact  # redact | detect | hash | mask
  recognizers:
    disabled:
      # PhoneRecognizer iterates patterns for every country code on every
      # request, making it by far the slowest built-in recognizer.
      # If you need phone detection, enable it and restrict to specific
      # countries via custom config to limit the performance impact.
      - PhoneRecognizer
    custom:
      - name: AWSKeyRecognizer
        entity: AWS_ACCESS_KEY
        patterns:
          - regex: "AKIA[0-9A-Z]{16}"
            score: 0.95
      - name: BearerTokenRecognizer
        entity: BEARER_TOKEN
        patterns:
          - regex: "Bearer [A-Za-z0-9\\-._~+/]+=*"
            score: 0.9
      - name: PrivateKeyRecognizer
        entity: PRIVATE_KEY
        patterns:
          - regex: "-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----"
            score: 0.99
```

Helm renders defaults into a ConfigMap, Presidio sidecar mounts it.

**Override model (most specific wins):**
1. **Helm values** — cluster-wide defaults
2. **Namespace ConfigMap** (`hortator-presidio-config`) — team overrides
3. **AgentTask spec inline** — per-task overrides (`presidio.scoreThreshold`, `presidio.action`, etc.)

**Why Helm values, not baked into image:**
- `helm show values` = instant visibility into every default and why
- Override via `--values` or `--set` — standard Helm workflow
- GitOps-friendly (values files in git, ArgoCD/Flux apply)
- Upstream Presidio image used as-is — zero maintenance burden

**Recommendation for MVP:**
- Default: `en_core_web_sm` + regex recognizers for secrets/keys
- Sidecar pattern alongside agent pods
- Skip PhoneRecognizer (document why in values.yaml)
- <10ms overhead per scan — negligible for agent workloads

### Memory Folder Structure (2026-02-08)

**Decision: Standardize the contract (required files), recommend patterns (conventions), free-form the rest.**

#### Required Files (operator reads/writes)

| Path | Owner | Direction | Purpose |
|------|-------|-----------|---------|
| `/inbox/task.json` | Operator → Agent | Read | Task definition, prompt, role, budget, capabilities |
| `/inbox/context.json` | Operator → Agent | Read | Prior work references, retained PVC mount paths |
| `/memory/state.json` | Agent → Operator | Write | Structured state for checkpoint/reincarnation |
| `/outbox/result.json` | Agent → Operator | Write | Task output, summary, artifact list |
| `/outbox/usage.json` | Agent → Operator | Write | Token usage for budget tracking |

#### Convention Files (recommended, not enforced)

**`/memory/decisions.log`** — Append-only decision audit trail:
```
[2026-02-08T13:30:00Z] DECISION: Use async cookie set instead of mutex
  REASON: Mutex causes deadlock under concurrent requests
  ALTERNATIVES_CONSIDERED: [mutex, queue, async]
  CONFIDENCE: high
```
Why: Self-documentation for future agents reading retained PVCs. Stuck detection (same decision repeated = loop). Enterprise audit compliance.

**`/memory/errors.log`** — Failed attempts tracker:
```
[2026-02-08T13:32:00Z] ATTEMPT: Added mutex to session handler
  FILE: src/session.ts:47
  RESULT: FAILED
  ERROR: Deadlock detected in test_concurrent_auth
  TOKENS_SPENT: 1200
```
Why: Prevents #1 agent failure mode (retrying same thing). Runtime can inject into LLM context.

**`/outbox/artifacts/`** — Produced files (patches, reports, screenshots, etc.)

**`/workspace/`** — Free-form scratch space. Agent's to use however they want.

#### Schema: `/inbox/task.json`
```json
{
  "taskId": "fix-auth-bug-42",
  "prompt": "Fix the session cookie not being set on login response",
  "role": "backend-dev",
  "flavor": "Use existing Drizzle ORM. Don't touch migrations.",
  "parentTaskId": "feature-auth-refactor",
  "tier": "legionary",
  "budget": { "maxTokens": 100000, "maxCostUsd": 0.50 },
  "capabilities": ["shell", "web-fetch"],
  "prior_work": []
}
```

#### Schema: `/outbox/result.json`
```json
{
  "taskId": "fix-auth-bug-42",
  "status": "completed",
  "summary": "Fixed session cookie by switching to async cookie set in session.ts:47",
  "artifacts": ["artifacts/session.ts.patch"],
  "decisions": 3,
  "tokensUsed": { "input": 45000, "output": 12000 },
  "duration": "4m32s"
}
```

#### Helm Config
```yaml
runtime:
  filesystem:
    enforceRequired: true        # fail task if required files missing at completion
    conventions:
      decisionsLog: true         # runtime auto-maintains decisions.log
      errorsLog: true            # runtime auto-maintains errors.log
```

---

### PVC Cleanup & Retained Knowledge (2026-02-08)

**Decision: TTL default + retention labels + quota guardrail + knowledge discovery.**

#### Cleanup: Three Layers

**Layer 1 — TTL (automatic):**
```yaml
storage:
  cleanup:
    ttl:
      completed: 7d
      failed: 2d
      cancelled: 1d
```
Operator annotates PVC with `hortator.io/expires-at` on task completion. Reconciliation loop deletes expired PVCs.

**Layer 2 — Retention labels (opt-out from TTL):**
Agents self-mark important PVCs:
```
hortator retain --reason "Auth architecture decisions" --tags "auth,architecture,backend"
```
Adds `hortator.io/retain=true`, `hortator.io/retain-reason`, `hortator.io/retain-tags` annotations. Operator skips during GC.

**Layer 3 — Namespace quota (guardrail):**
```yaml
storage:
  quota:
    enabled: true
    maxPerNamespace: 50Gi
    warningPercent: 80
    evictionPolicy: oldest-completed
```

#### Retained PVCs as Institutional Memory

Retained PVCs are useless if nobody knows they exist. The operator acts as librarian.

**Discovery flow:**
1. Manager creates new task: "Redesign auth system"
2. Operator tag-matches retained PVCs in namespace → finds `fix-auth-bug-42` (tags: auth, session)
3. Mounts matching PVC read-only at `/prior/fix-auth-bug-42/`
4. Injects references in `/inbox/context.json`:
```json
{
  "prior_work": [
    {
      "taskId": "fix-auth-bug-42",
      "role": "backend-dev",
      "completedAt": "2026-02-05T14:00:00Z",
      "retainReason": "Auth architecture decisions",
      "tags": ["auth", "architecture", "backend"],
      "mountPath": "/prior/fix-auth-bug-42"
    }
  ]
}
```

**Three levels of discovery:**
1. **Passive (MVP):** Operator lists retained PVCs in context.json. Agent decides whether to look.
2. **Tag-based matching:** Operator auto-matches task prompt/role tags → mounts relevant PVCs.
3. **Manager CLI (post-MVP):** `hortator knowledge list|search|mount` for active knowledge retrieval.

#### Retained PVC Lifecycle

```
1. Worker retains PVC with reason + tags
2. New tasks auto-receive matching retained PVCs (read-only mount)
3. PVC becomes stale (no tag matches in 90 days):
   → If vector storage available:
       → Index PVC contents into vector DB (embeddings)
       → Delete PVC (free storage)
       → Knowledge remains discoverable via `hortator knowledge search`
   → If object storage available (S3/MinIO):
       → Archive PVC contents to cold storage
       → Delete PVC
   → If neither:
       → Emit hortator.storage.retained_stale warning
       → Leave PVC until quota pressure forces eviction
```

**Helm config:**
```yaml
storage:
  retained:
    discovery: tags              # none | tags | semantic (post-MVP)
    autoMount: true
    mountMode: readOnly
    staleAfterDays: 90
    maxRetainedPerNamespace: 20
    graduation:
      vectorStore:
        enabled: false           # opt-in, requires vector DB
      objectStore:
        enabled: false           # opt-in (S3/MinIO)
        bucket: hortator-archives
```

**K8s-native patterns:** Owner references (CRD→PVC), finalizers (archive before delete), annotations (TTL, retention, tags).

---

### Context Compression (2026-02-08)

**Decision: Structured extraction default, summarization fallback, vector retrieval opt-in.**

| Approach | Default? | Best for | Infrastructure |
|----------|----------|----------|----------------|
| **Structured extraction** | ✅ Yes | Legionaries (focused tasks, state tracking) | None (files) |
| **Summarization** | ✅ Fallback | Centurions (narrative continuity) | None (LLM call) |
| **Vector retrieval** | ❌ Opt-in | Tribune / large knowledge bases | Vector DB required |

**Maps to hierarchy:**
- Legionaries → structured state in `/memory/state.json`
- Centurions → summarization of own history + structured results from legionaries
- Tribune → summarization + optional vector retrieval

**Graceful degradation (unique to Hortator):**
- 75% context → trigger summarization
- 90% context → checkpoint state, report partial results
- Operator spawns fresh agent with saved state in `/inbox/` ("agent reincarnation")

```yaml
runtime:
  contextManagement:
    strategy: structured
    structured:
      stateFile: /memory/state.json
      autoExtract: true
    summarization:
      enabled: true
      triggerPercent: 75
      keepRecentTurns: 10
    vectorRetrieval:
      enabled: false
```

**TODO:** Add "when to use which" guidance in user-facing docs.

---

### Local Model Routing (2026-02-08)

**Decision: Model-serving agnostic. Hortator routes, doesn't serve.**

Ollama (dev/small teams, ~1-3 req/sec) and vLLM (production, ~120-160 req/sec) both expose OpenAI-compatible APIs. From the agent's perspective, local and cloud models are identical — just a different `base_url`.

**Three deployment patterns (user chooses):**
1. **Cloud only** — agents call cloud APIs directly
2. **Local only** — agents call Ollama/vLLM in-cluster (air-gapped)
3. **Hybrid** — LiteLLM proxy routes between local+cloud (cost-based, fallback)

**Three levels of routing sophistication:**
1. **Static (MVP):** AgentTask/AgentRole specifies endpoint+model. Runtime gets `LLM_BASE_URL` env var.
2. **LiteLLM routing (optional):** Model aliases (`fast`/`smart`/`reasoning`), cost-based routing, automatic fallback (cloud down → local).
3. **Task-aware (post-MVP):** Auto-map hierarchy tier to model tier (legionary→cheap, centurion→mid, tribune→expensive).

**Helm values:**
```yaml
models:
  default:
    endpoint: ""
    name: ""
    apiKeyRef: {}
  presets:
    ollama:
      enabled: false
      endpoint: http://ollama.default.svc:11434/v1
    vllm:
      enabled: false
      endpoint: http://vllm.default.svc:8000/v1
    litellm:
      enabled: false
      endpoint: http://litellm.default.svc:4000/v1
```

**Why not build model serving into Hortator?** Ollama Operator and vLLM Helm charts already exist. Model serving is a massive domain (GPU scheduling, caching, quantization). Composability > monolith.

**LiteLLM flywheel:** Budget tracking + model routing + fallbacks. Natural companion — document as recommended (optional) LLM gateway.

---

### Agent Health Metrics (2026-02-08)

**Three categories of metrics, all emitted via OTel (same pipeline as audit events).**

#### 1. Operational Metrics (is the agent alive?)

| Metric | What | OTel type |
|--------|------|-----------|
| `hortator.task.duration` | Wall-clock spawn to completion | Histogram |
| `hortator.task.status` | Current state (pending/running/completed/failed) | Gauge per state |
| `hortator.task.queue_wait` | CRD creation → Pod start | Histogram |
| `hortator.worker.active` | Active workers per namespace | Gauge |
| `hortator.task.restarts` | Container restart count | Counter |

#### 2. Cognitive Metrics (is the agent making progress?)

| Metric | What | OTel type |
|--------|------|-----------|
| `hortator.task.tokens.total` | Cumulative tokens consumed | Counter |
| `hortator.task.tokens.rate` | Tokens per minute | Gauge |
| `hortator.task.llm_calls` | Number of LLM API calls | Counter |
| `hortator.task.tool_calls` | Tool invocations (shell, web-fetch, etc.) | Counter |
| `hortator.task.idle_time` | Time since last LLM/tool call | Gauge |

#### 3. Outcome Metrics (did the agent succeed?)

| Metric | What | OTel type |
|--------|------|-----------|
| `hortator.task.success_rate` | Completed / (Completed + Failed) per role | Gauge |
| `hortator.task.cost_per_completion` | Avg cost of successful task | Histogram |
| `hortator.task.escalation_rate` | Tasks that exceeded budget/context | Gauge |
| `hortator.task.children_spawned` | Legionaries spawned per centurion task | Counter |

#### Stuck Detection

**Key insight: detect loops and stalls, not bad output.** Measuring "meaningful output" is impossible — a one-liner fix to a complex bug is more valuable than 500 lines of boilerplate. Lines of code, file size, commit count are all terrible proxies for progress.

Instead, measure **behavioral signals**:

| Signal | What it means | How to measure |
|--------|---------------|----------------|
| **Tool diversity** | Progressing agents do varied things; stuck agents repeat | `unique_patterns / total_calls` over window |
| **Prompt repetition** | Same/similar prompts = going in circles | Count identical/near-identical LLM inputs |
| **State changes** | New files touched, different test results = progress | Track file set + test output hashes |
| **Self-reported status** | Agent heartbeat goes stale = stuck or crashed | `hortator progress --status "..."` freshness |

**What NOT to measure:**
- Output quality (that's evaluation, not monitoring)
- Lines of code / output size (one-liner problem)
- "Correctness" (requires understanding the task)

**Stuck score:** Weighted combination of signals (0-1). Action triggered when threshold exceeded.

```
stuck_indicators:
  tool_diversity < threshold over window       # repetitive actions
  identical_llm_prompts > threshold            # asking the same thing
  no_new_files_touched in window               # not exploring
  test_results_unchanged over N runs           # same error loop
  self_reported_status_stale > threshold       # agent stopped updating
```

**Actions:** `warn` → emit OTel event | `kill` → terminate task | `escalate` → kill + notify parent agent

#### Configuration: Three-Tier Override

Same pattern as Presidio config — Helm defaults, role overrides, task overrides.

```yaml
# values.yaml
health:
  enabled: true
  checkIntervalSeconds: 30
  stuckDetection:
    enabled: true
    defaults:
      toolDiversityMin: 0.3
      maxRepeatedPrompts: 3
      statusStaleMinutes: 5
      checkWindowMinutes: 5
      action: warn                # warn | kill | escalate
    # Per-role overrides — different roles have different "normal"
    roleOverrides:
      qa-engineer:
        toolDiversityMin: 0.15    # test loops are expected behavior
        maxRepeatedPrompts: 6     # retrying tests is normal
      researcher:
        statusStaleMinutes: 10    # reading takes longer, fewer tool calls
  metrics:
    expose: true                  # Prometheus endpoint on operator
    prefix: hortator
```

Per-task override in AgentTask spec:
```yaml
spec:
  health:
    stuckDetection:
      maxRepeatedPrompts: 10      # this task iterates a lot
      action: escalate            # don't kill, send to parent
```

**Override precedence:** Helm defaults → AgentRole → AgentTask (most specific wins)

**Future (post-MVP):** Adaptive thresholds — operator learns "normal" per role from historical p50/p95 data. Ship configurable first, learn from real usage, then automate.

**Dashboard:** All metrics feed into Grafana via OTel + Prometheus. Single dashboard shows task tree (traces), token burn (metrics), stuck alerts (events), budget status — one telemetry pipeline.

---

### Budget Controls (2026-02-08)

**Decision: Runtime reporting + LiteLLM price map (LiteLLM proxy optional)**

Budget tracking works with or without the LiteLLM proxy — just at different confidence levels.

#### Path A: Without LiteLLM proxy (default, open source)

```
Agent makes LLM call → gets response with usage
  → Runtime calls: hortator report-usage --model claude-sonnet --input 500 --output 200
    → Operator looks up price from cached LiteLLM price map
    → Calculates cost estimate
    → Emits OTel event with tokens + cost
    → Checks against task/namespace budget
```

- **Token source:** Runtime self-reports via `hortator report-usage` CLI after each LLM call
- **Price source:** LiteLLM's `model_prices_and_context_window.json` (MIT licensed), fetched daily by operator, cached in ConfigMap
- **Accuracy:** Best-effort estimate. Non-deterministic (cached tokens, batch discounts, rate tier pricing not accounted for). Good enough for guardrails and alerts, not for invoicing.
- **No providers expose a pricing API** — all pricing is website-only. The LiteLLM community-maintained price map is the de facto open source solution.

#### Path B: With LiteLLM proxy (optional, enterprise)

```
Agent makes LLM call → LiteLLM proxy → provider
  → Proxy tracks actual tokens + cost authoritatively
  → Operator reads LiteLLM metrics/DB
  → Deterministic budget enforcement
```

- **Token source:** LiteLLM proxy intercepts all LLM traffic at application layer (no TLS MITM — agents just point `LLM_BASE_URL` at the proxy)
- **Price source:** LiteLLM's built-in cost tracking (same price map, but applied at the proxy)
- **Accuracy:** Authoritative. Proxy sees every request/response.
- **Extras:** Rate limiting, key management, model routing, spend per team/key — all built into LiteLLM's MIT-licensed core
- **Deployment:** Optional Helm sub-chart deploys LiteLLM proxy. Agents configured via env var.

**LiteLLM licensing:** Core proxy + price map = MIT. Their `enterprise/` directory (SSO 5+ users, advanced guardrails) has a separate commercial license. We only use/ship MIT-licensed components.

#### Budget spec

```yaml
# On AgentTask
spec:
  budget:
    maxTokens: 100000        # total tokens (in + out)
    maxCostUsd: 0.50          # dollar cap
    # OR inherit from namespace annotation:
    # hortator.io/monthly-budget-usd: "100"

# Helm values
budget:
  enabled: true
  priceSource: litellm          # litellm | custom
  refreshIntervalHours: 24      # fetch updated prices from GitHub
  customPrices: {}               # override/add models not in LiteLLM map
  fallbackBehavior: track-tokens # track-tokens | block | warn
  litellmProxy:
    enabled: false               # opt-in for authoritative tracking
    # chart: litellm/litellm-proxy  # sub-chart reference
```

#### Enforcement flow
1. Runtime checks budget before LLM call: `hortator budget-remaining`
2. If over limit → runtime refuses call, exits gracefully with partial results
3. Operator sets task status to `budget-exceeded`
4. OTel event emitted: `hortator.budget.exceeded`

#### Why not a network proxy for token counting?
TLS interception in K8s is complex (MITM certs), streaming responses are fragile to parse, adds latency. LiteLLM proxy solves this at the application layer — agents call it as their LLM endpoint, no MITM needed.

---

### Audit Event Schema (2026-02-08)

**Decision: OpenTelemetry (OTLP)**

**Candidates evaluated:**
| Format | Verdict | Reason |
|--------|---------|--------|
| CEF | ❌ | Legacy, flat structure, designed for network security not cloud-native. No nesting, no trace correlation. |
| Custom JSON | ❌ | Total flexibility but zero ecosystem. Every SIEM needs custom parsers. We maintain the spec forever. |
| OpenTelemetry | ✅ | CNCF-native, universal ingestion (Datadog/Splunk/Elastic/Grafana), trace correlation, CEF export via Collector for free. |

**Key insight: Task hierarchy = distributed trace.** Tribune → centurion → legionaries maps naturally to OTel spans. Full task tree visible in Jaeger/Tempo. Way more powerful than flat audit logs.

**Event naming convention:** `hortator.<domain>.<action>`
- `hortator.task.spawned` / `.completed` / `.failed` / `.cancelled`
- `hortator.presidio.pii_detected`
- `hortator.policy.violation`

**Example event:**
```json
{
  "name": "hortator.task.spawned",
  "timestamp": "2026-02-08T11:30:00Z",
  "traceId": "abc123...",
  "spanId": "def456...",
  "parentSpanId": "ghi789...",
  "attributes": {
    "hortator.task.id": "fix-auth-bug-42",
    "hortator.task.role": "backend-dev",
    "hortator.task.tier": "legionary",
    "hortator.task.parent_id": "feature-auth-refactor",
    "hortator.task.capabilities": ["shell", "web-fetch"],
    "hortator.task.model": "sonnet",
    "hortator.task.namespace": "ai-team",
    "k8s.pod.name": "hortator-worker-abc123",
    "k8s.namespace.name": "ai-team"
  }
}
```

**Helm integration:**
```yaml
# values.yaml
telemetry:
  enabled: true
  collector:
    deploy: true  # deploy OTel Collector, or false to use existing
  exporters:
    otlp:
      endpoint: ""  # user fills in their backend
```

- OTel Collector as optional sub-chart dependency
- CEF compatibility: enterprise customers add syslog exporter to Collector config
- Stdout/JSON fallback when telemetry.enabled=false

---

## ✅ Answered Questions (2026-02-07)

- **Guardrails approach:** Presidio (self-hosted PII detection) + egress proxy + tool allowlists. No AI-in-the-loop.
- **State model:** K8s Jobs (ephemeral), not StatefulSets. Import/export folders, dispose after task.
- **Tenant isolation:** K8s-native (namespaces + NetworkPolicies + RBAC). Not an enterprise feature, just document it.
- **DLP without cloud dependency:** Microsoft Presidio (MIT, runs in-cluster, NLP + regex).
- **Storage model:** Legionaries: EmptyDir (auto-deleted with Pod). Centurions/Tribune: PVC (persists across turns, explicit/TTL cleanup).
- **Manager↔Worker comms:** Operator as broker. Worker completes → Operator writes to parent /inbox/ → Operator spawns parent turn. No message queue needed.
- **Manager turns:** Each turn is a Job mounting persistent PVC. Pod is ephemeral, PVC is identity. /memory/, /inbox/, /workspace/, /outbox/ structure.

## Learnings (from current LXC swarm)

- **One task per agent:** N tasks → N agents, not 1 overloaded agent
- **Token budget estimation:** Estimate cost before dispatching, match model to task complexity
- **Task decomposition:** Split broad tasks into surgical, single-file edits per agent
- **Pre-digest context:** Provide relevant snippets, not "go find it"
- **Progress checkpointing:** Agents commit/save partial work so context overflow doesn't lose everything
- **Model-task matching:** Use cheap/fast models (Sonnet) for grep/edit, expensive (Opus) for architecture
- **Graceful degradation:** Detect low remaining context, save state and escalate instead of silently failing
