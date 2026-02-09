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
presidio_scan() {
    local text="$1"
    if [ -n "${PRESIDIO_ENDPOINT:-}" ]; then
        local result
        result=$(curl -s --max-time 5 "$PRESIDIO_ENDPOINT/analyze" \
            -H "Content-Type: application/json" \
            -d "$(jq -n --arg t "$text" '{text:$t, language:"en"}')" 2>/dev/null) || {
            echo "[hortator-runtime] WARN: Presidio not reachable, skipping PII scan"
            return 0
        }
        # Log any PII found
        echo "$result" | jq -r '.[] | "PII detected: \(.entity_type) score=\(.score)"' 2>/dev/null || true
    fi
}

# --- Read task.json ---
[[ -f "$TASK_FILE" ]] || die "task.json not found at ${TASK_FILE}"

TASK_ID=$(jq -r '.taskId // "unknown"' "$TASK_FILE")
PROMPT=$(jq -r '.prompt // ""' "$TASK_FILE")
ROLE=$(jq -r '.role // "worker"' "$TASK_FILE")
FLAVOR=$(jq -r '.flavor // "default"' "$TASK_FILE")
TIER=$(jq -r '.tier // "fast"' "$TASK_FILE")
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

# Pick model based on tier
MODEL="${HORTATOR_MODEL:-}"
if [[ -z "$MODEL" ]]; then
  case "$TIER" in
    fast)   MODEL="gpt-4o-mini" ;;
    think)  MODEL="gpt-4o" ;;
    deep)   MODEL="gpt-4o" ;;
    *)      MODEL="gpt-4o-mini" ;;
  esac
fi

[[ -z "$PROMPT" ]] && die "Empty prompt in task.json"

echo "[hortator-runtime] Task=${TASK_ID} Role=${ROLE} Tier=${TIER} Model=${MODEL}"

# Scan prompt for PII before sending to LLM
presidio_scan "$PROMPT"

# --- Build system message ---
SYSTEM="You are an AI agent working as a ${ROLE}. Complete the task given to you."
[[ -n "$PRIOR_WORK" ]] && SYSTEM="${SYSTEM} Prior work from sub-agents: ${PRIOR_WORK}"

# --- Call LLM ---
if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "[hortator-runtime] Using Anthropic API..."
  # Map model name for Anthropic
  ANTHROPIC_MODEL="${MODEL}"
  case "$TIER" in
    fast)  ANTHROPIC_MODEL="claude-sonnet-4-20250514" ;;
    think) ANTHROPIC_MODEL="claude-sonnet-4-20250514" ;;
    deep)  ANTHROPIC_MODEL="claude-opus-4-20250514" ;;
  esac

  RESPONSE=$(curl -sS https://api.anthropic.com/v1/messages \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d "$(jq -n \
      --arg model "$ANTHROPIC_MODEL" \
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

# Scan response for PII before writing results
presidio_scan "$SUMMARY"

# --- Write results ---
write_result "completed" "$SUMMARY" "$INPUT_TOKENS" "$OUTPUT_TOKENS"

echo "[hortator-runtime] Done. Tokens: in=${INPUT_TOKENS} out=${OUTPUT_TOKENS}"
exit 0
