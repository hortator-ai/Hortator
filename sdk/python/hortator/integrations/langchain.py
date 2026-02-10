"""LangChain/LangGraph integration for Hortator."""

from __future__ import annotations

try:
    from langchain_core.tools import BaseTool
except ImportError:
    raise ImportError(
        "langchain-core is required for LangChain integration. "
        "Install with: pip install hortator[langchain]"
    )

from pydantic import Field

from ..client import HortatorClient


class HortatorDelegateTool(BaseTool):
    """LangChain tool that delegates work to a Hortator agent."""

    name: str = "hortator_delegate"
    description: str = (
        "Delegate a task to a specialized AI agent running on Kubernetes. "
        "Use this when you need another agent to research, code, analyze, or "
        "perform a focused sub-task. Provide a clear prompt describing what you need."
    )

    client: HortatorClient = Field(exclude=True)
    default_role: str = "legionary"
    default_capabilities: list[str] = Field(default_factory=list)
    default_tier: str = "legionary"

    def _run(self, prompt: str, role: str = "", capabilities: str = "") -> str:
        """Delegate a task to a Hortator agent."""
        r = role or self.default_role
        caps = capabilities.split(",") if capabilities else self.default_capabilities
        result = self.client.run(prompt, role=r, capabilities=caps, tier=self.default_tier)
        return result.content

    async def _arun(self, prompt: str, role: str = "", capabilities: str = "") -> str:
        return self._run(prompt, role, capabilities)
