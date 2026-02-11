"""
Tool definitions for the agentic runtime.

Tools are gated by the agent's capabilities (from AgentTask spec).
Returns OpenAI-compatible tool schemas for the LLM.
"""


def build_tools(capabilities: list[str], task_name: str, task_ns: str) -> list[dict]:
    """Build the list of available tools based on agent capabilities."""
    tools = []

    # Always available
    tools.append(_tool_read_file())
    tools.append(_tool_write_file())

    # spawn capability gates task management tools
    if "spawn" in capabilities:
        tools.append(_tool_spawn_task())
        tools.append(_tool_check_status())
        tools.append(_tool_get_result())
        tools.append(_tool_cancel_task())

    # shell capability gates command execution
    if "shell" in capabilities:
        tools.append(_tool_run_shell())

    return tools


# ── Tool Schemas (OpenAI function calling format) ────────────────────────────

def _tool_spawn_task() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "spawn_task",
            "description": (
                "Create a child AgentTask. The child runs as a separate agent pod. "
                "Use --wait to block until the child completes and get its result, "
                "or omit --wait to fire-and-forget (you'll need to check_status/get_result later). "
                "The child inherits your namespace and cannot escalate beyond your capabilities."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "prompt": {
                        "type": "string",
                        "description": "The task instruction for the child agent.",
                    },
                    "role": {
                        "type": "string",
                        "description": "AgentRole name for the child (e.g. 'backend-dev', 'qa-engineer').",
                    },
                    "tier": {
                        "type": "string",
                        "enum": ["centurion", "legionary"],
                        "description": "Hierarchy tier. Centurions can spawn legionaries; legionaries are leaf tasks.",
                    },
                    "capabilities": {
                        "type": "string",
                        "description": "Comma-separated capabilities (e.g. 'shell,web-fetch'). Must be subset of your own.",
                    },
                    "wait": {
                        "type": "boolean",
                        "description": "If true, block until the child completes and return its result. Default: false.",
                    },
                },
                "required": ["prompt"],
            },
        },
    }


def _tool_check_status() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "check_status",
            "description": "Check the current status of a child task (Pending, Running, Completed, Failed, etc.).",
            "parameters": {
                "type": "object",
                "properties": {
                    "task_name": {
                        "type": "string",
                        "description": "Name of the child task to check.",
                    },
                },
                "required": ["task_name"],
            },
        },
    }


def _tool_get_result() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "get_result",
            "description": "Retrieve the output/result of a completed child task.",
            "parameters": {
                "type": "object",
                "properties": {
                    "task_name": {
                        "type": "string",
                        "description": "Name of the child task.",
                    },
                },
                "required": ["task_name"],
            },
        },
    }


def _tool_cancel_task() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "cancel_task",
            "description": "Cancel a running child task.",
            "parameters": {
                "type": "object",
                "properties": {
                    "task_name": {
                        "type": "string",
                        "description": "Name of the child task to cancel.",
                    },
                },
                "required": ["task_name"],
            },
        },
    }


def _tool_run_shell() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "run_shell",
            "description": (
                "Execute a shell command in /workspace/. Returns stdout, stderr, and exit code. "
                "Use for: running tests, installing packages, compiling code, git operations, etc."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "The shell command to execute.",
                    },
                    "timeout": {
                        "type": "integer",
                        "description": "Timeout in seconds (default: 120).",
                    },
                },
                "required": ["command"],
            },
        },
    }


def _tool_read_file() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read the contents of a file from the agent's filesystem.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "Absolute path to the file to read.",
                    },
                },
                "required": ["path"],
            },
        },
    }


def _tool_write_file() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "write_file",
            "description": (
                "Write content to a file. Use /outbox/artifacts/ for deliverables "
                "(code, reports, patches) that should be returned to the caller. "
                "Use /workspace/ for temporary/scratch files."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "Absolute path to write to (e.g. /outbox/artifacts/main.py or /workspace/scratch.txt).",
                    },
                    "content": {
                        "type": "string",
                        "description": "The file content to write.",
                    },
                },
                "required": ["path", "content"],
            },
        },
    }
