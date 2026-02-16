# Multi-Tenancy: Namespace Isolation Model

> **Status:** Implemented (MIT/open source)
> **Date:** 2026-02-11

## Overview

Hortator uses **namespace-per-tenant** isolation. Each tenant (team, department, or company) operates in its own Kubernetes namespace. The Hortator operator runs in `hortator-system` and watches all namespaces.

```
┌───────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                         │
│                                                               │
│  ┌───────────────────┐                                        │
│  │  hortator-system  │  Operator, Gateway, Presidio           │
│  └───────────────────┘                                        │
│                                                               │
│  ┌───────────────────┐  ┌───────────────────┐                 │
│  │  tenant-alpha     │  │  tenant-beta      │  ...            │
│  │  ┌──────────────┐ │  │  ┌──────────────┐ │                 │
│  │  │ AgentTasks   │ │  │  │ AgentTasks   │ │                 │
│  │  │ AgentRoles   │ │  │  │ AgentRoles   │ │                 │
│  │  │ Worker SA    │ │  │  │ Worker SA    │ │                 │
│  │  │ NetworkPol   │ │  │  │ NetworkPol   │ │                 │
│  │  │ ResourceQuota│ │  │  │ ResourceQuota│ │                 │
│  │  │ LimitRange   │ │  │  │ LimitRange   │ │                 │
│  │  └──────────────┘ │  │  └──────────────┘ │                 │
│  └───────────────────┘  └───────────────────┘                 │
└───────────────────────────────────────────────────────────────┘
```

## Isolation Layers

### 1. RBAC Isolation

Worker pods run with a namespace-scoped ServiceAccount (`hortator-worker`) that can only CRUD AgentTasks within its own namespace. This is enforced by the `workerRbac` Helm templates (Role + RoleBinding).

A worker in `tenant-alpha` **cannot**:
- Read or create AgentTasks in `tenant-beta`
- Access Secrets, ConfigMaps, or any resource in other namespaces
- Escalate its own permissions

### 2. Network Isolation

NetworkPolicies (enabled via `networkPolicies.enabled: true`) enforce:
- **Default-deny egress** for all agent pods
- **Capability-based allowlists** — only the specific egress rules required by the agent's declared capabilities (e.g., `shell` allows DNS; `spawn` allows API server access)
- Agents cannot communicate with pods in other tenant namespaces

### 3. Storage Isolation

- PVCs are namespace-scoped by Kubernetes design
- Storage quota enforcement via `storage.quota` in values.yaml
- TTL-based cleanup prevents unbounded PVC growth

### 4. Resource Quotas (NEW)

The `tenant-resourcequota.yaml` Helm template enforces hard limits per namespace:
- CPU/memory requests and limits
- Maximum PVCs and pods
- Maximum concurrent AgentTasks (`count/agenttasks.core.hortator.ai`)

### 5. LimitRange (NEW)

The `tenant-limitrange.yaml` Helm template sets default and maximum resource limits per container, preventing any single agent from consuming the entire namespace quota.

## Deployment Patterns

### Pattern A: Shared Cluster (Recommended for Teams)

One Kubernetes cluster hosts multiple tenants. Each tenant gets a namespace with its own quotas, RBAC, and network policies. The operator is shared.

**Pros:** Cost-efficient, simpler operations, shared infrastructure
**Cons:** Noisy-neighbor risk (mitigated by quotas), shared control plane

```bash
# Install operator (once)
helm install hortator hortator/hortator -n hortator-system --create-namespace

# Onboard tenant
kubectl create namespace acme-ai
kubectl label namespace acme-ai hortator.ai/tenant=acme
helm install hortator-tenant-acme hortator/hortator -n acme-ai \
  --set tenant.enabled=true \
  --set tenant.resourceQuota.enabled=true \
  --set tenant.limitRange.enabled=true
```

### Pattern B: Dedicated Cluster

Each tenant (or group of tenants) gets their own cluster. Full blast-radius isolation.

**Pros:** Complete isolation, independent upgrades, no noisy neighbors
**Cons:** Higher cost, more operational overhead

This is a deployment decision — Hortator works identically in both patterns.

## Onboarding a New Tenant

1. **Create namespace** with tenant label:
   ```bash
   kubectl create namespace <tenant-name>
   kubectl label namespace <tenant-name> hortator.ai/tenant=<tenant-name>
   ```

2. **Apply tenant resources** (RBAC, quotas, network policies):
   ```bash
   helm install hortator-tenant-<name> hortator/hortator -n <tenant-name> \
     --set tenant.enabled=true \
     --set tenant.resourceQuota.enabled=true \
     --set tenant.limitRange.enabled=true
   ```

3. **Create API key Secret** for gateway authentication:
   ```bash
   kubectl create secret generic hortator-gateway-auth \
     -n <tenant-name> \
     --from-literal=<api-key-id>=<api-key-value>
   ```

4. **Create AgentRole(s)** with model configuration:
   ```yaml
   apiVersion: core.hortator.ai/v1alpha1
   kind: AgentRole
   metadata:
     name: developer
     namespace: <tenant-name>
   spec:
     defaultModel: claude-sonnet-4-20250514
     defaultEndpoint: https://api.anthropic.com/v1
     apiKeyRef:
       secretName: anthropic-key
       key: api-key
   ```

5. **Done** — tenant can submit tasks via the gateway.

## Enterprise Features (Future)

The following features are planned but **out of scope** for the open-source release:

- **Cross-namespace task spawning** — AgentPolicy controls allowing tasks in namespace A to spawn workers in namespace B
- **Per-tenant egress allowlists** — Fine-grained network rules per tenant (e.g., tenant A can reach GitHub, tenant B cannot)
- **OIDC/SSO per tenant** — Federated identity with per-tenant identity providers

## Security Considerations

- The operator's ClusterRole has broad read access. Restrict operator namespace access via PSPs/PSAs as needed.
- API keys in Secrets should be rotated regularly. Consider external secret management (e.g., Vault, AWS Secrets Manager).
- Audit logging via OpenTelemetry captures all task lifecycle events for compliance.
- Presidio PII scanning runs on all agent output before it leaves the cluster.
