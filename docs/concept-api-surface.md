# Hortator External API Surface: Design Concept

**Version:** 1.0  
**Date:** February 2026  
**Author:** Hortator Core Team  

## Executive Summary

This document proposes a comprehensive external API strategy for Hortator, the Kubernetes-native AI agent orchestrator. The design centers around a three-tier API architecture: OpenAI-compatible high-level API for immediate tool compatibility, REST/gRPC mid-level API for platform integration, and direct CRD access for power users. The key insight is that different clients have fundamentally different needs — coding tools want speed and familiarity, enterprise platforms want observability and control, and infrastructure teams want native Kubernetes integration.

**Recommendation:** Implement MVP with OpenAI-compatible API plus fast-path optimization, followed by streaming enhancements and SDK development.

## 1. API Surface Design

### Layered Architecture

```
┌─────────────────────────────────────────────────┐
│ High-Level API (OpenAI-Compatible)             │
│ /v1/chat/completions, /v1/models               │
│ Target: Cursor, Continue, Cody, etc.           │
└─────────────────┬───────────────────────────────┘
                  │
┌─────────────────┴───────────────────────────────┐
│ Mid-Level API (REST/gRPC)                      │
│ /api/v1/tasks, /api/v1/orchestrations          │
│ Target: Platforms, CI/CD, custom apps          │
└─────────────────┬───────────────────────────────┘
                  │
┌─────────────────┴───────────────────────────────┐
│ Low-Level API (Kubernetes CRDs)                │
│ AgentTask, Orchestration, TaskResult CRDs      │
│ Target: Infrastructure teams, advanced users    │
└─────────────────────────────────────────────────┘
```

### API Inventory

**Essential APIs:**
- **OpenAI-Compatible** (`/v1/*`): Drop-in replacement for existing tools
- **REST API** (`/api/v1/*`): Native Hortator concepts with full control
- **WebSocket API** (`/ws/*`): Real-time streaming and bidirectional communication
- **CRD Direct Access**: Full Kubernetes-native control plane

**Future APIs:**
- **gRPC API**: High-performance platform integration
- **GraphQL API**: Complex query patterns for dashboards
- **MCP Server**: Model Context Protocol for agent composition

### Python SDK *(Implemented)*

> **Note:** The Python SDK has been implemented — see [`sdk/python/`](https://github.com/hortator-ai/Hortator/tree/main/sdk/python) and `pip install hortator`. The actual API follows OpenAI conventions with Hortator extensions (capabilities, tiers, budgets). The conceptual design below was the original proposal; the shipped API is simpler.

```python
from hortator import HortatorClient, TaskConfig

# High-level (OpenAI-style)
client = HortatorClient(api_key="hrt_123...", base_url="https://hortator.company.com")

response = client.chat.completions.create(
    model="hortator/coding-expert",
    messages=[{"role": "user", "content": "Fix the memory leak in auth.py"}],
    stream=True,
    hortator_mode="fast"  # Custom parameter
)

# Mid-level (Native Hortator)
task = client.tasks.create(
    task_type="code_review",
    config=TaskConfig(
        decomposition_strategy="aggressive",
        max_agents=5,
        timeout="10m"
    ),
    files=["src/auth.py", "tests/test_auth.py"]
)

# Real-time streaming
for event in task.stream():
    if event.type == "agent_spawned":
        print(f"Agent {event.agent_id} starting: {event.task}")
    elif event.type == "partial_result":
        print(f"Progress: {event.content}")
    elif event.type == "completed":
        return event.result

# Low-level (Direct CRD)
from kubernetes import client as k8s_client

task_crd = k8s_client.CustomResourcesApi().create_namespaced_custom_object(
    group="hortator.io", version="v1", namespace="default", 
    plural="agenttasks", body={...}
)
```

## 2. Response Streaming & Progress Updates

### Challenge: Multi-Agent Latency

Traditional LLM APIs return a single response from one model. Hortator orchestrates multiple agents across a hierarchy, introducing significant latency (5-30 seconds for complex tasks). Users need immediate feedback and progress visibility.

### Streaming Strategy

**Three Verbosity Levels:**

1. **Default Mode** (OpenAI-compatible):
   - Stream only final content as it's being generated
   - Hide orchestration complexity
   - Compatible with existing tools

2. **Verbose Mode** (Progress events):
   - Key milestones: task decomposition, agent assignment, major completions
   - Estimated time remaining
   - High-level status updates

3. **Full Mode** (Complete observability):
   - Every agent message
   - Resource allocation events
   - Debug information
   - Performance metrics

### OpenAI-Compatible Streaming

```javascript
// Standard OpenAI streaming
const response = await openai.chat.completions.create({
  model: "hortator/coding-expert",
  messages: [{"role": "user", "content": "Refactor this component for better performance"}],
  stream: true,
  extra_headers: {
    "X-Hortator-Verbosity": "verbose"  // Custom header
  }
});

// Default: Only final content streams
for await (const chunk of response) {
  if (chunk.choices[0]?.delta?.content) {
    process.stdout.write(chunk.choices[0].delta.content);
  }
}

// Verbose: Progress events mixed in
for await (const chunk of response) {
  if (chunk.choices[0]?.delta?.content) {
    process.stdout.write(chunk.choices[0].delta.content);
  } else if (chunk.hortator_event) {
    console.log(`[${chunk.hortator_event.type}] ${chunk.hortator_event.message}`);
  }
}
```

### Event Stream Format

```json
// Progress event (verbose mode)
{
  "hortator_event": {
    "type": "task_decomposed",
    "timestamp": "2026-02-09T10:33:00Z",
    "message": "Task split into 3 subtasks",
    "details": {
      "subtasks": ["analyze_performance", "identify_bottlenecks", "refactor_code"],
      "estimated_time": "45s"
    }
  }
}

// Content chunk (always present)
{
  "choices": [{
    "index": 0,
    "delta": {"content": "Looking at your component, I can see several optimization opportunities..."},
    "finish_reason": null
  }],
  "model": "hortator/coding-expert",
  "created": 1707471180
}
```

## 3. Fast Response Times

### The Speed Problem

Coding tools expect LLM-like response times (< 2 seconds to first token). Multi-agent decomposition can take 10-30 seconds before any useful output. This breaks the developer experience.

### Fast-Path Architecture

```
Request → Tribune Intelligence Router → Decision (< 500ms)
                    │
    ┌───────────────┼───────────────┐
    ▼               ▼               ▼
Fast Path      Standard Path   Complex Path
(< 2s)         (5-15s)         (15-60s)
    │               │               │
Single Agent   Multi-Agent     Full Orchestra
Direct LLM     (2-5 agents)    (5+ agents)
    │               │               │
    └───────────────┼───────────────┘
                    ▼
                Response Stream
```

### Intelligence Router Logic

```python
class TribuneRouter:
    def route_request(self, messages: List[Message], model: str) -> RoutingDecision:
        # Analyze request complexity
        complexity = self.analyze_complexity(messages)
        
        if complexity.score < 0.3:
            # Simple question, code explanation, quick fixes
            return RoutingDecision(
                path="fast",
                agent_type="generalist",
                max_time="2s"
            )
        
        elif complexity.score < 0.7:
            # Moderate: refactoring, debugging, review
            return RoutingDecision(
                path="standard", 
                decompose=True,
                max_agents=3,
                max_time="15s"
            )
        
        else:
            # Complex: architecture, large refactors, research
            return RoutingDecision(
                path="complex",
                decompose_strategy="aggressive",
                max_agents=8,
                max_time="60s"
            )

    def analyze_complexity(self, messages: List[Message]) -> ComplexityScore:
        indicators = {
            "simple": ["explain", "what is", "how to", "example"],
            "moderate": ["refactor", "optimize", "debug", "review"],
            "complex": ["architecture", "redesign", "migrate", "research"]
        }
        # ML model or rule-based classification
        return ComplexityScore(score=0.6, reasoning="Contains refactor keywords")
```

### Performance Optimizations

**Warm Agent Pools:**
- Pre-spawned pods for common agent types
- Keep 2-3 generalist agents running per namespace
- Hot standby for popular model combinations

**Connection Keepalive:**
- WebSocket connections for repeat clients
- Session affinity for multi-request conversations
- Client-side caching of task decomposition patterns

**Immediate Streaming:**
- Start streaming within 200ms regardless of routing decision
- Stream thinking process while agents spin up
- Progressive enhancement of response quality

## 4. Beyond Coding — Universal AI Tasks

### The Broader Vision

Hortator's hierarchical orchestration applies to any complex AI task requiring decomposition, specialization, and coordination. The API should support diverse domains while maintaining the familiar OpenAI interface.

### Role-Based Model Routing

```python
# Model naming convention: hortator/{domain}-{role}
AVAILABLE_MODELS = {
    # Coding domain
    "hortator/coding-expert": "General software development",
    "hortator/security-reviewer": "Code security analysis", 
    "hortator/performance-optimizer": "Performance tuning specialist",
    
    # Legal domain
    "hortator/legal-reviewer": "Contract and document review",
    "hortator/legal-researcher": "Case law and statute research",
    "hortator/legal-writer": "Legal document drafting",
    
    # Research domain
    "hortator/researcher": "Academic and business research",
    "hortator/data-analyst": "Statistical analysis specialist",
    "hortator/market-researcher": "Market analysis and trends",
    
    # Content domain
    "hortator/technical-writer": "Documentation and guides",
    "hortator/copywriter": "Marketing and promotional content",
    "hortator/editor": "Content review and improvement"
}
```

### Domain-Specific Parameters

```javascript
// Legal review example
await openai.chat.completions.create({
  model: "hortator/legal-reviewer",
  messages: [{"role": "user", "content": "Review this software license agreement"}],
  hortator_config: {
    jurisdiction: "EU", // Legal domain parameter
    expertise_areas: ["data_protection", "liability"], 
    review_depth: "comprehensive",
    regulatory_frameworks: ["GDPR", "AI_Act"]
  }
});

// Research example  
await openai.chat.completions.create({
  model: "hortator/researcher", 
  messages: [{"role": "user", "content": "Analyze the impact of remote work on productivity"}],
  hortator_config: {
    sources: ["academic", "industry_reports"], // Research domain parameter
    time_range: "2020-2026",
    geographic_scope: "global", 
    methodology: "systematic_review"
  }
});

// Data analysis example
await openai.chat.completions.create({
  model: "hortator/data-analyst",
  messages: [{"role": "user", "content": "Find patterns in our customer churn data"}],
  hortator_config: {
    dataset_location: "s3://company-data/churn/", // Data domain parameter
    analysis_type: "exploratory",
    statistical_methods: ["regression", "clustering"],
    visualization_required: true
  }
});
```

### Integration with Non-Coding Tools

**Legal Firms:**
```python
# Document review workflow
client = HortatorClient(base_url="https://legal.lawfirm.com/hortator")

review_task = client.tasks.create(
    task_type="document_review",
    files=["contract_v1.pdf", "contract_v2.pdf"],
    config={
        "review_type": "change_analysis",
        "jurisdiction": "New York",
        "practice_areas": ["corporate", "IP"],
        "urgency": "high"
    }
)

# Integrates with existing legal software
for event in review_task.stream():
    if event.type == "risk_identified":
        # Auto-create case in legal management system
        case_mgmt.create_flag(
            document=event.document,
            risk=event.risk_description,
            severity=event.severity
        )
```

**Research Teams:**
```python
# Academic research pipeline
research_client = HortatorClient(base_url="https://research.university.edu/hortator")

literature_review = research_client.tasks.create(
    task_type="literature_review",
    query="quantum computing error correction",
    config={
        "databases": ["arxiv", "ieee", "springer"],
        "date_range": "2020-present", 
        "citation_style": "APA",
        "evidence_level": "systematic_review"
    }
)

# Integrates with reference management
for paper in literature_review.results.papers:
    zotero.add_reference(paper.citation, tags=paper.themes)
```

## 5. Integration Patterns

### Drop-in Replacement Pattern

**Objective:** Existing OpenAI tools work immediately with zero code changes.

```bash
# Point any OpenAI-compatible tool at Hortator
export OPENAI_BASE_URL="https://hortator.company.com/v1"
export OPENAI_API_KEY="hrt_abc123..."

# Cursor, Continue, Cody, etc. now use Hortator
cursor --model "hortator/coding-expert"
```

**Benefits:** 
- Instant adoption by development teams
- Gradual migration path from OpenAI
- Leverage existing tool ecosystem

**Limitations:**
- Can't expose Hortator-specific features through standard API
- Limited to OpenAI's parameter schema

### Sidecar Pattern

**Objective:** Hortator complements existing LLM infrastructure rather than replacing it.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ai-gateway
spec:
  template:
    spec:
      containers:
      - name: main-app
        image: company/ai-service:latest
        ports:
        - containerPort: 8000
      - name: hortator-sidecar
        image: hortator/sidecar:latest
        ports:
        - containerPort: 8001
        env:
        - name: HORTATOR_MODE
          value: "companion"
```

**Use Cases:**
- Complex tasks route to Hortator, simple ones to standard LLM
- Hortator provides specialized capabilities (code review, research)
- Existing infrastructure handles auth, logging, monitoring

### Gateway Pattern (LiteLLM Integration)

```yaml
# litellm_config.yaml
model_list:
  - model_name: gpt-4
    litellm_params:
      model: openai/gpt-4
      api_key: os.environ/OPENAI_API_KEY
      
  - model_name: hortator-coding
    litellm_params:
      model: hortator/coding-expert
      api_base: https://hortator.company.com/v1
      api_key: os.environ/HORTATOR_API_KEY
      
  - model_name: hortator-research  
    litellm_params:
      model: hortator/researcher
      api_base: https://hortator.company.com/v1
      api_key: os.environ/HORTATOR_API_KEY

router_settings:
  routing_strategy: "cost-based"  # Route to Hortator for complex tasks
```

**Benefits:**
- Unified API across multiple providers
- Intelligent routing based on task complexity
- Cost optimization (use cheaper models when appropriate)
- Observability across all models

### Webhook/Callback Pattern

**Objective:** Support long-running tasks that exceed HTTP timeout limits.

```python
# Submit long-running task
response = requests.post("https://hortator.company.com/api/v1/tasks", json={
    "type": "architecture_review",
    "repository": "https://github.com/company/monolith",
    "callback_url": "https://company.com/webhooks/hortator",
    "timeout": "2h"
})

task_id = response.json()["task_id"]

# Receive progress callbacks
@app.post("/webhooks/hortator")
def handle_hortator_webhook(data: HortatorWebhook):
    if data.event_type == "task_completed":
        # Process results
        results = data.results
        notify_team(f"Architecture review completed: {results.summary}")
        
    elif data.event_type == "task_progress":
        # Update dashboard
        update_progress_bar(data.task_id, data.progress_percent)
```

### Model Context Protocol (MCP) Server

**Objective:** Hortator as an MCP server for agent composition and tool orchestration.

```json
{
  "mcpVersion": "2024-11-05",
  "capabilities": {
    "resources": {
      "subscribe": true,
      "listChanged": true
    },
    "tools": {
      "listChanged": true
    }
  },
  "serverInfo": {
    "name": "hortator-mcp",
    "version": "1.0.0"
  }
}
```

**MCP Tools Exposed by Hortator:**
```python
# Tools available to MCP clients
HORTATOR_MCP_TOOLS = [
    {
        "name": "orchestrate_task",
        "description": "Decompose complex task into specialized agents",
        "inputSchema": {
            "type": "object", 
            "properties": {
                "task_description": {"type": "string"},
                "domain": {"enum": ["coding", "research", "legal", "analysis"]},
                "complexity": {"enum": ["simple", "moderate", "complex"]}
            }
        }
    },
    {
        "name": "specialist_agent",
        "description": "Route to domain-specific expert agent", 
        "inputSchema": {
            "type": "object",
            "properties": {
                "agent_type": {"type": "string"},
                "context": {"type": "object"},
                "files": {"type": "array"}
            }
        }
    }
]
```

## 6. Authentication & Multi-tenancy

### Authentication Methods

**API Key Authentication (Primary):**
```http
POST /v1/chat/completions
Authorization: Bearer hrt_sk_1234567890abcdef
Content-Type: application/json

{
  "model": "hortator/coding-expert",
  "messages": [...]
}
```

**OIDC/JWT Authentication (Enterprise):**
```http
POST /v1/chat/completions  
Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json
```

**Kubernetes Service Account (Internal):**
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hortator-client
  namespace: development
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hortator-task-creator
rules:
- apiGroups: ["hortator.io"]
  resources: ["agenttasks"]
  verbs: ["create", "get", "list", "watch"]
```

### Multi-tenant Architecture

**Namespace Mapping:**
```yaml
# API Key → Namespace mapping
apiVersion: v1
kind: ConfigMap
metadata:
  name: hortator-tenant-config
  namespace: hortator-system
data:
  tenant-mapping.yaml: |
    tenants:
      hrt_sk_dev_team_alpha: 
        namespace: "hortator-dev-alpha"
        budget_limit: "1000 tokens/hour"
        allowed_models: ["hortator/coding-expert", "hortator/security-reviewer"]
        
      hrt_sk_legal_dept:
        namespace: "hortator-legal" 
        budget_limit: "5000 tokens/hour"
        allowed_models: ["hortator/legal-*"]
        data_retention: "7 years"
        
      hrt_sk_research_lab:
        namespace: "hortator-research"
        budget_limit: "unlimited"
        allowed_models: ["*"]
        priority: "high"
```

**Resource Isolation:**
```yaml
# Each tenant gets dedicated namespace with resource quotas
apiVersion: v1
kind: ResourceQuota
metadata:
  name: hortator-quota
  namespace: hortator-dev-alpha
spec:
  hard:
    requests.cpu: "4"
    requests.memory: 8Gi
    persistentvolumeclaims: "10"
    pods: "20"
    hortator.io/agenttasks: "50"  # Custom resource quota
```

### Rate Limiting & Quota Management

```python
# Rate limiting implementation
class HortatorRateLimiter:
    def __init__(self, redis_client):
        self.redis = redis_client
    
    def check_rate_limit(self, api_key: str, endpoint: str) -> RateLimitResult:
        tenant_config = self.get_tenant_config(api_key)
        
        # Token-based limiting
        token_key = f"tokens:{api_key}:{datetime.now().hour}"
        current_tokens = self.redis.get(token_key) or 0
        
        if current_tokens >= tenant_config.token_limit:
            return RateLimitResult(
                allowed=False,
                reason="Token limit exceeded",
                reset_time=datetime.now().replace(minute=0, second=0) + timedelta(hours=1)
            )
        
        # Request-based limiting  
        request_key = f"requests:{api_key}:{datetime.now().minute}"
        current_requests = self.redis.get(request_key) or 0
        
        if current_requests >= tenant_config.requests_per_minute:
            return RateLimitResult(
                allowed=False, 
                reason="Request rate limit exceeded",
                reset_time=datetime.now().replace(second=0) + timedelta(minutes=1)
            )
            
        return RateLimitResult(allowed=True)
```

**Priority Queues:**
```python
# Task priority based on tenant configuration
class TaskScheduler:
    def schedule_task(self, task: AgentTask, tenant_config: TenantConfig):
        priority = self.calculate_priority(tenant_config)
        
        # High-priority tenants get faster agent allocation
        if tenant_config.priority == "high":
            task.spec.schedulingPolicy = "immediate"
            task.spec.resourceClass = "premium"
        elif tenant_config.priority == "standard":
            task.spec.schedulingPolicy = "balanced"
            task.spec.resourceClass = "standard"
        else:
            task.spec.schedulingPolicy = "best-effort"
            task.spec.resourceClass = "economy"
```

## 7. Technical Implementation Details

### API Gateway Architecture

```
┌─────────────────────────────────────────────────────┐
│                 Load Balancer                        │
│            (nginx/envoy/cloud)                       │
└─────────────────┬───────────────────────────────────┘
                  │
┌─────────────────┴───────────────────────────────────┐
│              API Gateway                            │
│  - Authentication                                   │ 
│  - Rate limiting                                    │
│  - Request routing                                  │
│  - Protocol translation                             │
└─────────────────┬───────────────────────────────────┘
                  │
    ┌─────────────┼─────────────┐
    ▼             ▼             ▼
┌─────────┐ ┌──────────┐ ┌───────────┐
│OpenAI   │ │REST API  │ │WebSocket  │
│Compat   │ │Service   │ │Service    │
│Service  │ │          │ │           │
└─────────┘ └──────────┘ └───────────┘
    │             │             │
    └─────────────┼─────────────┘
                  │
┌─────────────────┴───────────────────────────────────┐
│            Tribune Controller                       │
│  - Task decomposition                               │
│  - Agent orchestration                              │
│  - Result aggregation                               │
└─────────────────────────────────────────────────────┘
```

### Data Flow

```
Client Request → API Gateway → Authentication → Rate Limiting → 
Protocol Translation → Tribune Controller → Task Decomposition →
Agent Scheduling → Centurion Managers → Legionary Workers →
Result Collection → Response Assembly → Stream to Client
```

### Database Schema

```sql
-- Tenants and API keys
CREATE TABLE tenants (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(63) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    config JSONB NOT NULL
);

CREATE TABLE api_keys (
    key_hash VARCHAR(64) PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id),
    name VARCHAR(255),
    permissions JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    last_used_at TIMESTAMP
);

-- Task tracking
CREATE TABLE tasks (
    id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id),
    parent_task_id UUID REFERENCES tasks(id),
    status VARCHAR(50) NOT NULL,
    task_type VARCHAR(100) NOT NULL,
    model VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    config JSONB,
    results JSONB
);

-- Usage tracking for billing/quotas
CREATE TABLE usage_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id),
    task_id UUID REFERENCES tasks(id),
    event_type VARCHAR(50) NOT NULL,
    token_count INTEGER,
    compute_seconds DECIMAL,
    timestamp TIMESTAMP DEFAULT NOW(),
    metadata JSONB
);
```

## 8. MVP Scope & Development Phases

### Phase 1: Core API (Months 1-2)

**Scope:**
- OpenAI-compatible `/v1/chat/completions` endpoint
- Basic authentication with API keys
- Simple fast-path routing (smart/dumb task classification)
- Single-tenant MVP deployment
- Python SDK with OpenAI compatibility layer

**Success Criteria:**
- Cursor/Continue integration works out of the box
- < 2 second response time for simple queries
- < 15 second response time for moderate queries
- 99.5% uptime on test workloads

**Technical Tasks:**
- [ ] API Gateway with OpenAI schema validation
- [ ] Tribune intelligence router with complexity scoring
- [ ] Fast-path single-agent execution
- [ ] Basic streaming implementation
- [ ] Authentication service
- [ ] Python SDK with chat completions interface
- [ ] Integration tests with popular coding tools

### Phase 2: Streaming & Observability (Months 3-4)

**Scope:**
- Full SSE streaming with progress events
- Verbose/full streaming modes
- REST API for task management
- WebSocket real-time API
- Multi-tenant namespace mapping
- Rate limiting and quota management

**Success Criteria:**
- Real-time progress visibility for long-running tasks
- Multi-tenant isolation working correctly
- Rate limiting prevents abuse
- Detailed metrics and logging

**Technical Tasks:**
- [ ] SSE streaming with custom event types
- [ ] WebSocket bidirectional communication
- [ ] Tenant management API and database schema
- [ ] Rate limiting with Redis backend
- [ ] Usage tracking and analytics
- [ ] Monitoring dashboards

### Phase 3: Advanced Features (Months 5-6)

> **Note:** Python and TypeScript SDKs have been implemented ahead of schedule. See [`sdk/python/`](https://github.com/hortator-ai/Hortator/tree/main/sdk/python) and [`sdk/typescript/`](https://github.com/hortator-ai/Hortator/tree/main/sdk/typescript).

**Scope:**
- Domain-specific models (legal, research, etc.)
- Advanced task decomposition strategies
- Webhook callbacks for long-running tasks
- ~~Go and TypeScript SDKs~~ *(TypeScript SDK shipped; Go SDK pending)*
- MCP server implementation

**Success Criteria:**
- Successfully supporting non-coding use cases
- SDK adoption by partners
- Complex multi-hour tasks completing reliably
- MCP integration with agent frameworks

**Technical Tasks:**
- [ ] Domain-specific agent configurations
- [ ] Webhook delivery system with retry logic
- [ ] Go SDK with idiomatic interfaces
- [ ] TypeScript SDK with type safety
- [ ] MCP protocol implementation
- [ ] Advanced orchestration algorithms

### Phase 4: Enterprise Features (Months 7+)

**Scope:**
- OIDC/SAML enterprise authentication
- Advanced security and compliance features
- GraphQL API for complex queries
- gRPC API for high-performance integration
- Advanced analytics and cost allocation
- SLA guarantees and premium support

**Out of MVP Scope:**
- Complex billing/metering (use simple token counting)
- Advanced security scanning (basic input validation only)
- Multi-region deployment
- Advanced caching strategies
- AI model fine-tuning interfaces

## 9. Risk Mitigation

### Performance Risks

**Risk:** Multi-agent orchestration is too slow for interactive tools
**Mitigation:** 
- Fast-path routing for simple queries
- Warm agent pools
- Immediate streaming with progressive enhancement
- Circuit breakers to fall back to single agents

**Risk:** Streaming overhead impacts performance
**Mitigation:**
- Default mode minimizes stream events
- Buffered streaming to reduce HTTP overhead
- WebSocket connections for high-frequency clients

### Security Risks

**Risk:** Multi-tenancy leaks data between customers
**Mitigation:**
- Kubernetes namespace isolation
- Network policies between tenants
- Audit logging of all cross-tenant operations
- Regular security reviews of isolation boundaries

**Risk:** API key compromise
**Mitigation:**
- Key rotation capabilities
- Fine-grained permissions per key
- Usage monitoring for anomaly detection
- Rate limiting to contain damage

### Scalability Risks

**Risk:** Agent pods consume too many cluster resources
**Mitigation:**
- Resource quotas per tenant
- Automatic scaling policies
- Agent termination after idle timeout
- Priority-based scheduling

**Risk:** Database becomes bottleneck for task tracking
**Mitigation:**
- Read replicas for analytics queries
- Time-series database for usage events
- Async task updates where possible
- Database sharding by tenant if needed

## 10. Conclusion

This API design positions Hortator as both a drop-in OpenAI replacement and a powerful orchestration platform. The layered approach allows different client types to access appropriate levels of functionality, from simple coding tools to complex enterprise integrations.

**Key Success Factors:**
1. **Developer Experience:** OpenAI compatibility ensures immediate adoption
2. **Performance:** Fast-path routing keeps simple queries responsive
3. **Observability:** Streaming progress makes complex orchestration transparent
4. **Extensibility:** Domain-specific models support diverse use cases beyond coding

**Recommended Implementation Order:**
1. MVP OpenAI API with fast-path optimization
2. Streaming and multi-tenancy for production deployment
3. SDK development and ecosystem integration
4. Enterprise features and advanced orchestration

The design balances immediate utility (coding tool integration) with long-term vision (universal AI task orchestration). Success will be measured by both technical metrics (response time, uptime) and adoption metrics (number of integrated tools, diversity of use cases).

This architecture supports Hortator's evolution from a Kubernetes-native orchestrator to the de facto standard for complex AI task execution across industries.