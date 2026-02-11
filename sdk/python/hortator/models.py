"""Pydantic models for Hortator SDK."""

from __future__ import annotations

import base64
from pathlib import Path
from typing import Optional, Union

from pydantic import BaseModel


class Budget(BaseModel):
    max_cost_usd: Optional[str] = None
    max_tokens: Optional[int] = None


class FileContent(BaseModel):
    """A file attachment for message content."""
    file_data: str  # base64-encoded content
    filename: str


class ContentPart(BaseModel):
    """A typed content part (text or file)."""
    type: str  # "text" or "file"
    text: Optional[str] = None
    file: Optional[FileContent] = None

    @classmethod
    def text_part(cls, text: str) -> ContentPart:
        return cls(type="text", text=text)

    @classmethod
    def file_part(cls, filename: str, data: bytes) -> ContentPart:
        return cls(type="file", file=FileContent(
            filename=filename,
            file_data=base64.b64encode(data).decode(),
        ))

    @classmethod
    def from_path(cls, path: str | Path) -> ContentPart:
        """Create a file part from a file path."""
        p = Path(path)
        return cls.file_part(p.name, p.read_bytes())


class Message(BaseModel):
    role: str
    content: Union[str, list[ContentPart]]

    def text(self) -> str:
        """Return the text content of the message."""
        if isinstance(self.content, str):
            return self.content
        return "\n".join(p.text or "" for p in self.content if p.type == "text")

    def files(self) -> list[FileContent]:
        """Return all file parts from the content."""
        if isinstance(self.content, str):
            return []
        return [p.file for p in self.content if p.type == "file" and p.file]


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
