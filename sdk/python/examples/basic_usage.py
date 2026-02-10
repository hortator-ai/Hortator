"""Basic Hortator SDK usage."""
from hortator import HortatorClient

client = HortatorClient(
    base_url="http://localhost:8080",
    api_key="hrt-test-key",
)

# Simple task
result = client.run(
    "What are the key differences between Python 3.12 and 3.13?",
    role="researcher",
)
print(f"Result: {result.content}")
print(f"Tokens used: {result.usage.total_tokens}")
print(f"Finish reason: {result.finish_reason}")
