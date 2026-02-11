# Roadmap

For the full prioritized backlog, see [backlog.md](https://github.com/michael-niemand/Hortator/blob/main/backlog.md).

## Recently Completed

- **Result cache** — Content-addressable cache keyed on SHA-256(prompt+role). Identical tasks return instantly from cache. In-memory LRU with configurable TTL and max entries.
- **TypeScript SDK** — `@hortator/sdk` with zero runtime deps, streaming, LangChain.js integration.
- **Python SDK** — `hortator` package with sync/async clients, streaming, LangChain + CrewAI integrations.
- **Warm Pod pool** — Pre-provisioned idle Pods for sub-second task assignment. One-shot consumption, exec-based task injection.
- **Controller refactor** — Split into focused files: pod builder, policy, helpers, metrics.
- **Comprehensive unit tests** — Controller, gateway, helpers, pod builder, policy, warm pool, result cache.
- **Code review fixes** — ConfigMap caching (30s TTL), retry jitter, safe resource parsing, pinned init image.
- **`hortator watch` TUI** — Terminal UI showing live task tree, per-agent status, logs, cumulative cost.

## Next Up

- **Python agentic runtime** — Tool-calling loop for Tribunes and Centurions (`runtime/agentic/`). Enables autonomous decomposition, delegation, and consolidation. Legionaries keep the bash single-shot runtime. [Design doc](design-agentic-loop.md)
- **Reincarnation model** — Event-driven Tribune lifecycle: spawn children, checkpoint state, exit. Operator restarts Tribune when children complete. No idle pods, resilient to node failure, solves context overflow. [Design doc](design-agentic-loop.md)
- **Artifact download endpoint** — `GET /api/v1/tasks/{id}/artifacts` on the gateway + `hortator result --artifacts` CLI. Enables async result retrieval for mega-tasks. [Design doc](design-agentic-loop.md)
- Budget enforcement with LiteLLM integration
- Stuck detection + auto-escalation
- Retained PVC knowledge discovery

## Future

- Multi-tenancy (cross-namespace policies)
- Object storage archival
- Webhook callbacks on task completion
- OIDC/SSO (Enterprise)
- Web dashboard
- Go SDK
