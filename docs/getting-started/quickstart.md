# Quickstart

Get Hortator running in your cluster in under 5 minutes.

## Prerequisites

- Kubernetes 1.28+ cluster (RKE2, K3s, EKS, GKE, etc.)
- Helm 3.x
- An LLM API key (OpenAI, Anthropic, or local Ollama)
- **Default StorageClass** — required for tribune/centurion tiers. RKE2/K3s don't ship one by default; install [local-path-provisioner](https://github.com/rancher/local-path-provisioner):
  ```bash
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```

## Install

```bash
# Install the operator
helm install hortator oci://ghcr.io/michael-niemand/hortator/charts/hortator \
  --namespace hortator-system --create-namespace \
  --set models.default.endpoint=https://api.anthropic.com/v1 \
  --set models.default.name=claude-sonnet-4-20250514

# Create a secret with your API key
kubectl create namespace hortator-demo
kubectl create secret generic anthropic-api-key \
  --namespace hortator-demo \
  --from-literal=api-key=sk-ant-...
```

!!! note "Using OpenAI instead?"
    ```bash
    helm install hortator oci://ghcr.io/michael-niemand/hortator/charts/hortator \
      --namespace hortator-system --create-namespace \
      --set models.default.endpoint=https://api.openai.com/v1 \
      --set models.default.name=gpt-4o
    ```

## Verify Installation

```bash
# Check the operator is running
kubectl get pods -n hortator-system

# Check CRDs are installed
kubectl get crd | grep hortator

# Check example roles
kubectl get clusteragentroles
```

## Run Your First Task

```bash
kubectl apply -f examples/quickstart/hello-world.yaml
```

## Watch Your First Agent

```bash
# Watch the task progress
kubectl get agenttasks -n hortator-demo -w

# Check the agent's logs
kubectl logs -n hortator-demo -l hortator.ai/task=hello-world -c agent

# Get the result
kubectl get agenttask hello-world -n hortator-demo -o jsonpath='{.status.output}'
```

## Create Your Own Task

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: my-first-task
  namespace: hortator-demo
spec:
  prompt: "Write a haiku about Kubernetes"
  tier: legionary
  timeout: 120
```

```bash
kubectl apply -f my-task.yaml
kubectl get agenttask my-first-task -n hortator-demo -w
```

## Next Steps

- [Understand the concepts](concepts.md) — tiers, roles, storage model
- [Choose your strategies](../guides/choosing-strategies.md) — context, routing, budget
- [Configuration reference](../configuration/helm-values.md) — all Helm values explained

## Cleanup

```bash
# Remove examples
kubectl delete namespace hortator-demo

# Uninstall Hortator
helm uninstall hortator -n hortator-system
kubectl delete namespace hortator-system
```
