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

## Next Up

- Budget enforcement with LiteLLM integration
- Stuck detection + auto-escalation
- Retained PVC knowledge discovery
- Multi-tenancy (cross-namespace policies)
- `hortator watch` TUI

## Future

- Object storage archival
- OIDC/SSO (Enterprise)
- Web dashboard
- Go SDK
