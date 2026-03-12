"""
Plugin system for extending the agentic runtime with external tool sets.

Each plugin module must export:
- CAPABILITY: str - the capability name that gates loading
- TOOLS: list[dict] - OpenAI-compatible tool schemas
- execute(name: str, args: dict, env: dict) -> dict - tool execution function
"""


class ToolExecutionError(Exception):
    """Raised when a plugin encounters an unrecoverable error.

    Use this for credential/configuration errors that cannot be recovered
    from within the tool execution (e.g., missing API keys).
    """
    pass
