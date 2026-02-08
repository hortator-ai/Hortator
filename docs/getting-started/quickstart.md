# Quickstart

Get Hortator running in your cluster in under 5 minutes.

## Prerequisites

- Kubernetes 1.24+ cluster
- Helm 3.x
- An LLM API key (OpenAI, Anthropic, or local Ollama)

## Install

```bash
# Add the Hortator Helm repo
helm repo add hortator https://charts.hortator.io
helm repo update

# Install with examples enabled
helm install hortator hortator/hortator \
  --namespace hortator-system \
  --create-namespace \
  --set models.default.endpoint=https://api.anthropic.com/v1 \
  --set models.default.name=claude-sonnet \
  --set models.default.apiKeyRef.secretName=llm-credentials \
  --set models.default.apiKeyRef.key=api-key \
  --set examples.enabled=true
```

!!! note "Create your API key secret first"
    ```bash
    kubectl create namespace hortator-system
    kubectl create secret generic llm-credentials \
      --namespace hortator-system \
      --from-literal=api-key=sk-your-key-here
    ```

## Verify Installation

```bash
# Check the operator is running
kubectl get pods -n hortator-system

# Check CRDs are installed
kubectl get crd | grep hortator

# Check example roles
kubectl get clusteragentroles

# Check the hello-world task
kubectl get agenttasks -n hortator-demo
```

## Watch Your First Agent

```bash
# Watch the task progress
kubectl get agenttasks -n hortator-demo -w

# Check the agent's logs
kubectl logs -n hortator-demo -l hortator.io/task=hello-world -f

# Get the result
kubectl get agenttask hello-world -n hortator-demo -o jsonpath='{.status.output}'
```

## Create Your Own Task

```yaml
apiVersion: hortator.io/v1alpha1
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
