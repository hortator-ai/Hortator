"""Tests for prompt.py build_system_prompt."""

import unittest
from prompt import build_system_prompt


class TestBuildSystemPrompt(unittest.TestCase):
    """Test the v2 system prompt builder."""

    # --- Identity & Tier ---

    def test_legionary_identity(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        self.assertIn("**worker** (legionary)", result)
        self.assertIn("focused executor", result)
        self.assertNotIn("Delegation", result)  # No delegation for legionaries

    def test_tribune_identity(self):
        result = build_system_prompt(
            role="architect", tier="tribune",
            capabilities=["spawn"], tool_names=["spawn_task"],
        )
        self.assertIn("**architect** (tribune)", result)
        self.assertIn("strategic orchestrator", result)
        self.assertIn("do NOT implement", result)

    def test_centurion_identity(self):
        result = build_system_prompt(
            role="team-lead", tier="centurion",
            capabilities=["spawn", "shell"], tool_names=["spawn_task", "run_shell"],
        )
        self.assertIn("**team-lead** (centurion)", result)
        self.assertIn("team lead", result)

    # --- Constraints ---

    def test_budget_shown(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            budget_usd="5.00", budget_tokens=500000,
        )
        self.assertIn("$5.00", result)
        self.assertIn("500,000", result)

    def test_timeout_shown(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            timeout_seconds=3600,
        )
        self.assertIn("60m", result)

    def test_no_constraints_when_empty(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=[], tool_names=[],
        )
        self.assertNotIn("## Constraints", result)

    # --- Filesystem ---

    def test_filesystem_section_present(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        self.assertIn("/workspace/", result)
        self.assertIn("/outbox/artifacts/", result)
        self.assertNotIn("/inbox/", result)  # No inbox for non-spawners

    def test_inbox_shown_for_spawners(self):
        result = build_system_prompt(
            role="architect", tier="tribune",
            capabilities=["spawn"], tool_names=["spawn_task"],
        )
        self.assertIn("/inbox/", result)

    # --- Safety ---

    def test_safety_always_present(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=[], tool_names=[],
        )
        self.assertIn("## Safety", result)
        self.assertIn("Do not exfiltrate", result)

    # --- Tools ---

    def test_tool_section(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell", "write_file"],
        )
        self.assertIn("**run_shell**", result)
        self.assertIn("**write_file**", result)

    # --- Delegation ---

    def test_tribune_delegation_guidance(self):
        result = build_system_prompt(
            role="architect", tier="tribune",
            capabilities=["spawn"], tool_names=["spawn_task"],
            available_roles=[
                {"name": "coder", "tierAffinity": "legionary", "description": "Writes code"},
            ],
        )
        self.assertIn("## Delegation", result)
        self.assertIn("plan to `/workspace/plan.md`", result)
        self.assertIn("Never spawn two children with overlapping scope", result)
        self.assertIn("**coder** (legionary): Writes code", result)

    def test_centurion_delegation_guidance(self):
        result = build_system_prompt(
            role="lead", tier="centurion",
            capabilities=["spawn", "shell"], tool_names=["spawn_task", "run_shell"],
        )
        self.assertIn("## Delegation", result)
        self.assertIn("When to delegate vs execute directly", result)

    def test_no_delegation_for_legionary(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        self.assertNotIn("## Delegation", result)
        self.assertNotIn("Available roles", result)

    def test_no_delegation_without_spawn_tool(self):
        """Centurion without spawn_task should not get delegation section."""
        result = build_system_prompt(
            role="lead", tier="centurion",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        self.assertNotIn("## Delegation", result)

    # --- Role Context (recency bias — should be near end) ---

    def test_role_context_present(self):
        result = build_system_prompt(
            role="backend-dev", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            role_description="Backend developer with TDD focus",
            role_rules=["Write tests first", "Handle errors"],
            role_anti_patterns=["Don't use any type"],
        )
        self.assertIn("## Your Role: backend-dev", result)
        self.assertIn("Backend developer with TDD focus", result)
        self.assertIn("- Write tests first", result)
        self.assertIn("- Don't use any type", result)

    def test_role_context_after_tools(self):
        """Role context should appear AFTER tools section (recency bias)."""
        result = build_system_prompt(
            role="backend-dev", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            role_description="Backend dev",
            role_rules=["Write tests"],
        )
        tools_pos = result.find("## Tools")
        role_pos = result.find("## Your Role")
        self.assertGreater(role_pos, tools_pos)

    def test_no_role_context_without_params(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=[], tool_names=[],
        )
        self.assertNotIn("## Your Role", result)

    # --- Exit Criteria ---

    def test_exit_criteria_present(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            exit_criteria="All tests pass with exit code 0",
        )
        self.assertIn("## Exit Criteria", result)
        self.assertIn("All tests pass with exit code 0", result)

    def test_exit_criteria_absent_when_empty(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        self.assertNotIn("## Exit Criteria", result)

    # --- Iteration ---

    def test_iteration_first(self):
        result = build_system_prompt(
            role="architect", tier="tribune",
            capabilities=["spawn"], tool_names=["spawn_task"],
            iteration=1, max_iterations=5,
        )
        self.assertIn("Iteration 1/5", result)
        self.assertIn("plan.md", result)

    def test_iteration_final(self):
        result = build_system_prompt(
            role="architect", tier="tribune",
            capabilities=["spawn"], tool_names=["spawn_task"],
            iteration=5, max_iterations=5,
        )
        self.assertIn("FINAL iteration", result)

    def test_no_iteration_for_single_shot(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            iteration=1, max_iterations=1,
        )
        self.assertNotIn("Iteration", result)

    # --- Rules (at the very end) ---

    def test_rules_at_end(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        rules_pos = result.find("## Rules")
        self.assertEqual(rules_pos, result.rfind("## Rules"))  # Only one Rules section
        # Should be the last ## section
        last_section = result.rfind("\n## ")
        self.assertEqual(result[last_section:].strip().startswith("## Rules"), True)

    def test_spawner_rules(self):
        result = build_system_prompt(
            role="architect", tier="tribune",
            capabilities=["spawn"], tool_names=["spawn_task"],
        )
        self.assertIn("Plan before acting", result)
        self.assertIn("One scope per child", result)

    def test_focused_rules(self):
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
        )
        self.assertIn("Stay focused", result)
        self.assertIn("Iterate on failures", result)

    # --- Prompt mode (focused vs full) ---

    def test_legionary_gets_focused_prompt(self):
        """Legionaries should NOT get delegation or iteration sections."""
        result = build_system_prompt(
            role="worker", tier="legionary",
            capabilities=["shell"], tool_names=["run_shell"],
            iteration=3, max_iterations=5,
            available_roles=[{"name": "other", "tierAffinity": "legionary", "description": "x"}],
        )
        self.assertNotIn("## Delegation", result)
        self.assertNotIn("Iteration 3/5", result)
        self.assertNotIn("Available roles", result)


if __name__ == "__main__":
    unittest.main()
