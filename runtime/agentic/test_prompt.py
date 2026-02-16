"""Tests for prompt.py build_system_prompt."""

import unittest
from prompt import build_system_prompt


class TestBuildSystemPrompt(unittest.TestCase):
    def test_basic_prompt(self):
        result = build_system_prompt(
            role="worker",
            tier="legionary",
            capabilities=["shell"],
            tool_names=["run_shell"],
        )
        self.assertIn("worker", result)
        self.assertIn("Legionary", result)
        self.assertIn("run_shell", result)

    def test_role_context_injected(self):
        result = build_system_prompt(
            role="backend-dev",
            tier="legionary",
            capabilities=["shell"],
            tool_names=["run_shell"],
            role_description="Backend developer with TDD focus",
            role_rules=["Write tests first", "Handle errors"],
            role_anti_patterns=["Don't use any type"],
        )
        self.assertIn("## Role: backend-dev", result)
        self.assertIn("Backend developer with TDD focus", result)
        self.assertIn("### Rules", result)
        self.assertIn("- Write tests first", result)
        self.assertIn("### Anti-Patterns (avoid these)", result)
        self.assertIn("- Don't use any type", result)

    def test_no_role_context_without_params(self):
        result = build_system_prompt(
            role="worker",
            tier="legionary",
            capabilities=[],
            tool_names=[],
        )
        self.assertNotIn("### Rules", result)
        self.assertNotIn("### Anti-Patterns", result)

    def test_delegation_section(self):
        result = build_system_prompt(
            role="orchestrator",
            tier="tribune",
            capabilities=["spawn"],
            tool_names=["spawn_task"],
            available_roles=[
                {"name": "coder", "tierAffinity": "legionary", "description": "Writes code"},
                {"name": "reviewer", "tierAffinity": "legionary", "description": "Reviews code"},
            ],
        )
        self.assertIn("## Available Roles for Delegation", result)
        self.assertIn("- **coder** (legionary): Writes code", result)
        self.assertIn("- **reviewer** (legionary): Reviews code", result)
        self.assertIn("Choose the lowest-privilege role that meets the task's needs.", result)
        self.assertIn("If no role matches exactly, pick the closest fit and adapt your approach.", result)

    def test_no_delegation_without_roles(self):
        result = build_system_prompt(
            role="worker",
            tier="legionary",
            capabilities=[],
            tool_names=[],
        )
        self.assertNotIn("Available Roles for Delegation", result)


if __name__ == "__main__":
    unittest.main()
