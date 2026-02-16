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
    role_description: str = "",
    role_rules: list[str] = None,
    role_anti_patterns: list[str] = None,
    available_roles: list[dict] = None,
    exit_criteria: str = "",
) -> str:
    """Build the system prompt based on role, tier, and available tools."""

    tier_instruction = _tier_instruction(tier)
    tool_section = _tool_section(tool_names)
    workflow = _workflow_section(tier, tool_names)
    role_context = _role_context_section(role, role_description, role_rules, role_anti_patterns)
    delegation_section = _delegation_section(available_roles)
    exit_criteria_section = _exit_criteria_section(exit_criteria)

    return f"""You are an AI agent working as a **{role}** in the Hortator orchestration system.

## Your Tier: {tier.title()}
{tier_instruction}
{role_context}
## Available Tools
{tool_section}

## Capabilities
Your capabilities: {', '.join(capabilities) if capabilities else 'none (basic file I/O only)'}

## Workflow
{workflow}
{delegation_section}{exit_criteria_section}
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


def _role_context_section(
    role: str,
    description: str,
    rules: list[str] | None,
    anti_patterns: list[str] | None,
) -> str:
    """Build the role context section with rules and anti-patterns."""
    if not rules and not anti_patterns and not description:
        return ""

    parts = [f"\n## Role: {role}"]
    if description:
        parts.append(description)

    if rules:
        parts.append("\n### Rules")
        for rule in rules:
            parts.append(f"- {rule}")

    if anti_patterns:
        parts.append("\n### Anti-Patterns (avoid these)")
        for ap in anti_patterns:
            parts.append(f"- {ap}")

    parts.append("")
    return "\n".join(parts)


def _delegation_section(available_roles: list[dict] | None) -> str:
    """Build the delegation section listing available roles."""
    if not available_roles:
        return ""

    parts = [
        "\n## Available Roles for Delegation",
        "When spawning child tasks, choose from these roles:",
    ]
    for role in available_roles:
        name = role.get("name", "unknown")
        tier = role.get("tierAffinity", "")
        desc = role.get("description", "")
        parts.append(f"- **{name}** ({tier}): {desc}")

    parts.append(
        "\nChoose the lowest-privilege role that meets the task's needs.\n"
        "If no role matches exactly, pick the closest fit and adapt your approach.\n"
    )
    return "\n".join(parts)


def _exit_criteria_section(exit_criteria: str) -> str:
    """Build the exit criteria section when provided."""
    if not exit_criteria:
        return ""

    return (
        "\n## Exit Criteria\n"
        f"You are done when: {exit_criteria}\n\n"
        "Evaluate this criteria at the end of each iteration. If met, produce your final output.\n"
        "If not met and you have iterations remaining, continue working.\n"
    )
