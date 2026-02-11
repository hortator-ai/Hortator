#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# Hortator Meaty Demo — Multi-Tier Task with Substantial Prompts
# ============================================================================
# Creates a tribune orchestrating 5 legionaries doing real work.
# Tasks take 30-60s each so you can watch them in the TUI.
#
# Usage:
#   Terminal 1: hortator watch -n hortator-system --task meaty-tribune
#   Terminal 2: bash examples/e2e/meaty-demo.sh
# ============================================================================

NS="hortator-system"

echo ""
echo "  ⚔  HORTATOR MEATY DEMO"
echo "  ━━━━━━━━━━━━━━━━━━━━━━"
echo "  Creating tribune + 5 legionaries..."
echo "  Watch: hortator watch -n hortator-system --task meaty-tribune"
echo ""

# Clean up any previous run
kubectl delete agenttask meaty-tribune meaty-leg-research meaty-leg-arch meaty-leg-api meaty-leg-schema meaty-leg-plan -n "$NS" --ignore-not-found 2>/dev/null

cat <<'EOF' | kubectl apply -f -
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: meaty-tribune
  namespace: hortator-system
spec:
  prompt: |
    You are a senior architect planning a new microservice for Hortator: a "Task Analytics Service" 
    that collects telemetry from completed agent tasks and provides insights.
    
    Your sub-agents are researching and designing different aspects. Once they report back,
    synthesize their findings into a cohesive architecture decision record (ADR) with:
    - Context and problem statement
    - Decision drivers
    - Considered options (with pros/cons from the research)
    - Decision outcome
    - Consequences (positive and negative)
    
    Format as a proper ADR (Architecture Decision Record). Be thorough.
  role: tech-lead
  tier: tribune
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: meaty-leg-research
  namespace: hortator-system
spec:
  prompt: |
    Research the landscape of observability and analytics for AI agent systems.
    
    Cover these areas in depth:
    1. What metrics matter for AI agent orchestration? (task duration, token usage, cost per task,
       success/failure rates, retry rates, stuck detection signals, hierarchy depth, fan-out ratios)
    2. Compare time-series databases for this use case: Prometheus, InfluxDB, TimescaleDB, ClickHouse.
       Consider write throughput, query flexibility, retention policies, and K8s-native integration.
    3. How do existing platforms handle AI agent analytics? Look at LangSmith, Weights & Biases,
       Arize AI, and Datadog LLM Observability. What can we learn from their approaches?
    4. What visualization approaches work best? Pre-built dashboards (Grafana) vs custom UI vs
       embedded analytics?
    
    Provide a detailed comparison table for the databases and a summary of key insights.
    Include specific numbers where possible (queries/sec, storage per metric, etc).
  role: researcher
  tier: legionary
  parentTaskId: meaty-tribune
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: meaty-leg-arch
  namespace: hortator-system
spec:
  prompt: |
    Design the architecture for a "Task Analytics Service" that sits alongside the Hortator operator.
    
    Requirements:
    - Ingests OpenTelemetry spans and events from the operator (already emitted)
    - Stores time-series metrics: task counts, durations, token usage, costs, per namespace/role/tier
    - Provides a query API for dashboards and alerting
    - Must work both standalone (small clusters) and at scale (1000+ tasks/day)
    - Should integrate with existing Prometheus/Grafana stacks where available
    - Must be deployable as an optional Helm sub-chart
    
    Produce:
    1. A component diagram showing data flow from operator → collector → storage → query → dashboard
    2. Storage schema design (what metrics, what labels/dimensions, what aggregation windows)
    3. API design (REST endpoints for querying analytics)
    4. Resource estimates (CPU/memory/storage for different scale tiers)
    5. Deployment topology options (sidecar vs dedicated deployment vs external service)
    
    Be specific with technology choices and justify each one.
  role: researcher
  tier: legionary
  parentTaskId: meaty-tribune
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: meaty-leg-api
  namespace: hortator-system
spec:
  prompt: |
    Design the REST API specification for a Hortator Task Analytics Service.
    
    The API should support:
    1. Dashboard queries:
       - GET /api/v1/analytics/overview — summary stats (total tasks, success rate, avg duration, total cost)
       - GET /api/v1/analytics/tasks — time-series of task counts by phase, filterable by namespace/role/tier
       - GET /api/v1/analytics/costs — cost breakdown by namespace, role, model, time period
       - GET /api/v1/analytics/performance — p50/p95/p99 task durations by role and tier
       - GET /api/v1/analytics/hierarchy — tree visualization data for task hierarchies
    
    2. Alerting queries:
       - GET /api/v1/analytics/anomalies — tasks that exceeded budget, retried excessively, or got stuck
       - GET /api/v1/analytics/trends — week-over-week cost and usage trends
    
    3. Export:
       - GET /api/v1/analytics/export — CSV/JSON export of raw task data for a time range
    
    For each endpoint, specify:
    - Full URL with query parameters
    - Request/response JSON schemas with example payloads
    - Pagination approach
    - Caching strategy (which endpoints benefit from caching, TTLs)
    - Rate limiting considerations
    
    Write this as an OpenAPI-style specification with concrete examples.
  role: researcher
  tier: legionary
  parentTaskId: meaty-tribune
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: meaty-leg-schema
  namespace: hortator-system
spec:
  prompt: |
    Design the data model and storage schema for a Hortator Task Analytics Service.
    
    The service needs to store and query:
    1. Task lifecycle events (created, started, completed, failed, retried) with timestamps
    2. Token usage per task (input tokens, output tokens, by model)
    3. Cost data per task (estimated USD, by model pricing)
    4. Task hierarchy relationships (parent → children, with depth tracking)
    5. Namespace and role dimensions for multi-tenant filtering
    6. Aggregated metrics at multiple time windows (1min, 5min, 1h, 1d)
    
    Design for TWO storage backends and compare:
    
    Option A: PostgreSQL with TimescaleDB extension
    - Hypertable design for time-series data
    - Continuous aggregates for pre-computed rollups
    - Retention policies
    - Indexes for common query patterns
    - Provide actual CREATE TABLE statements
    
    Option B: ClickHouse
    - MergeTree table design
    - Materialized views for aggregations
    - TTL-based retention
    - ReplicatedMergeTree for HA
    - Provide actual CREATE TABLE statements
    
    For both: estimate storage requirements for 1K, 10K, and 100K tasks/day.
    Include query examples for the most common analytics patterns.
  role: researcher
  tier: legionary
  parentTaskId: meaty-tribune
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: meaty-leg-plan
  namespace: hortator-system
spec:
  prompt: |
    Create a detailed implementation plan for building a Hortator Task Analytics Service.
    
    Break the work into phases with concrete deliverables:
    
    Phase 1 — Foundation (Week 1-2):
    - What Go packages/frameworks to use for the service?
    - How to receive OTel data from the operator?
    - Initial storage setup and schema migration approach
    - Basic health endpoints and Helm chart
    
    Phase 2 — Core Analytics (Week 3-4):
    - Which analytics queries to implement first?
    - Dashboard integration approach (Grafana JSON model? Custom React UI?)
    - Testing strategy (what to mock, integration test approach)
    
    Phase 3 — Production Readiness (Week 5-6):
    - Horizontal scaling approach
    - Backup and disaster recovery for analytics data
    - Alerting integration (PagerDuty/Slack/email)
    - Documentation and runbooks
    
    For each phase:
    - List specific tasks with estimated effort (hours)
    - Identify dependencies and risks
    - Define "done" criteria
    - Estimate total cost (developer hours × rate)
    
    Also consider: should this be open source (MIT) or enterprise? What's the business case?
    Compare build vs buy (could we just use Grafana Cloud + custom dashboards?).
  role: researcher
  tier: legionary
  parentTaskId: meaty-tribune
EOF

echo ""
echo "  ✓ All tasks created. Watch them work!"
echo ""
echo "  Tasks:"
echo "    meaty-tribune       — Tribune (synthesizes ADR from legionary findings)"
echo "    meaty-leg-research  — Research observability landscape"
echo "    meaty-leg-arch      — Architecture design"
echo "    meaty-leg-api       — REST API specification"
echo "    meaty-leg-schema    — Data model & storage schema"
echo "    meaty-leg-plan      — Implementation plan"
echo ""
echo "  Waiting for completion..."

# Wait and report
TIMEOUT=300
ELAPSED=0
while [[ $ELAPSED -lt $TIMEOUT ]]; do
  DONE=0
  TOTAL=6
  for TASK in meaty-tribune meaty-leg-research meaty-leg-arch meaty-leg-api meaty-leg-schema meaty-leg-plan; do
    PHASE=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
    if [[ "$PHASE" == "Completed" || "$PHASE" == "Failed" ]]; then
      DONE=$((DONE+1))
    fi
  done
  
  echo -ne "\r  Progress: ${DONE}/${TOTAL} tasks done (${ELAPSED}s elapsed)   "
  
  if [[ $DONE -eq $TOTAL ]]; then
    echo ""
    echo ""
    echo "  ✓ All tasks completed!"
    echo ""
    
    # Print summary
    echo "  ┌──────────────────────┬───────────┬──────────┐"
    echo "  │ Task                 │ Status    │ Duration │"
    echo "  ├──────────────────────┼───────────┼──────────┤"
    for TASK in meaty-tribune meaty-leg-research meaty-leg-arch meaty-leg-api meaty-leg-schema meaty-leg-plan; do
      PHASE=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.phase}')
      DUR=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.duration}')
      printf "  │ %-20s │ %-9s │ %-8s │\n" "$TASK" "$PHASE" "$DUR"
    done
    echo "  └──────────────────────┴───────────┴──────────┘"
    echo ""
    echo "  View results:"
    echo "    hortator result meaty-tribune -n hortator-system"
    echo "    hortator tree meaty-tribune -n hortator-system"
    exit 0
  fi
  
  sleep 5
  ELAPSED=$((ELAPSED+5))
done

echo ""
echo "  ⏱ Timeout after ${TIMEOUT}s"
exit 1
