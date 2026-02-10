"""Tests for Hortator client."""

import json

import httpx
import pytest
import respx

from hortator import HortatorClient, AsyncHortatorClient
from hortator.exceptions import AuthenticationError, HortatorError
from hortator.models import RunResult

BASE = "https://hortator.test"

COMPLETION_RESPONSE = {
    "id": "chatcmpl-123",
    "model": "hortator/researcher",
    "choices": [
        {
            "message": {"role": "assistant", "content": "Hello world"},
            "finish_reason": "stop",
        }
    ],
    "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
}

MODELS_RESPONSE = {
    "data": [
        {"id": "hortator/researcher", "owned_by": "hortator", "created": 0},
        {"id": "hortator/tech-lead", "owned_by": "hortator", "created": 0},
    ]
}


def test_missing_base_url():
    with pytest.raises(ValueError):
        HortatorClient(base_url="", api_key="key")


@respx.mock
def test_run():
    respx.post(f"{BASE}/v1/chat/completions").mock(
        return_value=httpx.Response(200, json=COMPLETION_RESPONSE)
    )
    client = HortatorClient(base_url=BASE, api_key="test-key")
    result = client.run("hello", role="researcher")
    assert isinstance(result, RunResult)
    assert result.content == "Hello world"
    assert result.usage.total_tokens == 15
    assert result.finish_reason == "stop"
    assert result.model == "hortator/researcher"


@respx.mock
def test_chat():
    respx.post(f"{BASE}/v1/chat/completions").mock(
        return_value=httpx.Response(200, json=COMPLETION_RESPONSE)
    )
    client = HortatorClient(base_url=BASE, api_key="test-key")
    result = client.chat(
        messages=[
            {"role": "system", "content": "You are helpful."},
            {"role": "user", "content": "Hi"},
        ],
        role="tech-lead",
    )
    assert result.content == "Hello world"


@respx.mock
def test_list_models():
    respx.get(f"{BASE}/v1/models").mock(
        return_value=httpx.Response(200, json=MODELS_RESPONSE)
    )
    client = HortatorClient(base_url=BASE, api_key="test-key")
    models = client.list_models()
    assert len(models) == 2
    assert models[0].id == "hortator/researcher"


@respx.mock
def test_auth_error():
    respx.post(f"{BASE}/v1/chat/completions").mock(
        return_value=httpx.Response(401, text="Unauthorized")
    )
    client = HortatorClient(base_url=BASE, api_key="bad-key")
    with pytest.raises(AuthenticationError):
        client.run("hello", role="researcher")


@respx.mock
def test_stream():
    sse_body = (
        'data: {"choices":[{"delta":{"content":"Hello"}}]}\n\n'
        'data: {"choices":[{"delta":{"content":" world"}}]}\n\n'
        "data: [DONE]\n\n"
    )
    respx.post(f"{BASE}/v1/chat/completions").mock(
        return_value=httpx.Response(
            200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
    )
    client = HortatorClient(base_url=BASE, api_key="test-key")
    chunks = list(client.stream("hello", role="researcher"))
    assert len(chunks) == 2
    assert chunks[0].content == "Hello"
    assert chunks[1].content == " world"


@pytest.mark.asyncio
@respx.mock
async def test_async_run():
    respx.post(f"{BASE}/v1/chat/completions").mock(
        return_value=httpx.Response(200, json=COMPLETION_RESPONSE)
    )
    client = AsyncHortatorClient(base_url=BASE, api_key="test-key")
    result = await client.run("hello", role="researcher")
    assert result.content == "Hello world"
    await client.close()
