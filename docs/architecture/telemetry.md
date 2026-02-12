# Telemetry & Observability

## Overview

Hortator emits telemetry through two channels:

1. **Prometheus metrics** — quantitative operational data
2. **OpenTelemetry spans/events** — structured audit events with trace correlation

## Prometheus Metrics

The operator exposes three core metrics:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hortator_tasks_total` | Counter | `phase`, `namespace` | Total AgentTasks by phase and namespace |
| `hortator_tasks_active` | Gauge | `namespace` | Currently running AgentTasks |
| `hortator_task_duration_seconds` | Histogram | | Duration of completed tasks (exponential buckets: 1s to ~16384s) |

### ServiceMonitor

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: false     # Set to true if using Prometheus Operator
    interval: 30s
```

## OpenTelemetry Integration

Task hierarchy maps naturally to distributed traces: Tribune → Centurion → Legionary = parent → child spans.

### Event Naming Convention

`hortator.<domain>.<action>`:
- `hortator.task.spawned` / `.completed` / `.failed` / `.cancelled`
- `hortator.presidio.pii_detected`
- `hortator.policy.violation`

### Event Attributes

Each event carries task context:
- `hortator.task.id`, `hortator.task.namespace`
- `hortator.task.phase`, `hortator.task.role`, `hortator.task.tier`
- `hortator.task.parent`

### Configuration

```yaml
telemetry:
  enabled: true
  collector:
    deploy: true       # Deploy OTel Collector via sub-chart
  exporters:
    otlp:
      endpoint: ""     # Your backend (Datadog, Grafana Cloud, Jaeger, etc.)
```

## Implementation

- Metrics: [`internal/controller/metrics.go`](https://github.com/hortator-ai/Hortator/blob/main/internal/controller/metrics.go)
- OTel tracer and event emission are integrated into the reconcile loop via `emitTaskEvent()`
