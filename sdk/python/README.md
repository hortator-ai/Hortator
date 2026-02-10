# Hortator Python SDK

Python client for [Hortator](https://hortator.ai) — Kubernetes-native AI agent orchestration.

## Install

```bash
pip install hortator

# With LangChain integration
pip install hortator[langchain]

# With CrewAI integration
pip install hortator[crewai]
```

## Quick Start

### Basic Usage

```python
from hortator import HortatorClient

client = HortatorClient(base_url="https://hortator.example.com", api_key="hrt-...")

result = client.run("Research quantum computing advances", role="researcher")
print(result.content)
print(f"Tokens: {result.usage.total_tokens}")
```

### Streaming

```python
for chunk in client.stream("Write a detailed report", role="tech-lead"):
    print(chunk.content, end="", flush=True)
```

### LangChain / LangGraph Tool

```python
from hortator.integrations.langchain import HortatorDelegateTool

delegate = HortatorDelegateTool(client=client, default_role="researcher")

# Use in a LangGraph agent
from langgraph.prebuilt import create_react_agent
agent = create_react_agent(model, tools=[delegate])
```

## Hortator Extensions

The SDK supports Hortator-specific features beyond the OpenAI API:

```python
result = client.run(
    "Analyze this codebase",
    role="security-auditor",
    capabilities=["shell", "web-fetch"],
    tier="centurion",
    budget={"max_cost_usd": "1.00"},
)
```

## OpenAI Compatibility

Hortator's API is OpenAI-compatible, so the `openai` Python SDK works too. This SDK adds convenience methods and Hortator-specific features (capabilities, tiers, budgets).

## Docs

Full documentation: [hortator.ai/docs](https://hortator.ai/docs)
