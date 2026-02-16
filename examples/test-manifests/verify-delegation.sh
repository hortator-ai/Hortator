#!/usr/bin/env bash
# Verify delegation scenario results.
# Usage: ./verify-delegation.sh [namespace]
#
# Checks that completed tasks produced the expected deliverables.

set -euo pipefail
NS="${1:-hortator-test}"
PASS=0
FAIL=0

check_artifact() {
  local task="$1" file="$2" desc="$3" min_bytes="${4:-10}"
  local pod="${task}-agent"

  # Check if task exists and is completed
  local phase
  phase=$(kubectl get agenttask "$task" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")

  if [[ "$phase" == "NotFound" ]]; then
    echo "  ⏭  $task — not found (skipping)"
    return
  fi
  if [[ "$phase" != "Completed" ]]; then
    echo "  ⏳ $task — still $phase"
    return
  fi

  # Check deliverable exists and has content
  local size
  size=$(kubectl exec "$pod" -n "$NS" -- wc -c < "/outbox/artifacts/$file" 2>/dev/null || echo "0")

  if [[ "$size" -ge "$min_bytes" ]]; then
    echo "  ✅ $task — $desc ($file, ${size} bytes)"
    ((PASS++))
  else
    echo "  ❌ $task — $desc MISSING or empty ($file)"
    ((FAIL++))
  fi
}

check_children() {
  local task="$1" expected="$2" desc="$3"

  local phase
  phase=$(kubectl get agenttask "$task" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
  if [[ "$phase" == "NotFound" ]]; then
    echo "  ⏭  $task — not found (skipping)"
    return
  fi

  # Count child tasks (tasks whose name starts with "task-" spawned during this task's lifetime)
  # We check the inject pods as a proxy for child results
  local children
  children=$(kubectl get pods -n "$NS" -l "hortator.ai/task=$task,hortator.ai/inject=child-result" --no-headers 2>/dev/null | wc -l | tr -d ' ')

  if [[ "$expected" == "0" ]]; then
    if [[ "$children" == "0" ]]; then
      echo "  ✅ $task — $desc (0 children, as expected)"
      ((PASS++))
    else
      echo "  ❌ $task — $desc (expected 0 children, got $children)"
      ((FAIL++))
    fi
  else
    if [[ "$children" -ge 1 ]]; then
      echo "  ✅ $task — $desc ($children children spawned)"
      ((PASS++))
    else
      echo "  ❌ $task — $desc (expected children, got 0)"
      ((FAIL++))
    fi
  fi
}

echo "=== Delegation Scenario Verification ==="
echo "Namespace: $NS"
echo ""

echo "--- Scenario A: Complex coding task ---"
check_children "delegation-code-complex" "2+" "should delegate"
check_artifact "delegation-code-complex" "server.js" "Backend API" 100
check_artifact "delegation-code-complex" "index.html" "Frontend" 100
echo ""

echo "--- Scenario B: Simple coding task ---"
check_children "delegation-code-simple" "0" "should NOT delegate"
check_artifact "delegation-code-simple" "csv2json.py" "CSV to JSON script" 100
echo ""

echo "--- Scenario C: Complex research task ---"
check_children "delegation-research-complex" "2+" "should delegate"
check_artifact "delegation-research-complex" "carbon-capture-briefing.md" "Carbon capture briefing" 500
echo ""

echo "--- Scenario D: Simple research task ---"
check_children "delegation-research-simple" "0" "should NOT delegate"
check_artifact "delegation-research-simple" "eu-ai-act-summary.md" "EU AI Act summary" 200
echo ""

echo "--- Scenario E: Multi-iteration ---"
check_artifact "delegation-multi-iter" "orchestration-comparison.md" "Orchestration comparison report" 500
echo ""

echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]] && exit 0 || exit 1
