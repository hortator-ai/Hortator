# Enterprise Features

Hortator's enterprise tier adds governance, compliance, and multi-tenancy
capabilities on top of the MIT-licensed core. Enable it with
`enterprise.enabled=true` in your Helm values.

> **Status:** The enterprise binary is not yet published. The features below
> are implemented in the open-source core and available to all users today;
> a future enterprise image will add additional closed-source capabilities.

## Available Today (MIT core)

### AgentPolicy — Namespace-Scoped Governance

`AgentPolicy` CRDs let platform teams enforce guardrails per namespace:

| Control | Description |
|---------|-------------|
| **Allowed / Denied Capabilities** | Whitelist or blacklist agent capabilities (`shell`, `web-fetch`, `spawn`). Denied overrides allowed. |
| **Allowed Images** | Glob patterns for permitted container images. |
| **Max Budget** | Per-task cost (`maxCostUsd`) and token (`maxTokens`) ceilings. |
| **Max Tier** | Highest tier a task may use in the namespace. |
| **Max Concurrent Tasks** | Limit active tasks per namespace. |
| **Require Presidio** | Force PII scanning on all tasks. |
| **Egress Allowlist** | Restrict outbound network to specific hosts/ports. |
| **Allowed / Denied Shell Commands** | Restrict which base commands agents can execute. Denied overrides allowed. |
| **Read-Only Workspace** | Makes `/workspace` read-only for analysis-only tasks. |

See [`api/v1alpha1/agentpolicy_types.go`](../../api/v1alpha1/agentpolicy_types.go) for the full spec.

### Presidio PII Detection

Built-in integration with [Microsoft Presidio](https://microsoft.github.io/presidio/)
scans agent output for PII and secrets. Configurable globally via Helm values
(`presidio.*`) and per-task via `task.spec.presidio`.

Actions: `redact` | `detect` | `hash` | `mask`

Custom recognizers can be added in `values.yaml` (e.g., AWS keys, bearer tokens,
private keys).

### Multi-Tenancy

Namespace-scoped resource quotas and limit ranges via the `tenant.*` Helm values.
Combined with `AgentPolicy`, this provides full tenant isolation.

## Planned (Enterprise Image) — Coming Soon

- **SSO / OIDC integration** for gateway authentication
- **Audit log export** to external SIEM systems
- **Cross-namespace task delegation** with policy federation
- **Usage dashboards** with per-team cost attribution

## Licensing

See [license.md](./license.md) for details on the dual-license model.
