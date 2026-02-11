#!/usr/bin/env bash
#
# Hortator Local Quickstart
#
# Spins up a local Kind cluster, installs Hortator via Helm, and runs a demo task.
# Requires: kind, kubectl, helm, docker
#
# Usage:
#   ./hack/quickstart.sh              # Full setup + demo
#   ./hack/quickstart.sh --teardown   # Remove the cluster
#
set -euo pipefail

CLUSTER_NAME="${HORTATOR_CLUSTER:-hortator-dev}"
NAMESPACE="hortator-system"
OPERATOR_IMAGE="hortator-operator:dev"
RUNTIME_IMAGE="hortator-runtime:dev"
AGENTIC_IMAGE="hortator-agent-agentic:dev"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()   { echo -e "${GREEN}[hortator]${NC} $*"; }
warn()  { echo -e "${YELLOW}[hortator]${NC} $*"; }
err()   { echo -e "${RED}[hortator]${NC} $*" >&2; }
header() { echo -e "\n${CYAN}━━━ $* ━━━${NC}\n"; }

# --- Teardown ---
if [[ "${1:-}" == "--teardown" ]]; then
    header "Tearing down"
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    log "Cluster '$CLUSTER_NAME' deleted."
    exit 0
fi

# --- Preflight checks ---
header "Preflight checks"
for cmd in kind kubectl helm docker; do
    if ! command -v "$cmd" &>/dev/null; then
        err "Required command '$cmd' not found. Please install it first."
        exit 1
    fi
    log "✓ $cmd"
done

# --- Create cluster ---
header "Creating Kind cluster: $CLUSTER_NAME"
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    warn "Cluster '$CLUSTER_NAME' already exists, reusing."
else
    kind create cluster --name "$CLUSTER_NAME" --wait 60s
    log "Cluster created."
fi

kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# --- Build images ---
header "Building Hortator images"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

log "Building operator image..."
docker build -t "$OPERATOR_IMAGE" "$REPO_ROOT"

log "Building runtime image (legionary)..."
if [[ -f "$REPO_ROOT/runtime/Dockerfile" ]]; then
    docker build -t "$RUNTIME_IMAGE" -f "$REPO_ROOT/runtime/Dockerfile" "$REPO_ROOT"
else
    warn "No runtime Dockerfile found, skipping runtime image build."
fi

log "Building agentic runtime image (tribune/centurion)..."
if [[ -f "$REPO_ROOT/runtime/agentic/Dockerfile" ]]; then
    docker build -t "$AGENTIC_IMAGE" -f "$REPO_ROOT/runtime/agentic/Dockerfile" "$REPO_ROOT"
else
    warn "No agentic Dockerfile found, skipping agentic image build."
fi

log "Loading images into Kind..."
kind load docker-image "$OPERATOR_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$RUNTIME_IMAGE" --name "$CLUSTER_NAME" 2>/dev/null || true
kind load docker-image "$AGENTIC_IMAGE" --name "$CLUSTER_NAME" 2>/dev/null || true

# --- Install CRDs ---
header "Installing CRDs"
kubectl apply -f "$REPO_ROOT/crds/" 2>/dev/null || \
    kubectl apply -f "$REPO_ROOT/config/crd/bases/"

# --- Helm install ---
header "Installing Hortator via Helm"
kubectl create namespace "$NAMESPACE" 2>/dev/null || true

helm upgrade --install hortator "$REPO_ROOT/charts/hortator" \
    --namespace "$NAMESPACE" \
    --set operator.image.repository="${OPERATOR_IMAGE%%:*}" \
    --set operator.image.tag="${OPERATOR_IMAGE##*:}" \
    --set operator.image.pullPolicy=Never \
    --set agent.image="$RUNTIME_IMAGE" \
    --set agent.agenticImage="$AGENTIC_IMAGE" \
    --set presidio.enabled=false \
    --wait --timeout 120s

log "Hortator installed."

# --- Wait for operator ---
header "Waiting for operator to be ready"
kubectl rollout status deployment -l app.kubernetes.io/name=hortator \
    -n "$NAMESPACE" --timeout=60s
log "Operator is running."

# --- Run demo task ---
header "Running demo task"
cat <<'EOF' | kubectl apply -n "$NAMESPACE" -f -
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: quickstart-demo
spec:
  prompt: "Hello! Say 'Hortator is running successfully' and list 3 fun facts about Roman galleys."
  tier: legionary
  timeout: 120
EOF

log "Demo task 'quickstart-demo' created."
log "Waiting for task to complete..."

for i in $(seq 1 60); do
    phase=$(kubectl get agenttask quickstart-demo -n "$NAMESPACE" \
        -o jsonpath='{.status.phase}' 2>/dev/null || echo "Pending")
    case "$phase" in
        Completed|Failed|BudgetExceeded|TimedOut)
            break
            ;;
    esac
    sleep 2
done

echo ""
header "Demo task result"
kubectl get agenttask quickstart-demo -n "$NAMESPACE" -o yaml | \
    kubectl neat 2>/dev/null || \
    kubectl get agenttask quickstart-demo -n "$NAMESPACE" -o yaml

echo ""
header "Quick Reference"
log "List tasks:     kubectl get agenttasks -n $NAMESPACE"
log "Watch TUI:      hortator watch -n $NAMESPACE"
log "View task:      kubectl describe agenttask quickstart-demo -n $NAMESPACE"
log "Teardown:       $0 --teardown"
echo ""
log "Hortator is ready! Remigate, vermēs!"
