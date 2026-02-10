"""Hortator sync and async clients."""

from __future__ import annotations

from typing import Any, Generator, AsyncGenerator, Optional

import httpx

from . import __version__
from .models import Budget, RunResult, StreamChunk, ModelInfo, Usage
from .exceptions import HortatorError, AuthenticationError, RateLimitError
from ._streaming import iter_sse_events, aiter_sse_events

_USER_AGENT = f"hortator-python/{__version__}"


def _check_response(resp: httpx.Response) -> None:
    if resp.status_code == 401:
        raise AuthenticationError(f"Authentication failed: {resp.text}")
    if resp.status_code == 429:
        raise RateLimitError(f"Rate limited: {resp.text}")
    if resp.status_code >= 400:
        raise HortatorError(f"HTTP {resp.status_code}: {resp.text}")


def _build_body(
    messages: list[dict[str, str]],
    model: str,
    stream: bool,
    capabilities: list[str] | None,
    tier: str | None,
    budget: dict[str, Any] | None,
) -> dict[str, Any]:
    body: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "stream": stream,
    }
    if capabilities:
        body["x_capabilities"] = capabilities
    if tier:
        body["x_tier"] = tier
    if budget:
        body["x_budget"] = budget
    return body


def _parse_run_result(data: dict[str, Any]) -> RunResult:
    choice = data["choices"][0]
    usage_data = data.get("usage", {})
    return RunResult(
        id=data.get("id", ""),
        content=choice["message"]["content"],
        finish_reason=choice.get("finish_reason", "stop"),
        usage=Usage(**usage_data),
        model=data.get("model", ""),
    )


def _parse_stream_chunk(data: dict[str, Any]) -> StreamChunk:
    delta = data.get("choices", [{}])[0].get("delta", {})
    finish = data.get("choices", [{}])[0].get("finish_reason")
    usage_data = data.get("usage")
    return StreamChunk(
        content=delta.get("content", ""),
        finish_reason=finish,
        usage=Usage(**usage_data) if usage_data else None,
    )


class HortatorClient:
    """Synchronous Hortator client."""

    def __init__(self, base_url: str, api_key: str, timeout: float = 300.0):
        if not base_url:
            raise ValueError("base_url is required")
        self._base_url = base_url.rstrip("/")
        self._client = httpx.Client(
            base_url=self._base_url,
            headers={
                "Authorization": f"Bearer {api_key}",
                "User-Agent": _USER_AGENT,
            },
            timeout=timeout,
        )

    def run(
        self,
        prompt: str,
        role: str = "legionary",
        capabilities: list[str] | None = None,
        tier: str | None = None,
        budget: dict[str, Any] | None = None,
    ) -> RunResult:
        messages = [{"role": "user", "content": prompt}]
        return self.chat(messages, role=role, capabilities=capabilities, tier=tier, budget=budget)

    def chat(
        self,
        messages: list[dict[str, str]],
        role: str = "legionary",
        capabilities: list[str] | None = None,
        tier: str | None = None,
        budget: dict[str, Any] | None = None,
    ) -> RunResult:
        body = _build_body(messages, f"hortator/{role}", False, capabilities, tier, budget)
        resp = self._client.post("/v1/chat/completions", json=body)
        _check_response(resp)
        return _parse_run_result(resp.json())

    def stream(
        self,
        prompt: str,
        role: str = "legionary",
        capabilities: list[str] | None = None,
        tier: str | None = None,
        budget: dict[str, Any] | None = None,
    ) -> Generator[StreamChunk, None, None]:
        messages = [{"role": "user", "content": prompt}]
        body = _build_body(messages, f"hortator/{role}", True, capabilities, tier, budget)
        with self._client.stream("POST", "/v1/chat/completions", json=body) as resp:
            _check_response(resp)
            for event in iter_sse_events(resp):
                yield _parse_stream_chunk(event)

    def list_models(self) -> list[ModelInfo]:
        resp = self._client.get("/v1/models")
        _check_response(resp)
        data = resp.json()
        return [ModelInfo(**m) for m in data.get("data", [])]

    def close(self) -> None:
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()


class AsyncHortatorClient:
    """Asynchronous Hortator client."""

    def __init__(self, base_url: str, api_key: str, timeout: float = 300.0):
        if not base_url:
            raise ValueError("base_url is required")
        self._base_url = base_url.rstrip("/")
        self._client = httpx.AsyncClient(
            base_url=self._base_url,
            headers={
                "Authorization": f"Bearer {api_key}",
                "User-Agent": _USER_AGENT,
            },
            timeout=timeout,
        )

    async def run(
        self,
        prompt: str,
        role: str = "legionary",
        capabilities: list[str] | None = None,
        tier: str | None = None,
        budget: dict[str, Any] | None = None,
    ) -> RunResult:
        messages = [{"role": "user", "content": prompt}]
        return await self.chat(messages, role=role, capabilities=capabilities, tier=tier, budget=budget)

    async def chat(
        self,
        messages: list[dict[str, str]],
        role: str = "legionary",
        capabilities: list[str] | None = None,
        tier: str | None = None,
        budget: dict[str, Any] | None = None,
    ) -> RunResult:
        body = _build_body(messages, f"hortator/{role}", False, capabilities, tier, budget)
        resp = await self._client.post("/v1/chat/completions", json=body)
        _check_response(resp)
        return _parse_run_result(resp.json())

    async def stream(
        self,
        prompt: str,
        role: str = "legionary",
        capabilities: list[str] | None = None,
        tier: str | None = None,
        budget: dict[str, Any] | None = None,
    ) -> AsyncGenerator[StreamChunk, None]:
        messages = [{"role": "user", "content": prompt}]
        body = _build_body(messages, f"hortator/{role}", True, capabilities, tier, budget)
        async with self._client.stream("POST", "/v1/chat/completions", json=body) as resp:
            _check_response(resp)
            async for event in aiter_sse_events(resp):
                yield _parse_stream_chunk(event)

    async def list_models(self) -> list[ModelInfo]:
        resp = await self._client.get("/v1/models")
        _check_response(resp)
        data = resp.json()
        return [ModelInfo(**m) for m in data.get("data", [])]

    async def close(self) -> None:
        await self._client.aclose()

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        await self.close()
