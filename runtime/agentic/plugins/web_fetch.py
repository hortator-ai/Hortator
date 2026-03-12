"""
Web fetch plugin for the agentic runtime.

Provides HTTP request capabilities with HTML content extraction.
"""

import httpx
from bs4 import BeautifulSoup

from plugins import ToolExecutionError

CAPABILITY = "web-fetch"

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "web_fetch",
            "description": (
                "Fetch content from a URL. For HTML responses, extracts readable text. "
                "Supports GET, POST, PUT, PATCH, DELETE methods."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "The URL to fetch.",
                    },
                    "method": {
                        "type": "string",
                        "enum": ["GET", "POST", "PUT", "PATCH", "DELETE"],
                        "description": "HTTP method (default: GET).",
                    },
                    "headers": {
                        "type": "object",
                        "description": "Optional HTTP headers as key-value pairs.",
                    },
                    "body": {
                        "type": "string",
                        "description": "Optional request body (for POST/PUT/PATCH).",
                    },
                    "timeout": {
                        "type": "integer",
                        "description": "Timeout in seconds (default: 30).",
                    },
                },
                "required": ["url"],
            },
        },
    },
]

MAX_RESPONSE_LENGTH = 50000


def execute(name: str, args: dict, env: dict) -> dict:
    """Execute a web_fetch tool call."""
    if name != "web_fetch":
        return {"error": f"Unknown tool: {name}", "code": 0}

    url = args.get("url", "")
    if not url:
        return {"error": "url is required", "code": 0}

    method = args.get("method", "GET").upper()
    headers = args.get("headers", {})
    body = args.get("body")
    timeout = args.get("timeout", 30)

    try:
        with httpx.Client(follow_redirects=True, timeout=timeout) as client:
            response = client.request(
                method=method,
                url=url,
                headers=headers,
                content=body,
            )

        content_type = response.headers.get("content-type", "")
        body_text = response.text

        # For HTML responses, extract readable text
        if "text/html" in content_type:
            body_text = _extract_text_from_html(body_text)

        # Truncate if too long
        truncated = False
        if len(body_text) > MAX_RESPONSE_LENGTH:
            body_text = body_text[:MAX_RESPONSE_LENGTH]
            truncated = True

        return {
            "result": {
                "status_code": response.status_code,
                "content_type": content_type,
                "body": body_text,
                "truncated": truncated,
            }
        }

    except httpx.TimeoutException:
        return {"error": f"Request timed out after {timeout}s", "code": 408}

    except httpx.RequestError as e:
        return {"error": str(e), "code": 0}


def _extract_text_from_html(html: str) -> str:
    """Extract readable text from HTML, stripping non-content elements."""
    soup = BeautifulSoup(html, "html.parser")

    # Remove script, style, nav, footer elements
    for tag in soup(["script", "style", "nav", "footer", "header", "aside"]):
        tag.decompose()

    # Get text with reasonable spacing
    text = soup.get_text(separator="\n", strip=True)

    # Collapse multiple newlines
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    return "\n".join(lines)
