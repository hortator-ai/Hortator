# Hortator Architecture

## System Overview

```mermaid
graph TB
    subgraph "User / GitOps"
        User["👤 Platform Engineer"]
        GitOps["🔄 ArgoCD / Flux"]
    end

    subgraph "Control Plane"
        Helm["⎈ Helm Chart"]
        CRDs["📋 CRDs<br/>AgentTask<br/>AgentRole<br/>ClusterAgentRole<br/>AgentPolicy"]
        Operator["⚙️ Hortator Operator<br/>(Go)<br/>Result Cache · Warm Pool"]
        Gateway["🌐 API Gateway<br/>OpenAI-compatible<br/>/v1/chat/completions"]
    end

    subgraph "Agent Pods"
        direction TB
        Tribune["🏛️ Tribune Pod<br/>PVC + Runtime + CLI"]
        Centurion["⚔️ Centurion Pod<br/>PVC + Runtime + CLI"]
        Legionary["🗡️ Legionary Pod<br/>PVC (256Mi) + Runtime + CLI"]
    end

    subgraph "Sidecars (optional)"
        Presidio["🛡️ Presidio<br/>PII Detection"]
    end

    subgraph "Telemetry"
        OTel["📊 OTel Collector"]
        Prom["📈 Prometheus"]
        Grafana["📉 Grafana / Datadog"]
    end

    subgraph "LLM Layer"
        LiteLLM["🔀 LiteLLM Proxy<br/>(optional)"]
        Cloud["☁️ Cloud APIs<br/>OpenAI / Anthropic / Google"]
        Local["🏠 Local Models<br/>Ollama / vLLM"]
    end

    subgraph "Storage"
        PVC_C["💾 Tribune PVC<br/>/memory /inbox /outbox /workspace"]
        PVC_M["💾 Centurion PVC<br/>/memory /inbox /outbox /workspace"]
        Archive["🗄️ Object Storage<br/>S3 / MinIO (optional)"]
        VectorDB["🔍 Vector DB<br/>(optional)"]
    end

    User -->|helm install| Helm
    GitOps -->|manages| Helm
    Helm -->|deploys| Operator
    Helm -->|deploys| Gateway
    Helm -->|installs| CRDs

    Gateway -->|creates| CRDs
    Operator -->|watches| CRDs
    Operator -->|creates| Tribune
    Operator -->|creates| Centurion
    Operator -->|creates| Legionary
    Operator -->|brokers results| Tribune
    Operator -->|brokers results| Centurion

    Tribune -->|hortator spawn| CRDs
    Centurion -->|hortator spawn| CRDs

    Tribune --- PVC_C
    Centurion --- PVC_M
    Tribune -.- Presidio
    Centurion -.- Presidio
    Legionary -.- Presidio

    Tribune -->|LLM calls| LiteLLM
    Centurion -->|LLM calls| LiteLLM
    Legionary -->|LLM calls| LiteLLM
    Tribune -.->|direct| Cloud
    LiteLLM --> Cloud
    LiteLLM --> Local

    Operator -->|OTLP events| OTel
    Operator -->|metrics| Prom
    OTel --> Grafana
    Prom --> Grafana

    Operator -->|archive stale PVCs| Archive
    Operator -->|graduate knowledge| VectorDB

    style Tribune fill:#4a90d9,color:#fff
    style Centurion fill:#d4a029,color:#fff
    style Legionary fill:#7a7a7a,color:#fff
    style Operator fill:#2d6b3f,color:#fff
    style Gateway fill:#2d6b3f,color:#fff
```

## Key Components

| Component | Description |
|-----------|-------------|
| **Operator** | Core reconciler — watches AgentTask CRDs, creates Pods, brokers results, enforces policies |
| **API Gateway** | OpenAI-compatible HTTP API (`/v1/chat/completions`, `/v1/models`). Translates chat requests into AgentTask CRDs. Optional, opt-in via `gateway.enabled`. |
| **Warm Pod Pool** | Pre-provisioned idle Pods that accept tasks immediately (<1s vs 5-30s cold start). Optional, opt-in via `warmPool.enabled`. See [warm-pool.md](warm-pool.md). |
| **Result Cache** | Content-addressable cache keyed on SHA-256(prompt+role). Identical tasks return instantly without spawning Pods. In-memory LRU with TTL. Optional, opt-in via `resultCache.enabled`. |
| **Presidio Service** | Centralized PII detection Deployment+Service (not a sidecar). Agent pods call via cluster DNS. Enterprise feature. |
| **Agentic Runtime** | Python tool-calling loop for tribune/centurion tiers. Uses `litellm` for provider-agnostic LLM calls, supports checkpoint/restore for the reincarnation model. Located at `runtime/agentic/`. |

## Task Lifecycle

```mermaid
sequenceDiagram
    participant U as User / Parent Agent
    participant K as K8s API
    participant O as Hortator Operator
    participant P as Agent Pod
    participant L as LLM Provider

    U->>K: Create AgentTask CRD
    K->>O: Watch event (new task)
    O->>O: Check result cache (hit → complete instantly)
    O->>O: Resolve AgentRole (ns-local → cluster fallback)
    O->>O: Enforce AgentPolicy (capabilities, budget, tier, images)
    O->>O: Inject role rules + flavor into /inbox/task.json
    O->>O: Match retained PVCs by tags → /inbox/context.json
    O->>O: Check warm pool (claim idle Pod or create new)
    O->>K: Create Pod + PVC (all tiers: 1Gi tribune/centurion, 256Mi legionary)
    K->>P: Schedule Pod
    
    activate P
    P->>P: Read /inbox/task.json
    P->>P: Read /inbox/context.json (prior work)
    
    loop Agent execution
        P->>L: LLM call
        L-->>P: Response + usage tokens
        P->>P: hortator report-usage
        P->>P: Update /memory/state.json
        P->>P: Append /memory/decisions.log
        
        opt Spawn sub-agent
            P->>K: hortator spawn → Create child AgentTask
            K->>O: Watch event (child task)
            O->>K: Create child Pod
            Note over O: Child completes → result copied to parent /inbox/
        end
        
        opt Context pressure (>75%)
            P->>P: Trigger summarization
        end
        
        opt Context critical (>90%)
            P->>O: hortator checkpoint
            O->>K: Kill Pod, spawn fresh Pod with same PVC
        end
    end
    
    P->>P: Write /outbox/result.json + /outbox/usage.json
    deactivate P
    
    O->>O: Detect completion
    O->>O: Calculate cost (tokens × LiteLLM price map)
    O->>O: Emit OTel events (task.completed, usage)
    O->>K: Update AgentTask status
    
    opt Has parent task
        O->>O: Copy result to parent /inbox/
        O->>K: Trigger parent's next turn
    end
    
    opt TTL cleanup
        O->>K: Annotate PVC with expires-at
        Note over O: Later: delete expired PVCs (unless retained)
    end
```

## Override Precedence

```mermaid
graph LR
    A["⎈ Helm values.yaml<br/>(cluster defaults)"] --> B["📋 AgentRole CRD<br/>(role overrides)"]
    B --> C["📋 AgentTask CRD<br/>(task overrides)"]
    C --> D["✅ Effective Config"]
    
    style A fill:#e8e8e8,color:#333
    style B fill:#c8d8e8,color:#333
    style C fill:#a8c8d8,color:#333
    style D fill:#4a90d9,color:#fff
```

Applies to: model selection, health thresholds, Presidio config, budget limits, capabilities.

## Storage Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Active: Task created
    Active --> Completed: Task succeeds
    Active --> Waiting: Agent checkpoints (tribune/centurion)
    Active --> Failed: Task fails
    Active --> Cancelled: Task cancelled

    Waiting --> Active: All children complete → reincarnate
    
    Completed --> TTL_Window: PVC annotated (expires-at)
    Failed --> TTL_Window: PVC annotated (shorter TTL)
    Cancelled --> TTL_Window: PVC annotated (shortest TTL)
    
    TTL_Window --> Deleted: TTL expired
    TTL_Window --> Retained: Agent called hortator retain
    
    Retained --> Discoverable: Tag-matched by new tasks
    Discoverable --> Retained: Still matching
    Discoverable --> Stale: No matches in 90 days
    
    Stale --> VectorDB: Vector storage available
    Stale --> Archived: Object storage available
    Stale --> Deleted: Neither available + quota pressure
    
    VectorDB --> Deleted: PVC freed, knowledge searchable
    Archived --> Deleted: PVC freed, data in cold storage
```

## Operator Components

The operator is split into focused files under `internal/controller/`:

| File | Purpose |
|------|---------|
| `agenttask_controller.go` | Main reconciliation loop, phase machine, struct definitions |
| `pod_builder.go` | Pod spec construction, PVC creation, volume mounts, knowledge discovery integration |
| `helpers.go` | Config loading, PVC reader, token extraction, parent notification, child result injection |
| `metrics.go` | Prometheus metrics (tasks, duration, cost, stuck detection) and OTel event emission |
| `budget.go` | LiteLLM price map loader, cost calculation, budget enforcement |
| `health.go` | Stuck detection signal analysis (tool diversity, prompt repetition, staleness), action execution |
| `knowledge.go` | Retained PVC discovery, tag matching, context.json generation |
| `policy.go` | AgentPolicy enforcement |
| `warm_pool.go` | Warm Pod pool management |
| `result_cache.go` | Content-addressable result cache |
