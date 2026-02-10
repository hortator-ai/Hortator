"""Using Hortator as a LangChain/LangGraph tool."""
from hortator import HortatorClient
from hortator.integrations.langchain import HortatorDelegateTool

client = HortatorClient(
    base_url="http://localhost:8080",
    api_key="hrt-test-key",
)

# Create the tool
delegate = HortatorDelegateTool(
    client=client,
    default_role="researcher",
    default_capabilities=["web-fetch"],
)

# Use directly
result = delegate.run("Research the latest advances in quantum error correction")
print(result)

# Or add to a LangGraph agent:
# from langgraph.prebuilt import create_react_agent
# agent = create_react_agent(model, tools=[delegate])
