"""Streaming response example."""
from hortator import HortatorClient

client = HortatorClient(
    base_url="http://localhost:8080",
    api_key="hrt-test-key",
)

print("Streaming response:")
for chunk in client.stream(
    "Write a comprehensive analysis of Kubernetes operator patterns",
    role="tech-lead",
    tier="centurion",
):
    print(chunk.content, end="", flush=True)
print()  # newline at end
