#!/usr/bin/env bash
set -euo pipefail

# --- Graceful shutdown ---
KILLED=0
cleanup() {
  KILLED=1
  echo "[hortator-runtime] SIGTERM received, shutting down..."
  # Kill child if running
  [[ -n "${CHILD_PID:-}" ]] && kill "$CHILD_PID" 2>/dev/null || true
}
trap cleanup SIGTERM SIGINT

# --- Constants ---
INBOX="/inbox"
OUTBOX="/outbox"
TASK_FILE="${INBOX}/task.json"
RESULT_FILE="${OUTBOX}/result.json"
USAGE_FILE="${OUTBOX}/usage.json"
START_TIME=$(date +%s)

# --- Helpers ---
die() {
  local msg="$1"
  echo "[hortator-runtime] ERROR: ${msg}" >&2
  jq -n \
    --arg tid "${TASK_ID:-unknown}" \
    --arg status "failed" \
    --arg summary "$msg" \
    '{taskId:$tid, status:$status, summary:$summary, artifacts:[], decisions:0, tokensUsed:{input:0,output:0}, duration:0}' \
    > "$RESULT_FILE"
  jq -n '{input:0, output:0, total:0}' > "$USAGE_FILE"
  exit 1
}

write_result() {
  local status="$1" summary="$2" input_tokens="${3:-0}" output_tokens="${4:-0}"
  local duration=$(( $(date +%s) - START_TIME ))
  jq -n \
    --arg tid "$TASK_ID" \
    --arg status "$status" \
    --arg summary "$summary" \
    --argjson inp "$input_tokens" \
    --argjson out "$output_tokens" \
    --argjson dur "$duration" \
    '{taskId:$tid, status:$status, summary:$summary, artifacts:[], decisions:1, tokensUsed:{input:$inp,output:$out}, duration:$dur}' \
    > "$RESULT_FILE"
  jq -n --argjson i "$input_tokens" --argjson o "$output_tokens" \
    '{input:$i, output:$o, total:($i+$o)}' > "$USAGE_FILE"
}

# --- Presidio PII scanning ---
PRESIDIO_READY=0

wait_for_presidio() {
    if [ -z "${PRESIDIO_ENDPOINT:-}" ]; then
        return 0
    fi
    echo "[hortator-runtime] Waiting for Presidio at ${PRESIDIO_ENDPOINT}..."
    local max_attempts="${PRESIDIO_WAIT_SECONDS:-60}"
    local attempt=0
    while [ $attempt -lt "$max_attempts" ]; do
        if curl -sf --max-time 2 "${PRESIDIO_ENDPOINT}/health" >/dev/null 2>&1; then
            echo "[hortator-runtime] Presidio ready after ${attempt}s"
            PRESIDIO_READY=1
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 1
    done
    echo "[hortator-runtime] WARN: Presidio not reachable after ${max_attempts}s, PII scanning disabled"
    return 0
}

presidio_scan() {
    local text="$1"
    if [ -n "${PRESIDIO_ENDPOINT:-}" ] && [ "$PRESIDIO_READY" -eq 1 ]; then
        local result
        result=$(curl -s --max-time 5 "$PRESIDIO_ENDPOINT/analyze" \
            -H "Content-Type: application/json" \
            -d "$(jq -n --arg t "$text" '{text:$t, language:"en"}')" 2>/dev/null) || {
            echo "[hortator-runtime] WARN: Presidio scan failed, skipping"
            return 0
        }
        # Log any PII found
        echo "$result" | jq -r '.[] | "PII detected: \(.entity_type) score=\(.score)"' 2>/dev/null || true
    fi
}

presidio_redact() {
    local text="$1"
    if [ -z "${PRESIDIO_ENDPOINT:-}" ] || [ "$PRESIDIO_READY" -ne 1 ]; then
        echo "$text"
        return 0
    fi
    # Step 1: Analyze
    local analyze_result
    analyze_result=$(curl -s --max-time 5 "$PRESIDIO_ENDPOINT/analyze" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg t "$text" '{text:$t, language:"en"}')" 2>/dev/null) || {
        echo "[hortator-runtime] WARN: Presidio analyze failed, returning original text" >&2
        echo "$text"
        return 0
    }
    # If no PII found, return original
    local count
    count=$(echo "$analyze_result" | jq 'length' 2>/dev/null) || count=0
    if [ "$count" -eq 0 ]; then
        echo "$text"
        return 0
    fi
    echo "[hortator-runtime] Redacting ${count} PII entities..." >&2
    # Step 2: Anonymize
    local anon_result
    anon_result=$(curl -s --max-time 5 "$PRESIDIO_ENDPOINT/anonymize" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg t "$text" --argjson ar "$analyze_result" \
            '{text:$t, analyzer_results:$ar, anonymizers:{DEFAULT:{type:"replace"}}}')" 2>/dev/null) || {
        echo "[hortator-runtime] WARN: Presidio anonymize failed, returning original text" >&2
        echo "$text"
        return 0
    }
    local redacted
    redacted=$(echo "$anon_result" | jq -r '.text // empty' 2>/dev/null)
    if [ -n "$redacted" ]; then
        echo "$redacted"
    else
        echo "$text"
    fi
}

# --- Read task.json ---
[[ -f "$TASK_FILE" ]] || die "task.json not found at ${TASK_FILE}"

TASK_ID=$(jq -r '.taskId // "unknown"' "$TASK_FILE")
# Fall back to HORTATOR_TASK_NAME env var if taskId is missing from task.json
[[ "$TASK_ID" == "unknown" ]] && TASK_ID="${HORTATOR_TASK_NAME:-unknown}"
PROMPT=$(jq -r '.prompt // ""' "$TASK_FILE")
ROLE=$(jq -r '.role // "worker"' "$TASK_FILE")
FLAVOR=$(jq -r '.flavor // "default"' "$TASK_FILE")
TIER=$(jq -r '.tier // "legionary"' "$TASK_FILE")
BUDGET=$(jq -r '.budget // 0' "$TASK_FILE")
PRIOR_WORK=$(jq -r '.prior_work // ""' "$TASK_FILE")

# Export for child processes
export HORTATOR_TASK_ID="$TASK_ID"
export HORTATOR_PROMPT="$PROMPT"
export HORTATOR_ROLE="$ROLE"
export HORTATOR_FLAVOR="$FLAVOR"
export HORTATOR_TIER="$TIER"
export HORTATOR_BUDGET="$BUDGET"
export HORTATOR_TASK_NAME="${HORTATOR_TASK_NAME:-$TASK_ID}"

# Model is injected by the operator from AgentRole config (HORTATOR_MODEL env var).
# Only fall back to a generic default if unset (e.g. local dev without operator).
MODEL="${HORTATOR_MODEL:-claude-sonnet-4-20250514}"

[[ -z "$PROMPT" ]] && die "Empty prompt in task.json"

echo "[hortator-runtime] Task=${TASK_ID} Role=${ROLE} Tier=${TIER} Model=${MODEL}"

# Wait for Presidio to become ready before first scan
wait_for_presidio

# --- PII redaction on input (before LLM submission) ---
# Controlled by HORTATOR_REDACT_INPUT (default: true when Presidio is enabled).
# When enabled, prompts and system messages are redacted via Presidio before
# being sent to third-party LLM APIs, preventing PII exfiltration.
REDACT_INPUT="${HORTATOR_REDACT_INPUT:-true}"

if [ "$PRESIDIO_READY" -eq 1 ] && [ "$REDACT_INPUT" = "true" ]; then
  # Scan prompt for PII (logging only)
  presidio_scan "$PROMPT"
  # Redact PII from prompt before sending to LLM
  PROMPT=$(presidio_redact "$PROMPT")
else
  # Still scan for visibility even if redaction is disabled
  presidio_scan "$PROMPT"
fi

# --- Build system message ---
SYSTEM="You are an AI agent working as a ${ROLE}. Complete the task given to you."
[[ -n "$PRIOR_WORK" ]] && SYSTEM="${SYSTEM} Prior work from sub-agents: ${PRIOR_WORK}"

# Redact system message if it contains prior work (which may have PII from child results)
if [ "$PRESIDIO_READY" -eq 1 ] && [ "$REDACT_INPUT" = "true" ] && [ -n "$PRIOR_WORK" ]; then
  SYSTEM=$(presidio_redact "$SYSTEM")
fi

# --- Call LLM ---
if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "[hortator-runtime] Using Anthropic API..."
  RESPONSE=$(curl -sS https://api.anthropic.com/v1/messages \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d "$(jq -n \
      --arg model "$MODEL" \
      --arg system "$SYSTEM" \
      --arg prompt "$PROMPT" \
      '{model:$model, max_tokens:4096, system:$system, messages:[{role:"user",content:$prompt}]}')" \
  ) || true

  [[ $KILLED -eq 1 ]] && die "Killed by SIGTERM"

  # Parse response
  ERROR=$(echo "$RESPONSE" | jq -r '.error.message // empty')
  [[ -n "$ERROR" ]] && die "Anthropic API error: ${ERROR}"

  SUMMARY=$(echo "$RESPONSE" | jq -r '.content[0].text // "No response"')
  INPUT_TOKENS=$(echo "$RESPONSE" | jq -r '.usage.input_tokens // 0')
  OUTPUT_TOKENS=$(echo "$RESPONSE" | jq -r '.usage.output_tokens // 0')

elif [[ -n "${OPENAI_API_KEY:-}" ]]; then
  echo "[hortator-runtime] Using OpenAI API..."

  RESPONSE=$(curl -sS https://api.openai.com/v1/chat/completions \
    -H "Authorization: Bearer ${OPENAI_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$(jq -n \
      --arg model "$MODEL" \
      --arg system "$SYSTEM" \
      --arg prompt "$PROMPT" \
      '{model:$model, messages:[{role:"system",content:$system},{role:"user",content:$prompt}]}')" \
  ) || true

  [[ $KILLED -eq 1 ]] && die "Killed by SIGTERM"

  ERROR=$(echo "$RESPONSE" | jq -r '.error.message // empty')
  [[ -n "$ERROR" ]] && die "OpenAI API error: ${ERROR}"

  SUMMARY=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // "No response"')
  INPUT_TOKENS=$(echo "$RESPONSE" | jq -r '.usage.prompt_tokens // 0')
  OUTPUT_TOKENS=$(echo "$RESPONSE" | jq -r '.usage.completion_tokens // 0')

else
  # No API key — echo mode
  echo "[hortator-runtime] No API key set, running in echo mode"
  SUMMARY="[echo] Received prompt (${#PROMPT} chars): ${PROMPT:0:200}"
  INPUT_TOKENS=0
  OUTPUT_TOKENS=0
fi

# Redact PII from LLM response before reporting results
if [ "$PRESIDIO_READY" -eq 1 ]; then
  SUMMARY=$(presidio_redact "$SUMMARY")
fi

# --- Report results via CRD status ---
# Primary path: patch the AgentTask CRD directly via the K8s API.
# This is instant — the gateway and operator watch the CRD and pick it up.
# Artifacts (code files, patches) stay on the PVC at /outbox/artifacts/.
if hortator report --result "$SUMMARY" --tokens-in "$INPUT_TOKENS" --tokens-out "$OUTPUT_TOKENS" 2>/dev/null; then
  echo "[hortator-runtime] Result reported to CRD. Tokens: in=${INPUT_TOKENS} out=${OUTPUT_TOKENS}"
else
  # Fallback: write result markers to stdout for legacy operator log scraping
  echo "[hortator-runtime] WARN: hortator report failed, falling back to stdout"
  echo "[hortator-result-begin]"
  echo "$SUMMARY"
  echo "[hortator-result-end]"
  echo "[hortator-runtime] Done. Tokens: in=${INPUT_TOKENS} out=${OUTPUT_TOKENS}"
fi

# Also write result.json to PVC for artifact consumers
write_result "completed" "$SUMMARY" "$INPUT_TOKENS" "$OUTPUT_TOKENS"

exit 0
