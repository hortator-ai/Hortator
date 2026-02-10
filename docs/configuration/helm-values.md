# Helm Values Reference

The full annotated values file is at [`charts/hortator/values.yaml`](https://github.com/michael-niemand/Hortator/blob/main/charts/hortator/values.yaml).

All configuration is Helm-driven — transparent, GitOps-friendly, no custom images needed.

**Three-tier override:** Helm defaults → AgentRole CRD → AgentTask CRD (most specific wins).

## Configuration Areas

| Section | What it controls |
|---------|-----------------|
| `operator.*` | Operator replicas, image, resources, security context, leader election |
| `agent.*` | Default agent image, timeout, resource limits |
| `warmPool.*` | Pre-provisioned idle Pods for sub-second task assignment |
| `resultCache.*` | Content-addressable result cache (TTL, max entries) |
| `models.*` | Default LLM endpoint, model name, API key ref, presets (Ollama/vLLM/LiteLLM) |
| `runtime.*` | Filesystem conventions, context management strategy |
| `storage.*` | PVC cleanup TTLs, retention, knowledge discovery, quotas |
| `budget.*` | Cost tracking, price source, per-task limits, enforcement |
| `health.*` | Stuck detection thresholds, behavioral analysis, per-role overrides |
| `telemetry.*` | OpenTelemetry audit events, distributed traces |
| `presidio.*` | PII detection (centralized service), recognizers, scan thresholds |
| `security.*` | Default capabilities, NetworkPolicy toggle |
| `gateway.*` | OpenAI-compatible API gateway (opt-in), auth, HTTPRoute config |
| `metrics.*` | Prometheus ServiceMonitor configuration |
| `examples.*` | Install quickstart examples (off by default) |
| `enterprise.*` | Enterprise toggle and image |

## Key Opt-in Features

### Warm Pod Pool

```yaml
warmPool:
  enabled: false   # opt-in
  size: 2          # idle Pods per namespace
```

Pre-provisions idle agent Pods. Tasks are assigned to warm Pods instantly (<1s) instead of waiting for image pull + container startup (5-30s). See [Warm Pool design doc](../architecture/warm-pool.md).

### Result Cache

```yaml
resultCache:
  enabled: false      # opt-in
  ttlSeconds: 600     # 10 minute TTL
  maxEntries: 1000    # LRU eviction
```

Content-addressable cache keyed on SHA-256(prompt+role). Identical tasks return from cache instantly without spawning Pods. In-memory only; restarts clear cache. Opt out per task via annotation `hortator.ai/no-cache: "true"`.

### API Gateway

```yaml
gateway:
  enabled: false           # opt-in
  replicas: 1
  authSecret: hortator-gateway-auth
```

Exposes an OpenAI-compatible API (`/v1/chat/completions`, `/v1/models`) so any OpenAI SDK or tool (Cursor, Continue, Cody) can submit work to Hortator. Supports blocking and SSE streaming responses. Bearer token auth against a K8s Secret.
