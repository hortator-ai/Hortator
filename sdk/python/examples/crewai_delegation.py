"""Using Hortator with CrewAI."""
from hortator import HortatorClient
from hortator.integrations.crewai import HortatorCrewTool

client = HortatorClient(
    base_url="http://localhost:8080",
    api_key="hrt-test-key",
)

# Create tools for different roles
researcher = HortatorCrewTool(client=client, role="researcher")
coder = HortatorCrewTool(client=client, role="backend-dev")

# Use in CrewAI:
# from crewai import Agent, Task, Crew
# agent = Agent(
#     role="Project Manager",
#     tools=[researcher, coder],
#     ...
# )
