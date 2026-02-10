# Budget Controls

## Overview

Hortator tracks token usage and estimated cost per task. Budget enforcement prevents runaway spending by autonomous agents.

## How It Works

1. Agents report token usage via `hortator report-usage` CLI after each LLM call
2. Operator calculates cost from LiteLLM's community-maintained price map (refreshed daily)
3. When budget is exceeded, task transitions to `BudgetExceeded` phase

## Configuration

### Per-Task Budget

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: my-task
spec:
  budget:
    maxTokens: 100000      # Total tokens (input + output)
    maxCostUsd: "0.50"     # Dollar cap
```

### Cluster Defaults

```yaml
# values.yaml
budget:
  enabled: true
  priceSource: litellm
  refreshIntervalHours: 24
  fallbackBehavior: track-tokens   # track-tokens | block | warn
  defaultLimit:
    maxCostUsd: "1.00"
```

### LiteLLM Proxy (Enterprise)

For authoritative cost tracking, deploy LiteLLM proxy as an optional sub-chart:

```yaml
budget:
  litellmProxy:
    enabled: true
```

See [Choosing Strategies](choosing-strategies.md#3-budget-tracking-how-accurate-do-you-need-cost-data) for guidance on which approach to use.
