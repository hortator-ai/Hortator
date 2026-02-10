"""Exceptions for Hortator SDK."""


class HortatorError(Exception):
    """Base exception for Hortator SDK."""


class AuthenticationError(HortatorError):
    """Raised on 401 responses."""


class TaskError(HortatorError):
    """Raised when a task fails."""

    def __init__(self, message: str, phase: str = ""):
        super().__init__(message)
        self.phase = phase


class RateLimitError(HortatorError):
    """Raised on 429 responses."""
