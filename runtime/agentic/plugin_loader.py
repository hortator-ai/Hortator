"""
Plugin loader for the agentic runtime.

Auto-discovers and loads plugin modules from the plugins/ directory,
gated by task capabilities.
"""

import importlib
import pkgutil
from pathlib import Path
from types import ModuleType

PLUGIN_DIR = Path(__file__).parent / "plugins"


def load_plugins(capabilities: list[str]) -> dict[str, ModuleType]:
    """Auto-discover plugins in PLUGIN_DIR. Only load if CAPABILITY in capabilities.

    Args:
        capabilities: List of capability strings from the task spec.

    Returns:
        Dictionary mapping capability name to loaded plugin module.
    """
    plugins = {}

    if not PLUGIN_DIR.exists():
        return plugins

    # Iterate over all modules in the plugins directory
    for finder, name, ispkg in pkgutil.iter_modules([str(PLUGIN_DIR)]):
        if name.startswith("_"):
            continue

        try:
            module = importlib.import_module(f"plugins.{name}")

            # Check if module exports required attributes
            if not hasattr(module, "CAPABILITY"):
                continue
            if not hasattr(module, "TOOLS"):
                continue
            if not hasattr(module, "execute"):
                continue

            # Only load if capability is enabled
            capability = module.CAPABILITY
            if capability in capabilities:
                plugins[capability] = module

        except Exception:
            # Skip modules that fail to load
            continue

    return plugins


def get_plugin_tools(capabilities: list[str]) -> list[dict]:
    """Return combined tool schemas from all active plugins.

    Args:
        capabilities: List of capability strings from the task spec.

    Returns:
        List of OpenAI-compatible tool schemas from all matching plugins.
    """
    tools = []
    plugins = load_plugins(capabilities)

    for module in plugins.values():
        tools.extend(module.TOOLS)

    return tools


def dispatch_plugin_tool(
    name: str,
    args: dict,
    capabilities: list[str],
    env: dict,
) -> dict | None:
    """Try to dispatch a tool call to a plugin.

    Args:
        name: Tool name to execute.
        args: Parsed LLM tool call arguments.
        capabilities: List of capability strings from the task spec.
        env: Dictionary of environment variables (API keys etc.).

    Returns:
        Tool execution result dict, or None if no plugin handles this tool.
    """
    plugins = load_plugins(capabilities)

    for module in plugins.values():
        # Check if this plugin has the requested tool
        tool_names = [t["function"]["name"] for t in module.TOOLS]
        if name in tool_names:
            return module.execute(name, args, env)

    return None
