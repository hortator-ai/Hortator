"""
Core agentic tool-calling loop.

Calls the LLM, parses tool calls, executes them, feeds results back.
Repeats until the LLM returns a final answer (end_turn) or the agent
decides to checkpoint and wait for children.
"""

import hashlib
import json
import time
from dataclasses import dataclass, field

import litellm

from checkpoint import save_checkpoint
from tool_executor import execute_tool

# Silence litellm's noisy startup logs
litellm.suppress_debug_info = True

MAX_ITERATIONS = 200  # Safety valve — no infinite loops


@dataclass
class LoopResult:
    """Result of the agentic loop."""
    status: str  # "completed", "failed", "waiting", "budget_exceeded"
    output: str = ""
    tokens_in: int = 0
    tokens_out: int = 0
    artifacts: list[str] = field(default_factory=list)
    pending_children: list[str] = field(default_factory=list)


def agentic_loop(
    messages: list[dict],
    tools: list[dict],
    model: str,
    task_name: str,
    task_ns: str,
    budget: dict,
    state_file: str,
    is_killed: callable,
) -> LoopResult:
    """
    Run the tool-calling loop until the LLM produces a final answer,
    the agent checkpoints to wait for children, or the budget is exhausted.
    """
    total_in = 0
    total_out = 0
    spawned_children: list[str] = []
    pending_children: list[str] = []
    decisions: list[str] = []
    artifacts: list[str] = []

    max_tokens_budget = budget.get("maxTokens")
    if isinstance(max_tokens_budget, str):
        try:
            max_tokens_budget = int(max_tokens_budget)
        except ValueError:
            max_tokens_budget = None

    for iteration in range(MAX_ITERATIONS):
        if is_killed():
            # Graceful shutdown — checkpoint and exit
            _save_waiting_checkpoint(
                state_file, task_name, spawned_children,
                pending_children, decisions, messages,
            )
            return LoopResult(
                status="failed",
                output="Task killed by SIGTERM",
                tokens_in=total_in,
                tokens_out=total_out,
            )

        # Budget check
        if max_tokens_budget and (total_in + total_out) >= max_tokens_budget:
            _save_waiting_checkpoint(
                state_file, task_name, spawned_children,
                pending_children, decisions, messages,
            )
            return LoopResult(
                status="budget_exceeded",
                output=f"Budget exceeded: {total_in + total_out} tokens used "
                       f"(limit: {max_tokens_budget})",
                tokens_in=total_in,
                tokens_out=total_out,
            )

        # Log prompt hash for stuck detection (operator parses this)
        last_user = ""
        for msg in reversed(messages):
            if msg.get("role") == "user" and isinstance(msg.get("content"), str):
                last_user = msg["content"]
                break
        if last_user:
            prompt_hash = hashlib.sha256(last_user.encode()).hexdigest()[:16]
            print(f"[hortator-agentic] Prompt hash: {prompt_hash}")

        # Call LLM
        try:
            response = litellm.completion(
                model=model,
                messages=messages,
                tools=tools if tools else None,
                tool_choice="auto" if tools else None,
                max_tokens=4096,
            )
        except Exception as e:
            return LoopResult(
                status="failed",
                output=f"LLM call failed: {e}",
                tokens_in=total_in,
                tokens_out=total_out,
            )

        # Track tokens
        usage = response.usage
        if usage:
            total_in += usage.prompt_tokens or 0
            total_out += usage.completion_tokens or 0

        choice = response.choices[0]
        assistant_message = choice.message

        # Append assistant response to conversation
        messages.append(assistant_message.model_dump())

        # Check stop reason
        finish_reason = choice.finish_reason

        if finish_reason == "stop" or finish_reason == "end_turn":
            # LLM is done — extract final answer
            content = assistant_message.content or ""
            return LoopResult(
                status="completed",
                output=content,
                tokens_in=total_in,
                tokens_out=total_out,
                artifacts=artifacts,
            )

        if finish_reason == "tool_calls" or assistant_message.tool_calls:
            # Execute each tool call
            for tool_call in assistant_message.tool_calls or []:
                func_name = tool_call.function.name
                try:
                    func_args = json.loads(tool_call.function.arguments)
                except json.JSONDecodeError:
                    func_args = {}

                print(f"[hortator-agentic] Tool call: {func_name}({json.dumps(func_args)[:200]})")

                result = execute_tool(func_name, func_args, task_name, task_ns)

                # Track spawned children
                if func_name == "spawn_task" and result.get("success"):
                    child_name = result.get("task_name", "")
                    if child_name:
                        spawned_children.append(child_name)
                        if result.get("async"):
                            pending_children.append(child_name)

                # Track artifacts from write_file
                if func_name == "write_file" and result.get("success"):
                    path = result.get("path", "")
                    if path.startswith("/outbox/artifacts/"):
                        artifacts.append(path.removeprefix("/outbox/artifacts/"))

                # Append tool result to conversation
                messages.append({
                    "role": "tool",
                    "tool_call_id": tool_call.id,
                    "content": json.dumps(result),
                })

            continue

        # If the LLM stopped for another reason (length, content filter),
        # treat the content as the final output
        content = assistant_message.content or ""
        if content:
            return LoopResult(
                status="completed",
                output=content,
                tokens_in=total_in,
                tokens_out=total_out,
                artifacts=artifacts,
            )

        # No content and no tool calls — unexpected, bail out
        return LoopResult(
            status="failed",
            output=f"Unexpected stop reason: {finish_reason}",
            tokens_in=total_in,
            tokens_out=total_out,
        )

    # Exhausted iteration limit
    _save_waiting_checkpoint(
        state_file, task_name, spawned_children,
        pending_children, decisions, messages,
    )
    return LoopResult(
        status="failed",
        output=f"Iteration limit reached ({MAX_ITERATIONS})",
        tokens_in=total_in,
        tokens_out=total_out,
    )


def _save_waiting_checkpoint(
    state_file: str,
    task_name: str,
    spawned_children: list[str],
    pending_children: list[str],
    decisions: list[str],
    messages: list[dict],
):
    """Save checkpoint state for reincarnation."""
    # Build accumulated context from the last few assistant messages
    accumulated = []
    for msg in reversed(messages):
        if msg.get("role") == "assistant" and msg.get("content"):
            accumulated.insert(0, msg["content"][:500])
        if len(accumulated) >= 3:
            break

    save_checkpoint(state_file, {
        "version": 1,
        "taskId": task_name,
        "phase": "waiting",
        "completedChildren": [
            {"name": c, "status": "Completed"}
            for c in spawned_children if c not in pending_children
        ],
        "pendingChildren": [
            {"name": c, "status": "Running"} for c in pending_children
        ],
        "decisions": decisions,
        "accumulatedContext": "\n---\n".join(accumulated),
    })
