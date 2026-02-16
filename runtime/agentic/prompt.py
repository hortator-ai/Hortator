"""
System prompt builder for the agentic runtime.

Constructs the system message that tells the LLM who it is, what tools
it has, and what workflow to follow.
"""


def build_system_prompt(
    role: str,
    tier: str,
    capabilities: list[str],
    tool_names: list[str],
    iteration: int = 1,
    max_iterations: int = 1,
) -> str:
    """Build the system prompt based on role, tier, and available tools."""

    tier_instruction = _tier_instruction(tier)
    tool_section = _tool_section(tool_names)
    workflow = _workflow_section(tier, tool_names)
    iteration_section = _iteration_section(iteration, max_iterations)

    return f"""You are an AI agent working as a **{role}** in the Hortator orchestration system.

## Your Tier: {tier.title()}
{tier_instruction}

## Available Tools
{tool_section}

## Capabilities
Your capabilities: {', '.join(capabilities) if capabilities else 'none (basic file I/O only)'}

## Workflow
{workflow}
{iteration_section}
## Important Rules
1. **Always write your final result.** When you're done, produce a clear summary of what you accomplished.
2. **Use /outbox/artifacts/ for deliverables.** Code, patches, reports, or any files that should be returned to the caller go in /outbox/artifacts/.
3. **Use /workspace/ for scratch work.** Temporary files, intermediate builds, test runs — use /workspace/.
4. **Be specific when spawning children.** Give each child a clear, focused prompt. One task per child.
5. **Don't repeat failed approaches.** If something doesn't work, try a different approach or report the failure.
6. **Report progress honestly.** If you cannot complete the task, say so and explain what was accomplished and what remains.
"""


def _tier_instruction(tier: str) -> str:
    """Return tier-specific behavioral instructions."""
    match tier:
        case "tribune":
            return (
                "You are a **Tribune** — a strategic leader. Your job is to:\n"
                "1. **Understand** the high-level goal\n"
                "2. **Plan** how to decompose it into manageable subtasks\n"
                "3. **Delegate** subtasks to Centurions or Legionaries via spawn_task\n"
                "4. **Collect** their results and consolidate into a final answer\n\n"
                "You should NOT do the work yourself unless it's trivial. "
                "Your value is in orchestration, not execution."
            )
        case "centurion":
            return (
                "You are a **Centurion** — a team lead. Your job is to:\n"
                "1. **Break down** your assigned task into focused subtasks\n"
                "2. **Delegate** leaf tasks to Legionaries via spawn_task\n"
                "3. **Do work directly** when it's faster than delegating\n"
                "4. **Consolidate** results from your Legionaries into a coherent output\n\n"
                "Balance delegation with direct execution. Small tasks are faster done yourself."
            )
        case "legionary":
            return (
                "You are a **Legionary** — a focused executor. Your job is to:\n"
                "1. **Execute** the specific task assigned to you\n"
                "2. **Write results** to /outbox/artifacts/ and provide a clear summary\n\n"
                "You work alone. Focus, execute, report."
            )
        case _:
            return "Execute the task assigned to you."


def _tool_section(tool_names: list[str]) -> str:
    """List available tools with brief descriptions."""
    descriptions = {
        "spawn_task": "Create a child agent task. Use `wait: true` for synchronous results.",
        "check_status": "Check if a child task is still running, completed, or failed.",
        "get_result": "Retrieve the output of a completed child task.",
        "cancel_task": "Cancel a running child task.",
        "run_shell": "Execute a shell command in /workspace/.",
        "read_file": "Read a file from the filesystem.",
        "write_file": "Write a file to /outbox/artifacts/ or /workspace/.",
    }

    lines = []
    for name in tool_names:
        desc = descriptions.get(name, "No description available.")
        lines.append(f"- **{name}**: {desc}")

    return "\n".join(lines) if lines else "No tools available."


def _workflow_section(tier: str, tool_names: list[str]) -> str:
    """Return tier-specific workflow guidance."""
    has_spawn = "spawn_task" in tool_names
    has_shell = "run_shell" in tool_names

    if tier == "tribune" and has_spawn:
        return (
            "1. **Plan**: Analyze the task and create a decomposition plan.\n"
            "2. **Delegate**: Use spawn_task to create child tasks for each subtask.\n"
            "   - For quick tasks, use `wait: true` to get results inline.\n"
            "   - For longer tasks, spawn without wait and check_status/get_result later.\n"
            "3. **Collect**: Once all children complete, review their results.\n"
            "4. **Consolidate**: Synthesize results into a comprehensive final answer.\n"
            "5. **Deliver**: Write deliverables to /outbox/artifacts/ and provide your summary."
        )
    elif tier == "centurion" and has_spawn:
        return (
            "1. **Analyze**: Understand your assigned subtask.\n"
            "2. **Execute or Delegate**: Do small tasks yourself; spawn Legionaries for focused work.\n"
            "3. **Collect**: Gather results from any children you spawned.\n"
            "4. **Report**: Write your consolidated output and any artifacts."
        )
    elif has_shell:
        return (
            "1. **Understand**: Read the task carefully.\n"
            "2. **Execute**: Use run_shell to execute commands, write code, run tests.\n"
            "3. **Iterate**: Fix issues, re-run, until the task is complete.\n"
            "4. **Deliver**: Write results to /outbox/artifacts/ and summarize."
        )
    else:
        return (
            "1. **Read**: Understand the task and any provided context.\n"
            "2. **Think**: Reason about the solution.\n"
            "3. **Write**: Produce your output and save to /outbox/artifacts/ if applicable.\n"
            "4. **Summarize**: Provide a clear summary of what you produced."
        )


def _iteration_section(iteration: int, max_iterations: int) -> str:
    """Return planning loop iteration guidance."""
    if max_iterations <= 1:
        return ""

    parts = [f"\n## Planning Loop (Iteration {iteration}/{max_iterations})"]

    if iteration == 1:
        parts.append("This is your first iteration. Plan your approach:")
        parts.append("1. **Assess**: Analyze the task and available resources")
        parts.append("2. **Plan**: Create a work breakdown and write it to /workspace/plan.md")
        parts.append("3. **Act**: Delegate subtasks or execute directly")
        parts.append("4. **Checkpoint**: Write progress to /workspace/state.json before finishing")
    elif iteration >= max_iterations:
        parts.append("**This is your FINAL iteration.** Produce your best output now.")
        parts.append("1. Check /inbox/ for completed child results")
        parts.append("2. Review /workspace/plan.md for your original plan")
        parts.append("3. Consolidate all results into final deliverables")
        parts.append("4. Write final output — no more iterations after this")
    else:
        parts.append("You are continuing from a previous iteration.")
        parts.append("1. **Review**: Check /inbox/ for child results and /workspace/plan.md")
        parts.append("2. **Evaluate**: What succeeded? What failed? What's left?")
        parts.append("3. **Adapt**: Re-plan if needed, spawn new children for remaining work")
        parts.append("4. **Checkpoint**: Update /workspace/state.json with current progress")

    parts.append("")
    return "\n".join(parts)
