"""SSE stream parser for httpx responses."""

from __future__ import annotations

import json
from typing import Any, Generator, AsyncGenerator

import httpx


def iter_sse_events(response: httpx.Response) -> Generator[dict[str, Any], None, None]:
    """Yield parsed SSE data events from an httpx streaming response."""
    for line in response.iter_lines():
        if line.startswith("data: "):
            data = line[6:]
            if data == "[DONE]":
                return
            yield json.loads(data)


async def aiter_sse_events(response: httpx.Response) -> AsyncGenerator[dict[str, Any], None]:
    """Yield parsed SSE data events from an async httpx streaming response."""
    async for line in response.aiter_lines():
        if line.startswith("data: "):
            data = line[6:]
            if data == "[DONE]":
                return
            yield json.loads(data)
