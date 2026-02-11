#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# Hortator E2E Multi-Tier Test
# ============================================================================
# Tests the full lifecycle: tribune + 3 legionaries with parent-child hierarchy.
# Run on the sandbox: ssh root@89.167.62.139 "bash /root/Hortator/examples/e2e/multi-tier-test.sh"
#
# Prerequisites:
#   - Hortator operator + gateway running in hortator-system
#   - anthropic-api-key Secret exists
#   - AgentRole "tech-lead" and "researcher" exist
#   - hortator CLI built and in PATH
# ============================================================================

NS="hortator-system"
PREFIX="e2e-$(date +%s)"
TRIBUNE="${PREFIX}-tribune"
LEG1="${PREFIX}-leg-alpha"
LEG2="${PREFIX}-leg-beta"
LEG3="${PREFIX}-leg-gamma"
TIMEOUT=120

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; FAILURES=$((FAILURES+1)); }
info() { echo -e "${YELLOW}► $1${NC}"; }

FAILURES=0

echo ""
echo "═══════════════════════════════════════════════════"
echo "  HORTATOR E2E — Multi-Tier Task Test"
echo "  Namespace: ${NS}"
echo "  Prefix: ${PREFIX}"
echo "═══════════════════════════════════════════════════"
echo ""

# --- Step 1: Create tasks ---
info "Creating tribune + 3 legionaries..."

cat <<EOF | kubectl apply -f -
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: ${TRIBUNE}
  namespace: ${NS}
spec:
  prompt: "Summarize the security findings from your three sub-agents into a prioritized report."
  role: tech-lead
  tier: tribune
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: ${LEG1}
  namespace: ${NS}
spec:
  prompt: "List 3 common authentication vulnerabilities in Go HTTP APIs. Be concise."
  role: researcher
  tier: legionary
  parentTaskId: ${TRIBUNE}
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: ${LEG2}
  namespace: ${NS}
spec:
  prompt: "List 3 common container security misconfigurations in Kubernetes. Be concise."
  role: researcher
  tier: legionary
  parentTaskId: ${TRIBUNE}
---
apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: ${LEG3}
  namespace: ${NS}
spec:
  prompt: "List 3 Helm chart security best practices. Be concise."
  role: researcher
  tier: legionary
  parentTaskId: ${TRIBUNE}
EOF

echo ""

# --- Step 2: Verify creation ---
info "Verifying tasks created..."

for TASK in "$TRIBUNE" "$LEG1" "$LEG2" "$LEG3"; do
  if kubectl get agenttask "$TASK" -n "$NS" &>/dev/null; then
    pass "Created: $TASK"
  else
    fail "Missing: $TASK"
  fi
done

# --- Step 3: Verify parent-child hierarchy ---
info "Checking parent-child relationships..."

for LEG in "$LEG1" "$LEG2" "$LEG3"; do
  PARENT=$(kubectl get agenttask "$LEG" -n "$NS" -o jsonpath='{.spec.parentTaskId}')
  if [[ "$PARENT" == "$TRIBUNE" ]]; then
    pass "Parent correct: $LEG → $TRIBUNE"
  else
    fail "Wrong parent for $LEG: got '$PARENT', want '$TRIBUNE'"
  fi
done

# --- Step 4: Wait for completion ---
info "Waiting for all tasks to complete (timeout ${TIMEOUT}s)..."

ELAPSED=0
while [[ $ELAPSED -lt $TIMEOUT ]]; do
  COMPLETED=$(kubectl get agenttasks -n "$NS" -l "!hortator.ai/source" \
    --field-selector="metadata.name!=${TRIBUNE}" \
    -o jsonpath='{range .items[?(@.status.phase=="Completed")]}{.metadata.name}{"\n"}{end}' 2>/dev/null | \
    grep -c "^${PREFIX}" || true)
  
  TRIBUNE_PHASE=$(kubectl get agenttask "$TRIBUNE" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
  
  ALL_DONE=true
  for TASK in "$TRIBUNE" "$LEG1" "$LEG2" "$LEG3"; do
    PHASE=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
    if [[ "$PHASE" != "Completed" && "$PHASE" != "Failed" ]]; then
      ALL_DONE=false
    fi
  done
  
  if $ALL_DONE; then
    break
  fi
  
  echo -ne "\r  Waiting... ${ELAPSED}s (tribune: ${TRIBUNE_PHASE}, legionaries completed: ${COMPLETED}/3)"
  sleep 5
  ELAPSED=$((ELAPSED+5))
done
echo ""

# --- Step 5: Verify completion ---
info "Checking final status..."

for TASK in "$TRIBUNE" "$LEG1" "$LEG2" "$LEG3"; do
  PHASE=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.phase}')
  DURATION=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.duration}')
  if [[ "$PHASE" == "Completed" ]]; then
    pass "Completed: $TASK (${DURATION})"
  else
    fail "Not completed: $TASK (phase: $PHASE)"
  fi
done

# --- Step 6: Verify output exists ---
info "Checking task output..."

for TASK in "$TRIBUNE" "$LEG1" "$LEG2" "$LEG3"; do
  OUTPUT=$(kubectl get agenttask "$TASK" -n "$NS" -o jsonpath='{.status.output}')
  if [[ -n "$OUTPUT" && ${#OUTPUT} -gt 20 ]]; then
    pass "Output exists: $TASK (${#OUTPUT} chars)"
  else
    fail "No/short output: $TASK (${#OUTPUT} chars)"
  fi
done

# --- Step 7: Verify tree display ---
info "Testing hortator tree command..."

if command -v hortator &>/dev/null; then
  TREE_OUT=$(hortator tree "$TRIBUNE" -n "$NS" 2>&1 || true)
  if echo "$TREE_OUT" | grep -q "$LEG1"; then
    pass "Tree shows legionaries"
  else
    fail "Tree missing legionaries"
  fi
  echo ""
  echo "$TREE_OUT"
else
  info "Skipping tree test (hortator CLI not in PATH)"
fi

# --- Step 8: Test gateway API ---
info "Testing gateway API..."

GW_IP=$(kubectl get svc hortator-gateway -n "$NS" -o jsonpath='{.spec.clusterIP}')
GW_PORT=$(kubectl get svc hortator-gateway -n "$NS" -o jsonpath='{.spec.ports[0].port}')
API_KEY=$(kubectl get secret hortator-gateway-auth -n "$NS" -o jsonpath='{.data}' | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(list(d.values())[0]).decode())" 2>/dev/null || echo "")

if [[ -n "$API_KEY" ]]; then
  GW_TASK="${PREFIX}-gw-test"
  GW_RESP=$(curl -sS "http://${GW_IP}:${GW_PORT}/v1/chat/completions" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"hortator/researcher\",\"messages\":[{\"role\":\"user\",\"content\":\"Say 'hello from gateway' in exactly those words.\"}]}" \
    --max-time 60 2>&1 || true)
  
  if echo "$GW_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['choices'][0]['message']['content']" 2>/dev/null; then
    pass "Gateway API returned valid response"
  else
    fail "Gateway API failed: ${GW_RESP:0:200}"
  fi
else
  info "Skipping gateway test (no API key found)"
fi

# --- Step 9: Cleanup ---
echo ""
info "Cleaning up test tasks..."

for TASK in "$TRIBUNE" "$LEG1" "$LEG2" "$LEG3"; do
  kubectl delete agenttask "$TASK" -n "$NS" --ignore-not-found &>/dev/null
done
# Gateway task cleanup (name is auto-generated, find by label)
kubectl delete agenttasks -n "$NS" -l "hortator.ai/source=gateway" --field-selector="metadata.name=${GW_TASK}" --ignore-not-found &>/dev/null 2>&1 || true

pass "Cleanup done"

# --- Summary ---
echo ""
echo "═══════════════════════════════════════════════════"
if [[ $FAILURES -eq 0 ]]; then
  echo -e "  ${GREEN}ALL TESTS PASSED ✓${NC}"
else
  echo -e "  ${RED}${FAILURES} TEST(S) FAILED ✗${NC}"
fi
echo "═══════════════════════════════════════════════════"
echo ""

exit $FAILURES
