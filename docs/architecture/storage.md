# Storage Model

## Overview

Hortator uses Kubernetes PVCs to provide agents with persistent filesystem access. The `/inbox` volume uses an EmptyDir for operator-written task definitions.

### Tier-Based Storage

All tiers get a PVC so agents can produce artifacts that survive pod termination. The default size varies by tier:

| Tier | Storage Type | Default Size | Use Case |
|------|-------------|-------------|----------|
| **Tribune** | PVC (persistent) | 1Gi | Long-lived strategic tasks |
| **Centurion** | PVC (persistent) | 1Gi | Multi-turn coordination |
| **Legionary** | PVC (persistent) | 256Mi | Single focused tasks |

The `/inbox` volume uses an EmptyDir (operator writes `task.json` via init container or exec), while `/outbox`, `/workspace`, and `/memory` are sub-paths on the PVC.

### Filesystem Contract

Every agent Pod gets four mount points:

| Path | Who Writes | Who Reads | Purpose |
|------|-----------|-----------|---------|
| `/inbox/` | Operator | Agent | Task definition, context, prior work |
| `/outbox/` | Agent | Operator | Results, artifacts, usage report |
| `/memory/` | Agent | Agent | Persistent state across turns |
| `/workspace/` | Agent | Agent | Scratch space for temporary files |

## PVC Lifecycle

See the [architecture overview](overview.md#storage-lifecycle) for the full lifecycle state diagram.

### Cleanup

TTL-based cleanup is configured in Helm values:

```yaml
storage:
  cleanup:
    ttl:
      completed: 7d
      failed: 2d
      cancelled: 1d
```

### Retention

Agents can mark PVCs for retention using `hortator retain --reason "..." --tags "..."`. Retained PVCs are exempt from TTL cleanup and can be automatically mounted (read-only) into future tasks via tag matching.

### Knowledge Discovery

When a new task arrives, the operator checks retained PVCs in the namespace for tag matches and mounts relevant ones at `/prior/<task-name>/`, injecting references into `/inbox/context.json`.

### Quota

```yaml
storage:
  quota:
    enabled: true
    maxPerNamespace: 50Gi
    warningPercent: 80
    evictionPolicy: oldest-completed
```

## Configuration

Full storage configuration is in [`charts/hortator/values.yaml`](https://github.com/hortator-ai/Hortator/blob/main/charts/hortator/values.yaml) under the `storage` section.
