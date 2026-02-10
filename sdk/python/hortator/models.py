"""Pydantic models for Hortator SDK."""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel


class Budget(BaseModel):
    max_cost_usd: Optional[str] = None
    max_tokens: Optional[int] = None


class Message(BaseModel):
    role: str
    content: str


class Usage(BaseModel):
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


class RunResult(BaseModel):
    """Result from a blocking run() call."""

    id: str
    content: str
    finish_reason: str
    usage: Usage
    model: str


class StreamChunk(BaseModel):
    """A single chunk from a streaming response."""

    content: str = ""
    finish_reason: Optional[str] = None
    usage: Optional[Usage] = None


class ModelInfo(BaseModel):
    id: str
    owned_by: str = "hortator"
    created: int = 0
