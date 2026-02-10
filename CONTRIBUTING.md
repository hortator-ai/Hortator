# Contributing to Hortator

Thank you for your interest in contributing to Hortator!

## Development Setup

1. Fork and clone the repository
2. Install dependencies:
   - Go 1.24+
   - kubectl
   - kubebuilder
   - A Kubernetes cluster (kind, minikube, etc.)
   - golangci-lint (for linting)

3. Install CRDs:
   ```bash
   kubectl apply -f crds/
   ```

4. Run the operator locally:
   ```bash
   go run cmd/main.go
   ```

5. Run tests:
   ```bash
   go test ./...
   ```

## Project Structure

```
api/v1alpha1/          # CRD Go types (AgentTask, AgentPolicy)
cmd/
  main.go              # Operator entrypoint
  gateway/main.go      # OpenAI-compatible API gateway
  hortator/            # CLI (for use inside agent pods)
internal/
  controller/          # Reconciler, pod builder, policy, warm pool, result cache, metrics
  gateway/             # HTTP handlers, types, helpers
charts/hortator/       # Helm chart
sdk/
  python/              # Python SDK (hortator package)
  typescript/          # TypeScript SDK (@hortator/sdk)
docs/                  # Documentation
```

## Making Changes

1. Create a feature branch
2. Make your changes
3. Run tests: `go test ./...`
4. Run linter: `golangci-lint run`
5. Update documentation in `docs/` if behavior changed
6. Submit a pull request

## Commit Style

Use [conventional commits](https://www.conventionalcommits.org/):

```
type(scope): description
```

Types: `feat`, `fix`, `improvement`, `build`, `ci`, `chore`, `docs`, `perf`, `refactor`, `revert`, `style`, `test`

## Code Style

- Follow standard Go conventions
- Run `gofmt` and `goimports`
- Add tests for new functionality
- Update documentation as needed

## Pull Request Process

1. Update documentation if you changed behavior
2. Add tests for new features
3. Ensure all tests pass and linter is clean
4. Request review from maintainers

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
