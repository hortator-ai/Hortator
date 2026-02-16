"""
System prompt builder for the agentic runtime.

Constructs the system message from composable sections, inspired by
OpenClaw's layered prompt architecture:

  1. Identity + tier (who you are)
  2. Constraints (budget, time, iterations) — front-loaded
  3. Filesystem contract (workspace, inbox, outbox)
  4. Safety (always present)
  5. Tools (what you can call)
  6. Delegation guide (tribunes/centurions only)
  7. Role context (rules, anti-patterns) — at the END for recency bias

Two modes:
  - "full" for tribunes/centurions (delegation, role registry, planning)
  - "focused" for legionaries (just execute, no delegation noise)
"""


def build_system_prompt(
    role: str,
    tier: str,
    capabilities: list[str],
    tool_names: list[str],
    role_description: str = "",
    role_rules: list[str] | None = None,
    role_anti_patterns: list[str] | None = None,
    available_roles: list[dict] | None = None,
    exit_criteria: str = "",
    iteration: int = 1,
    max_iterations: int = 1,
    budget_tokens: int | None = None,
    budget_usd: str | None = None,
    timeout_seconds: int | None = None,
) -> str:
    """Build the system prompt based on role, tier, and available tools."""

    is_spawner = tier in ("tribune", "centurion") and "spawn_task" in tool_names
    is_focused = tier == "legionary" or not is_spawner

    sections = [
        _identity_section(role, tier),
        _constraints_section(budget_tokens, budget_usd, timeout_seconds, iteration, max_iterations),
        _filesystem_section(is_spawner),
        _safety_section(),
        _tool_section(tool_names),
        _delegation_section(tier, available_roles) if is_spawner else "",
        _iteration_section(iteration, max_iterations) if not is_focused else "",
        _exit_criteria_section(exit_criteria),
        # Role context goes LAST — recency bias means the LLM weights
        # the end of the prompt more heavily. Role-specific rules and
        # anti-patterns are the most important behavioral constraints.
        _role_context_section(role, role_description, role_rules, role_anti_patterns),
        _rules_section(tier, is_spawner),
    ]

    return "\n".join(s for s in sections if s)


def _identity_section(role: str, tier: str) -> str:
    """Who you are and how you operate."""
    tier_desc = {
        "tribune": (
            "You are a **Tribune** — a strategic orchestrator.\n"
            "You analyse tasks, decide whether they need decomposition, and delegate to specialists when they do.\n"
            "Your value is judgement: knowing WHEN to delegate (multi-concern tasks) vs WHEN to just do it (simple, focused work).\n"
            "When you delegate, design the plan so workers can run in parallel — define shared contracts upfront."
        ),
        "centurion": (
            "You are a **Centurion** — a team lead.\n"
            "You balance direct execution with delegation.\n"
            "Do small tasks yourself. Delegate focused leaf tasks to Legionaries.\n"
            "Consolidate results into a coherent output."
        ),
        "legionary": (
            "You are a **Legionary** — a focused executor.\n"
            "You receive a specific task and execute it thoroughly.\n"
            "Focus, execute, deliver. No delegation."
        ),
    }.get(tier, "Execute the task assigned to you.")

    return f"You are **{role}** ({tier}) in the Hortator orchestration system.\n\n{tier_desc}"


def _constraints_section(
    budget_tokens: int | None,
    budget_usd: str | None,
    timeout_seconds: int | None,
    iteration: int,
    max_iterations: int,
) -> str:
    """Front-load constraints so the agent plans around them."""
    lines = ["\n## Constraints"]
    if budget_usd:
        lines.append(f"- **Budget:** ${budget_usd} USD. Each child task and tool call costs tokens. Be efficient.")
    if budget_tokens:
        lines.append(f"- **Token limit:** {budget_tokens:,} tokens.")
    if timeout_seconds:
        minutes = timeout_seconds // 60
        lines.append(f"- **Timeout:** {minutes}m. Plan accordingly.")
    if max_iterations > 1:
        lines.append(f"- **Iterations:** {iteration}/{max_iterations}.")
    if len(lines) == 1:
        return ""  # No constraints to show
    return "\n".join(lines)


def _filesystem_section(is_spawner: bool) -> str:
    """Explicit filesystem contract — where things live."""
    lines = [
        "\n## Filesystem",
        "- `/workspace/` — your scratch space. Plans, intermediate files, builds.",
        "- `/outbox/artifacts/` — **deliverables go here.** Code, reports, anything returned to the caller.",
        "- `/outbox/result.json` — structured result summary (optional).",
    ]
    if is_spawner:
        lines.append(
            "- `/inbox/` — child task results appear here automatically when children complete."
        )
    return "\n".join(lines)


def _safety_section() -> str:
    """Always present, brief."""
    return (
        "\n## Safety\n"
        "- Operate within your assigned tier and capabilities. Do not attempt to escalate.\n"
        "- Report failures and blockers honestly — do not fabricate results.\n"
        "- Do not exfiltrate data outside your task scope."
    )


def _tool_section(tool_names: list[str]) -> str:
    """List available tools with brief descriptions."""
    descriptions = {
        "spawn_task": "Create a child agent task. Specify role, tier, and a focused prompt.",
        "check_status": "Check if a child task is running, completed, or failed.",
        "get_result": "Retrieve the output of a completed child task.",
        "cancel_task": "Cancel a running child task.",
        "run_shell": "Execute a shell command in /workspace/.",
        "read_file": "Read a file from the filesystem.",
        "write_file": "Write a file to the filesystem.",
    }

    lines = ["\n## Tools"]
    for name in tool_names:
        desc = descriptions.get(name, "No description available.")
        lines.append(f"- **{name}**: {desc}")

    if not tool_names:
        lines.append("No tools available.")

    return "\n".join(lines)


def _delegation_section(tier: str, available_roles: list[dict] | None) -> str:
    """Delegation guidance for tribunes and centurions."""
    lines = ["\n## Delegation"]

    if tier == "tribune":
        lines.extend([
            "Before spawning ANY children, write a plan to `/workspace/plan.md`.",
            "The plan must define clear interfaces and contracts (API endpoints, data models, file paths)",
            "so that workers share a common spec and can work **in parallel**.",
            "",
            "**When to delegate vs do it yourself:**",
            "- If the task is a single focused deliverable (one file, < ~100 lines), just do it. Delegation has overhead (pod spin-up, context transfer) that isn't worth it for trivial work.",
            "- If the task has multiple distinct concerns that require different expertise, delegate.",
            "",
            "**Delegation rules:**",
            "- Each child gets ONE clearly scoped piece of work. Never give a child the entire task.",
            "- Never spawn two children with overlapping scope.",
            "- Give each child a specific prompt including: what to build, interfaces/contracts to follow, file paths, and constraints.",
            "- **Spawn independent children in parallel.** Don't wait for one child to finish before spawning the next unless there's a real data dependency (e.g., child B needs the actual output of child A, not just the same spec).",
            "- Wait for all children to complete, then review quality before consolidating.",
        ])
    else:  # centurion
        lines.extend([
            "**When to delegate vs execute directly:**",
            "- Tasks you can finish in a few tool calls → do it yourself.",
            "- Tasks requiring focused, independent work → spawn a Legionary.",
            "- When delegating, give each child a precise scope and expected output path.",
        ])

    if available_roles:
        lines.append("\n**Available roles:**")
        for r in available_roles:
            name = r.get("name", "unknown")
            t = r.get("tierAffinity", "")
            desc = r.get("description", "")
            lines.append(f"- **{name}** ({t}): {desc}")
        lines.append("\nChoose the lowest-privilege role that fits. Don't over-delegate simple work.")

    return "\n".join(lines)


def _exit_criteria_section(exit_criteria: str) -> str:
    """When the agent should consider itself done."""
    if not exit_criteria:
        return ""

    return (
        f"\n## Exit Criteria\n"
        f"You are done when: {exit_criteria}\n"
        f"Evaluate this after each major step. If met, produce your final output."
    )


def _iteration_section(iteration: int, max_iterations: int) -> str:
    """Planning loop iteration guidance."""
    if max_iterations <= 1:
        return ""

    lines = [f"\n## Iteration {iteration}/{max_iterations}"]

    if iteration == 1:
        lines.extend([
            "First iteration. Write your plan to `/workspace/plan.md` before acting.",
            "Checkpoint progress to `/workspace/state.json` before finishing."
        ])
    elif iteration >= max_iterations:
        lines.extend([
            "**FINAL iteration.** Produce your best output now.",
            "Check `/inbox/` for child results. Consolidate into final deliverables.",
        ])
    else:
        lines.extend([
            "Continuing from previous iteration.",
            "Check `/inbox/` for child results. Review `/workspace/plan.md`.",
            "Adapt plan if needed. Checkpoint to `/workspace/state.json`.",
        ])

    return "\n".join(lines)


def _role_context_section(
    role: str,
    description: str,
    rules: list[str] | None,
    anti_patterns: list[str] | None,
) -> str:
    """Role context at the END of the prompt for recency bias."""
    if not rules and not anti_patterns and not description:
        return ""

    parts = [f"\n## Your Role: {role}"]
    if description:
        parts.append(description)

    if rules:
        parts.append("\n**Rules:**")
        for rule in rules:
            parts.append(f"- {rule}")

    if anti_patterns:
        parts.append("\n**Avoid:**")
        for ap in anti_patterns:
            parts.append(f"- {ap}")

    return "\n".join(parts)


def _rules_section(tier: str, is_spawner: bool) -> str:
    """Hard rules at the very end — highest recency weight."""
    lines = ["\n## Rules"]

    # Universal rules
    lines.extend([
        "1. **Always deliver.** Write your final result to `/outbox/artifacts/`. No silent exits.",
        "2. **Be honest.** If you can't complete the task, say what you accomplished and what remains.",
    ])

    if is_spawner:
        lines.extend([
            "3. **Plan before acting.** No spawn calls before you have a written plan.",
            "4. **One scope per child.** Overlapping delegation wastes budget and causes conflicts.",
            "5. **Review before consolidating.** Check child results for quality.",
        ])
    else:
        lines.extend([
            "3. **Stay focused.** Execute your specific task. Don't expand scope.",
            "4. **Iterate on failures.** If something breaks, try a different approach before giving up.",
        ])

    return "\n".join(lines)
