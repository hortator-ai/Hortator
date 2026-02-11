# Roadmap

For the full prioritized backlog, see [backlog.md](https://github.com/michael-niemand/Hortator/blob/main/backlog.md).

## Recently Completed

- **Budget enforcement** — LiteLLM price map integration with 24h auto-refresh cache. Per-task cost calculation, `BudgetExceeded` phase transition, `hortator_task_cost_usd` histogram and `hortator_budget_exceeded_total` counter metrics. Configurable via ConfigMap and per-task `spec.budget`.
- **Stuck detection** — Behavioral analysis of running agents via pod logs. Measures tool diversity, prompt repetition, and progress staleness with weighted aggregate scoring. Configurable `warn`/`kill`/`escalate` actions. Runtime emits prompt hashes for operator-side analysis.
- **Knowledge discovery** — Tag-based retained PVC matching. Completed task PVCs annotated with `hortator.ai/retain-tags` are auto-discovered and mounted read-only at `/prior/<task-name>/` for new tasks. `/inbox/context.json` injected with prior work references.
- **File delivery** — Gateway accepts OpenAI-compatible content part arrays with `{type:"file", file:{filename, file_data}}` attachments. Files are base64-decoded and delivered to `/inbox/` via the init container. Python and TypeScript SDKs updated with `files` parameter.
- **CR garbage collection** — `handleTTLCleanup()` now deletes both the PVC and the AgentTask CR after the configurable retention period. Respects `hortator.ai/retain` annotation. Per-phase TTLs (completed/failed/cancelled) configurable via ConfigMap.
- **Presidio readiness** — `wait_for_presidio()` retry loop (30s) in both bash and agentic runtimes, ensuring PII scanning is active before first LLM call.
- **Extended ClusterDefaults** — Budget, health, storage-retained, and cleanup-TTL config all parsed from the ConfigMap with sensible defaults.
- **Result cache** — Content-addressable cache keyed on SHA-256(prompt+role). Identical tasks return instantly from cache. In-memory LRU with configurable TTL and max entries.
- **TypeScript SDK** — `@hortator/sdk` with zero runtime deps, streaming, LangChain.js integration.
- **Python SDK** — `hortator` package with sync/async clients, streaming, LangChain + CrewAI integrations.
- **Warm Pod pool** — Pre-provisioned idle Pods for sub-second task assignment. One-shot consumption, exec-based task injection.
- **Controller refactor** — Split into focused files: pod builder, policy, helpers, metrics.
- **Comprehensive unit tests** — Controller, gateway, helpers, pod builder, policy, warm pool, result cache.
- **Code review fixes** — ConfigMap caching (30s TTL), retry jitter, safe resource parsing, pinned init image.
- **`hortator watch` TUI** — Terminal UI showing live task tree, per-agent status, logs, cumulative cost.
- **Python agentic runtime** — Tool-calling loop for Tribunes and Centurions (`runtime/agentic/`). Enables autonomous decomposition, delegation, and consolidation. Legionaries keep the bash single-shot runtime. [Design doc](design-agentic-loop.md)
- **Reincarnation model** — Event-driven Tribune lifecycle: spawn children, checkpoint state, exit. Operator restarts Tribune when children complete. No idle pods, resilient to node failure, solves context overflow. [Design doc](design-agentic-loop.md)
- **Artifact download endpoint** — `GET /api/v1/tasks/{id}/artifacts` on the gateway + `hortator result --artifacts` CLI. Enables async result retrieval for mega-tasks. [Design doc](design-agentic-loop.md)

## Next Up

- Full end-to-end validation of Tribune orchestration flow (agentic image build, multi-agent delegation, reincarnation)
- Budget and stuck detection smoke tests in CI
- Per-role and per-tier stuck detection overrides

## Future

- Multi-tenancy (cross-namespace policies)
- Object storage archival
- Webhook callbacks on task completion
- OIDC/SSO (Enterprise)
- Web dashboard
- Go SDK
