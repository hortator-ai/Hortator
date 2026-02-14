# Contributing to Hortator

Thank you for your interest in contributing to Hortator! This guide will help you get started.

## Development Setup

You'll need:

- **Go 1.25+**
- **kubectl**
- **Helm 3**
- **A Kubernetes cluster** (kind, minikube, k3s, or a remote cluster)
- **golangci-lint** (for linting)

### Clone and build

```bash
git clone https://github.com/hortator-ai/Hortator.git
cd Hortator
make generate        # Generate deepcopy, RBAC, etc.
make manifests       # Generate CRD manifests from Go types
make sync-crds       # Sync CRDs to crds/ and charts/hortator/crds/
go build ./...
```

### Run tests

```bash
go test ./...
```

### Run envtest (controller tests)

envtest uses a local API server and etcd binary instead of a full cluster:

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use
go test ./...
```

### Run the operator locally

```bash
kubectl apply -f crds/
go run cmd/main.go
```

## Project Structure

```
api/v1alpha1/          # CRD Go types (AgentTask, AgentPolicy)
config/crd/bases/      # Generated CRD YAMLs (controller-gen output — do not edit)
crds/                  # Aggregated CRDs: generated + hand-written (single source of truth)
charts/hortator/       # Helm chart (crds/ mirrored into charts/hortator/crds/)
cmd/
  main.go              # Operator entrypoint
  gateway/main.go      # OpenAI-compatible API gateway
  hortator/            # CLI (for use inside agent pods)
internal/
  controller/          # Reconciler, pod builder, policy, warm pool, result cache, metrics
  gateway/             # HTTP handlers, types, helpers
sdk/
  python/              # Python SDK (hortator package)
  typescript/          # TypeScript SDK (@hortator/sdk)
docs/                  # Documentation
```

## CRD Workflow

If you touch files in `api/v1alpha1/`, you **must** regenerate CRDs:

```bash
make generate manifests sync-crds
```

CI runs `make sync-crds && git diff --exit-code` on every PR to catch forgotten syncs.

> **Never edit** files in `config/crd/bases/` or `charts/hortator/crds/` directly. Edit Go types or `crds/agentrole.yaml`, then run `make sync-crds`.

## Pull Request Conventions

- **One feature per PR.** Keep changes focused and reviewable.
- **Rebase on main** before submitting. No merge commits.
- **Conventional commits** are required (see below).
- Add or update tests for any changed behavior.
- Update documentation in `docs/` if behavior changed.

## Commit Message Format

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description
```

**Types:** `feat`, `fix`, `ci`, `docs`, `chore`, `perf`, `refactor`, `test`

**Examples:**

```
feat(controller): add warm pool pre-scaling based on AgentPolicy
fix(gateway): handle nil response body from upstream LLM
docs: add architecture diagram to docs/
ci: add golangci-lint to CI pipeline
chore: bump controller-runtime to v0.20.0
```

## Code Style

- Run **golangci-lint** with the repo config: `golangci-lint run`
- Run **goimports** with local module grouping: `goimports -local github.com/hortator-ai/Hortator`
- All Go source files must include the MIT license header (see `hack/boilerplate.go.txt`)
- Follow standard Go conventions

## Questions?

Open a thread in [GitHub Discussions](https://github.com/hortator-ai/Hortator/discussions). We're happy to help!

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
