"""Tests for SSE stream parser."""

import json
from unittest.mock import MagicMock

from hortator._streaming import iter_sse_events


def _make_response(lines: list[str]):
    """Create a mock httpx response with iter_lines."""
    resp = MagicMock()
    resp.iter_lines.return_value = iter(lines)
    return resp


def test_valid_events():
    lines = [
        'data: {"choices":[{"delta":{"content":"A"}}]}',
        'data: {"choices":[{"delta":{"content":"B"}}]}',
        "data: [DONE]",
    ]
    events = list(iter_sse_events(_make_response(lines)))
    assert len(events) == 2
    assert events[0]["choices"][0]["delta"]["content"] == "A"
    assert events[1]["choices"][0]["delta"]["content"] == "B"


def test_done_terminates():
    lines = [
        'data: {"id":"1"}',
        "data: [DONE]",
        'data: {"id":"2"}',  # should not be yielded
    ]
    events = list(iter_sse_events(_make_response(lines)))
    assert len(events) == 1


def test_empty_lines_ignored():
    lines = [
        "",
        'data: {"id":"1"}',
        "",
        "",
        "data: [DONE]",
    ]
    events = list(iter_sse_events(_make_response(lines)))
    assert len(events) == 1


def test_no_events():
    events = list(iter_sse_events(_make_response([])))
    assert events == []
