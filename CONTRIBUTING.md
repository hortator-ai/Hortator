# Contributing to Hortator

Thank you for your interest in contributing to Hortator!

## Development Setup

1. Fork and clone the repository
2. Install dependencies:
   - Go 1.22+
   - kubectl
   - kubebuilder
   - A Kubernetes cluster (kind, minikube, etc.)

3. Install CRDs:
   ```bash
   make install
   ```

4. Run the operator locally:
   ```bash
   make run
   ```

## Making Changes

1. Create a feature branch
2. Make your changes
3. Run tests: `make test`
4. Run linter: `make lint`
5. Submit a pull request

## Code Style

- Follow standard Go conventions
- Run `gofmt` and `goimports`
- Add tests for new functionality
- Update documentation as needed

## Pull Request Process

1. Update README.md if needed
2. Add tests for new features
3. Ensure all tests pass
4. Request review from maintainers

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
