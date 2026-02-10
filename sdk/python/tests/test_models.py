"""Tests for Hortator models."""

from hortator.models import RunResult, StreamChunk, Budget, Usage


def test_run_result_from_dict():
    r = RunResult(
        id="123",
        content="hello",
        finish_reason="stop",
        usage=Usage(prompt_tokens=10, completion_tokens=5, total_tokens=15),
        model="hortator/researcher",
    )
    assert r.content == "hello"
    assert r.usage.total_tokens == 15


def test_stream_chunk_defaults():
    c = StreamChunk()
    assert c.content == ""
    assert c.finish_reason is None
    assert c.usage is None


def test_stream_chunk_with_data():
    c = StreamChunk(content="hi", finish_reason="stop")
    assert c.content == "hi"
    assert c.finish_reason == "stop"


def test_budget_serialization():
    b = Budget(max_cost_usd="1.00", max_tokens=1000)
    d = b.model_dump()
    assert d["max_cost_usd"] == "1.00"
    assert d["max_tokens"] == 1000


def test_budget_defaults():
    b = Budget()
    assert b.max_cost_usd is None
    assert b.max_tokens is None
