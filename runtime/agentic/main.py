#!/usr/bin/env python3
"""
Hortator Agentic Runtime — Tool-calling loop for Tribune and Centurion tiers.

Reads /inbox/task.json, runs an LLM tool-calling loop, writes results to
/outbox/result.json. Supports checkpoint/restore for the reincarnation model.

Legionaries use the bash single-shot runtime (entrypoint.sh) instead.
"""

import json
import os
import signal
import sys
import time
import urllib.request
import urllib.error

from loop import agentic_loop
from checkpoint import load_checkpoint, save_checkpoint
from tools import build_tools
from prompt import build_system_prompt

# ── Constants ────────────────────────────────────────────────────────────────

INBOX = "/inbox"
OUTBOX = "/outbox"
MEMORY = "/memory"
TASK_FILE = os.path.join(INBOX, "task.json")
RESULT_FILE = os.path.join(OUTBOX, "result.json")
USAGE_FILE = os.path.join(OUTBOX, "usage.json")
STATE_FILE = os.path.join(MEMORY, "state.json")
CHILD_RESULTS_DIR = os.path.join(INBOX, "child-results")

# ── Graceful shutdown ────────────────────────────────────────────────────────

_killed = False


def _on_signal(signum, _frame):
    global _killed
    _killed = True
    print("[hortator-agentic] SIGTERM received, shutting down...")


signal.signal(signal.SIGTERM, _on_signal)
signal.signal(signal.SIGINT, _on_signal)


# ── Helpers ──────────────────────────────────────────────────────────────────

def die(task_id: str, msg: str):
    """Write a failed result and exit."""
    result = {
        "taskId": task_id,
        "status": "failed",
        "summary": msg,
        "artifacts": [],
        "decisions": 0,
        "tokensUsed": {"input": 0, "output": 0},
        "duration": 0,
    }
    os.makedirs(os.path.dirname(RESULT_FILE), exist_ok=True)
    with open(RESULT_FILE, "w") as f:
        json.dump(result, f)
    with open(USAGE_FILE, "w") as f:
        json.dump({"input": 0, "output": 0, "total": 0}, f)
    sys.exit(1)


def write_result(task_id: str, status: str, summary: str,
                 input_tokens: int, output_tokens: int, start_time: float,
                 artifacts: list[str] | None = None):
    """Write the final result.json and usage.json."""
    duration = int(time.time() - start_time)
    result = {
        "taskId": task_id,
        "status": status,
        "summary": summary,
        "artifacts": artifacts or [],
        "decisions": 0,
        "tokensUsed": {"input": input_tokens, "output": output_tokens},
        "duration": duration,
    }
    os.makedirs(os.path.dirname(RESULT_FILE), exist_ok=True)
    with open(RESULT_FILE, "w") as f:
        json.dump(result, f, indent=2)
    with open(USAGE_FILE, "w") as f:
        json.dump({"input": input_tokens, "output": output_tokens,
                    "total": input_tokens + output_tokens}, f)


def report_to_crd(summary: str, tokens_in: int, tokens_out: int) -> bool:
    """Report result to the AgentTask CRD via the hortator CLI."""
    import subprocess
    try:
        subprocess.run(
            ["hortator", "report",
             "--result", summary[:16000],
             "--tokens-in", str(tokens_in),
             "--tokens-out", str(tokens_out)],
            capture_output=True, timeout=30, check=True,
        )
        return True
    except Exception:
        return False


def wait_for_presidio() -> bool:
    """Wait for Presidio service to become ready (up to 60s, configurable)."""
    endpoint = os.environ.get("PRESIDIO_ENDPOINT", "")
    if not endpoint:
        return False
    max_wait = int(os.environ.get("PRESIDIO_WAIT_SECONDS", "60"))
    print(f"[hortator-agentic] Waiting for Presidio at {endpoint} (timeout: {max_wait}s)...")
    for attempt in range(max_wait):
        try:
            req = urllib.request.Request(f"{endpoint}/health", method="GET")
            with urllib.request.urlopen(req, timeout=2):
                print(f"[hortator-agentic] Presidio ready after {attempt}s")
                return True
        except (urllib.error.URLError, OSError):
            time.sleep(1)
    print(f"[hortator-agentic] WARN: Presidio not reachable after {max_wait}s, PII scanning disabled")
    return False


def load_child_results() -> dict[str, dict]:
    """Load child results from /inbox/child-results/."""
    results = {}
    if not os.path.isdir(CHILD_RESULTS_DIR):
        return results
    for fname in os.listdir(CHILD_RESULTS_DIR):
        if not fname.endswith(".json"):
            continue
        fpath = os.path.join(CHILD_RESULTS_DIR, fname)
        try:
            with open(fpath) as f:
                results[fname.removesuffix(".json")] = json.load(f)
        except Exception as e:
            print(f"[hortator-agentic] WARN: failed to read child result {fpath}: {e}")
    return results


# ── Main ─────────────────────────────────────────────────────────────────────

def main():
    start_time = time.time()

    # Read task.json
    if not os.path.isfile(TASK_FILE):
        die("unknown", f"task.json not found at {TASK_FILE}")

    with open(TASK_FILE) as f:
        task = json.load(f)

    task_id = task.get("taskId") or os.environ.get("HORTATOR_TASK_NAME") or task.get("prompt", "unknown")[:40]
    prompt = task.get("prompt", "")
    role = task.get("role", "worker")
    tier = task.get("tier", "centurion")
    capabilities = task.get("capabilities", [])
    budget = task.get("budget", {})

    if not prompt:
        die(task_id, "Empty prompt in task.json")

    # Environment overrides (injected by operator)
    task_name = os.environ.get("HORTATOR_TASK_NAME", task_id)
    task_ns = os.environ.get("HORTATOR_TASK_NAMESPACE", "default")
    model = os.environ.get("HORTATOR_MODEL", "claude-sonnet-4-20250514")

    # LLM endpoint — litellm auto-detects Anthropic/OpenAI from API key env vars.
    # Only set LITELLM_API_BASE for custom/self-hosted endpoints (Ollama, vLLM, etc.).
    # Setting it for known providers breaks litellm's routing (it would try the
    # OpenAI /chat/completions path against Anthropic's /v1/messages endpoint).
    llm_endpoint = task.get("model", {}).get("endpoint", "")
    if llm_endpoint:
        endpoint_lower = llm_endpoint.lower()
        is_known_provider = (
            "anthropic.com" in endpoint_lower
            or "openai.com" in endpoint_lower
        )
        if not is_known_provider:
            os.environ["LITELLM_API_BASE"] = llm_endpoint

    print(f"[hortator-agentic] Task={task_name} Role={role} Tier={tier} Model={model}")

    # Wait for Presidio to become ready (if configured)
    wait_for_presidio()

    # Check for checkpoint (reincarnation)
    checkpoint = load_checkpoint(STATE_FILE)
    child_results = load_child_results()

    if checkpoint:
        print(f"[hortator-agentic] Resuming from checkpoint: phase={checkpoint.get('phase', 'unknown')}")

    # Build tools based on capabilities
    tools = build_tools(capabilities, task_name, task_ns)

    # Build system prompt
    system_prompt = build_system_prompt(
        role=role,
        tier=tier,
        capabilities=capabilities,
        tool_names=[t["function"]["name"] for t in tools],
    )

    # Build initial messages
    messages = [{"role": "system", "content": system_prompt}]

    # If resuming from checkpoint, inject context
    if checkpoint:
        resume_context = _build_resume_context(checkpoint, child_results)
        messages.append({"role": "user", "content": resume_context})
    else:
        messages.append({"role": "user", "content": prompt})

    # Run the agentic loop
    result = agentic_loop(
        messages=messages,
        tools=tools,
        model=model,
        task_name=task_name,
        task_ns=task_ns,
        budget=budget,
        state_file=STATE_FILE,
        is_killed=lambda: _killed,
    )

    # Write results
    write_result(
        task_id=task_name,
        status=result.status,
        summary=result.output,
        input_tokens=result.tokens_in,
        output_tokens=result.tokens_out,
        start_time=start_time,
        artifacts=result.artifacts,
    )

    # Report to CRD
    if report_to_crd(result.output, result.tokens_in, result.tokens_out):
        print(f"[hortator-agentic] Result reported to CRD. "
              f"Tokens: in={result.tokens_in} out={result.tokens_out}")
    else:
        print("[hortator-agentic] WARN: hortator report failed, result on PVC only")

    # If the agent is waiting for children, save checkpoint and exit
    if result.status == "waiting":
        print(f"[hortator-agentic] Exiting with Waiting status, "
              f"pending children: {result.pending_children}")
        sys.exit(0)

    print(f"[hortator-agentic] Done. Status={result.status} "
          f"Tokens: in={result.tokens_in} out={result.tokens_out}")


def _build_resume_context(checkpoint: dict, child_results: dict[str, dict]) -> str:
    """Build the resume prompt for a reincarnated agent."""
    parts = ["You are resuming from a previous run. Here is your saved state:\n"]

    if checkpoint.get("plan"):
        plan = checkpoint["plan"]
        parts.append(f"## Plan\n"
                     f"Phases: {plan.get('phases', [])}\n"
                     f"Current phase: {plan.get('currentPhase', 0)}\n")

    if checkpoint.get("decisions"):
        parts.append("## Previous Decisions\n"
                     + "\n".join(f"- {d}" for d in checkpoint["decisions"]))

    if checkpoint.get("accumulatedContext"):
        parts.append(f"\n## Accumulated Context\n{checkpoint['accumulatedContext']}")

    # Inject new child results
    if child_results:
        parts.append("\n## New Child Results (since your last run)")
        for name, result in child_results.items():
            status = result.get("status", "unknown")
            summary = result.get("summary", result.get("output", "No output"))
            parts.append(f"\n### {name} ({status})\n{summary}")

    # What previously completed
    if checkpoint.get("completedChildren"):
        parts.append("\n## Previously Completed Children")
        for child in checkpoint["completedChildren"]:
            parts.append(f"- {child.get('name', 'unknown')}: {child.get('status', 'unknown')}")

    parts.append("\n\nContinue your work. Review the new child results and decide what to do next.")
    return "\n".join(parts)


if __name__ == "__main__":
    main()
