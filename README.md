# Hortator

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev/)

**Kubernetes-native AI agent orchestration operator.**

Hortator manages AI agent workloads as native Kubernetes resources, providing full lifecycle management, observability, and scaling for agent-based automation.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Kubernetes Cluster                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐     ┌──────────────────────────────────────────────┐  │
│  │   hortator CLI   │────▶│              Kubernetes API                  │  │
│  └──────────────────┘     └───────────────────┬──────────────────────────┘  │
│                                               │                              │
│  ┌────────────────────────────────────────────┼────────────────────────────┐│
│  │                     Hortator Operator      │                            ││
│  │  ┌─────────────────────────────────────────▼──────────────────────────┐ ││
│  │  │                    AgentTask Controller                            │ ││
│  │  │                                                                    │ ││
│  │  │  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────────────┐ │ ││
│  │  │  │  Watch   │──▶│  Create  │──▶│ Monitor  │──▶│ Update Status    │ │ ││
│  │  │  │  Tasks   │   │   Pods   │   │  Pods    │   │ on Completion    │ │ ││
│  │  │  └──────────┘   └──────────┘   └──────────┘   └──────────────────┘ │ ││
│  │  └────────────────────────────────────────────────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                         Agent Task Pods                                 ││
│  │                                                                         ││
│  │  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                   ││
│  │  │  task-abc   │   │  task-def   │   │  task-xyz   │    ...            ││
│  │  │  ┌───────┐  │   │  ┌───────┐  │   │  ┌───────┐  │                   ││
│  │  │  │ Agent │  │   │  │ Agent │  │   │  │ Agent │  │                   ││
│  │  │  └───────┘  │   │  └───────┘  │   │  └───────┘  │                   ││
│  │  └─────────────┘   └─────────────┘   └─────────────┘                   ││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Description |
|-----------|-------------|
| **Operator** | Kubernetes controller that watches AgentTask CRDs and manages agent pod lifecycle |
| **AgentTask CRD** | Custom Resource Definition representing a unit of agent work |
| **Agent Pods** | Short-lived pods executing agent tasks with LLM capabilities |
| **CLI** | Command-line interface for spawning and managing tasks |

### Flow

1. User creates an `AgentTask` resource (via CLI or kubectl)
2. Operator detects the new resource and creates an agent pod
3. Agent pod executes the task using the configured LLM
4. Operator monitors pod status and updates `AgentTask` status
5. On completion, output is captured and pod is cleaned up

## Quick Start

### Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured
- Helm 3.x (for Helm installation)

### Installation

#### Using Helm

```bash
helm repo add hortator https://michael-niemand.github.io/Hortator/charts
helm install hortator hortator/hortator -n hortator-system --create-namespace
```

#### Using kubectl

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Install operator
kubectl apply -k config/default/
```

### Install CLI

```bash
go install github.com/michael-niemand/Hortator/cmd/hortator@latest
```

## Usage

### Spawn a Task

```bash
# Simple task
hortator spawn --prompt "Write a Python script that prints the Fibonacci sequence"

# With capabilities and timeout
hortator spawn \
  --prompt "Deploy the application to staging" \
  --capabilities exec,kubernetes \
  --timeout 1h \
  --image myregistry/custom-agent:v1

# Wait for completion
hortator spawn --prompt "Quick analysis task" --wait
```

### Check Status

```bash
# Status of specific task
hortator status my-task

# List all tasks
hortator list

# List tasks in all namespaces
hortator list -A
```

### View Logs

```bash
# View logs
hortator logs my-task

# Follow logs
hortator logs my-task -f
```

### Get Results

```bash
# Get output
hortator result my-task

# Output as JSON
hortator result my-task --json
```

### Delete Tasks

```bash
# Delete a task
hortator delete my-task

# Delete all tasks
hortator delete --all
```

### Using kubectl

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: hello-world
spec:
  prompt: "Write a haiku about Kubernetes"
  timeout: 10m
  capabilities:
    - read
    - write
  image: ghcr.io/hortator-ai/agent:latest
  model: gpt-4
  env:
    DEBUG: "true"
  resources:
    cpu: "500m"
    memory: "256Mi"
```

```bash
kubectl apply -f task.yaml
kubectl get agenttasks
kubectl describe agenttask hello-world
```

## AgentTask Reference

### Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `prompt` | string | Yes | - | Task instruction for the agent |
| `capabilities` | []string | No | [] | Permissions/tools available to the agent |
| `timeout` | string | No | "30m" | Maximum duration (e.g., "30m", "1h") |
| `image` | string | No | ghcr.io/hortator-ai/agent:latest | Agent container image |
| `model` | string | No | - | LLM model to use |
| `env` | map[string]string | No | {} | Environment variables |
| `resources.cpu` | string | No | - | CPU request |
| `resources.memory` | string | No | - | Memory request |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase: Pending, Running, Succeeded, Failed |
| `output` | string | Result/output from the agent |
| `podName` | string | Name of the executing pod |
| `startTime` | timestamp | When execution started |
| `completionTime` | timestamp | When execution finished |
| `message` | string | Human-readable status |
| `conditions` | []Condition | Detailed condition status |

## Development

### Prerequisites

- Go 1.22+
- Docker
- kubectl
- kubebuilder

### Build

```bash
# Build operator
make build

# Build CLI
go build -o bin/hortator ./cmd/hortator

# Build container image
make docker-build IMG=hortator:dev
```

### Test

```bash
# Run unit tests
make test

# Run e2e tests (requires cluster)
make test-e2e
```

### Deploy for Development

```bash
# Install CRDs
make install

# Run operator locally
make run

# In another terminal, create a task
hortator spawn --prompt "Test task"
```

### Generate

```bash
# Generate code (types, clients)
make generate

# Generate manifests (CRDs, RBAC)
make manifests
```

## Configuration

### Operator Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--leader-elect` | true | Enable leader election |
| `--namespace` | "" | Watch specific namespace (empty = all) |
| `--zap-log-level` | info | Log level (debug, info, error) |
| `--metrics-bind-address` | :8080 | Metrics server address |
| `--health-probe-bind-address` | :8081 | Health probe address |

### Helm Values

See [charts/hortator/values.yaml](charts/hortator/values.yaml) for all available configuration options.

Key settings:

```yaml
operator:
  replicas: 1
  image:
    repository: ghcr.io/hortator-ai/operator
    tag: latest

agent:
  image: ghcr.io/hortator-ai/agent:latest
  timeout: 30m

metrics:
  enabled: true
  serviceMonitor:
    enabled: false
```

## Roadmap

- [ ] Task queuing and priority
- [ ] Webhook notifications
- [ ] Multi-step task pipelines
- [ ] Agent memory/context persistence
- [ ] Cost tracking and limits
- [ ] Web dashboard
- [ ] Distributed task execution

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
