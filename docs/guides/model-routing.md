# Model Routing

## Overview

Hortator is model-agnostic. Agents call whatever LLM endpoint you configure — cloud APIs, local models, or a hybrid via LiteLLM proxy.

## Three Deployment Patterns

### 1. Cloud Only (default)

Agents call cloud APIs directly. Configure once in Helm values:

```yaml
models:
  default:
    endpoint: https://api.anthropic.com/v1
    name: claude-sonnet-4-20250514
    apiKeyRef:
      secretName: llm-keys
      key: api-key
```

### 2. Local Only (air-gapped)

Agents call Ollama or vLLM in-cluster:

```yaml
models:
  presets:
    ollama:
      enabled: true
      endpoint: http://ollama.default.svc:11434/v1
```

### 3. Hybrid (LiteLLM Proxy)

LiteLLM proxy routes between local and cloud based on cost/availability:

```yaml
models:
  presets:
    litellm:
      enabled: true
      endpoint: http://litellm.default.svc:4000/v1
```

## Override Precedence

Model configuration follows the three-tier override pattern:

1. **Helm values** — cluster-wide defaults
2. **AgentRole** — role-specific model
3. **AgentTask** — per-task override (most specific wins)

See [Choosing Strategies](choosing-strategies.md#2-model-routing-which-llm-should-agents-use) for detailed guidance on when to use each pattern.
