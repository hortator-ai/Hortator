# PII Detection with Presidio

## Overview

Hortator integrates [Microsoft Presidio](https://github.com/microsoft/presidio) for PII and secrets detection in agent output. Presidio runs as a **centralized Deployment+Service** in the operator namespace (not as a sidecar).

## Architecture

```
Agent Pod → HTTP call → Presidio Service → Response with PII findings
```

Agent pods call the Presidio service via cluster DNS. This avoids the sidecar exit-code-137 problem and simplifies pod lifecycle.

## Configuration

```yaml
# values.yaml
presidio:
  enabled: true
  image: mcr.microsoft.com/presidio-analyzer:latest
  replicas: 1
  model: en_core_web_sm
  scoreThreshold: 0.5
  action: redact       # redact | detect | hash | mask

  recognizers:
    disabled:
      - PhoneRecognizer     # Slow: iterates all country codes
    custom:
      - name: AWSKeyRecognizer
        entity: AWS_ACCESS_KEY
        patterns:
          - regex: "AKIA[0-9A-Z]{16}"
            score: 0.95
```

## Actions

| Action | Behavior |
|--------|----------|
| `detect` | Log PII findings as OTel events, don't modify output |
| `redact` | Replace detected PII with placeholder tokens |
| `hash` | Replace detected PII with hashed values |
| `mask` | Partially mask detected PII |

## Per-Task Override

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
spec:
  presidio:
    scoreThreshold: 0.8
    action: detect
    configRef: custom-presidio-config
```

## Performance

- `en_core_web_sm` + regex recognizers: <10ms per ~1,000 tokens
- PhoneRecognizer is disabled by default due to performance (iterates all country codes)
- Scale the `replicas` count for high-throughput clusters

See [Choosing Strategies](choosing-strategies.md#4-pii-detection-when-to-enable-presidio) for guidance on when to enable Presidio.
