"""CrewAI integration for Hortator."""

from __future__ import annotations

try:
    from crewai.tools import BaseTool as CrewAIBaseTool
except ImportError:
    raise ImportError(
        "crewai is required for CrewAI integration. "
        "Install with: pip install hortator[crewai]"
    )

from ..client import HortatorClient


class HortatorCrewTool(CrewAIBaseTool):
    """CrewAI tool that delegates work to a Hortator agent."""

    name: str = "Hortator Delegate"
    description: str = (
        "Delegate a focused sub-task to a specialized AI agent running on Kubernetes. "
        "The agent will research, code, or analyze as instructed and return results."
    )

    def __init__(self, client: HortatorClient, role: str = "legionary", **kwargs):
        super().__init__(**kwargs)
        self._client = client
        self._role = role

    def _run(self, prompt: str) -> str:
        result = self._client.run(prompt, role=self._role)
        return result.content
