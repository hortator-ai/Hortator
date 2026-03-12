"""
Notion plugin for the agentic runtime.

Provides Notion search and page operations.
"""

import httpx

from plugins import ToolExecutionError

CAPABILITY = "notion"

BASE_URL = "https://api.notion.com"
NOTION_VERSION = "2022-06-28"

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "notion_search",
            "description": "Search for pages and databases in Notion.",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "The search query string.",
                    },
                    "filter_type": {
                        "type": "string",
                        "enum": ["page", "database"],
                        "description": "Optional filter to only return pages or databases.",
                    },
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "notion_fetch_page",
            "description": "Fetch a page by ID from Notion.",
            "parameters": {
                "type": "object",
                "properties": {
                    "page_id": {
                        "type": "string",
                        "description": "The Notion page ID.",
                    },
                },
                "required": ["page_id"],
            },
        },
    },
]


def _get_api_key(env: dict) -> str:
    """Get API key from environment, raising if not set."""
    api_key = env.get("NOTION_API_KEY")
    if not api_key:
        raise ToolExecutionError("NOTION_API_KEY environment variable is not set")
    return api_key


def _make_request(
    method: str,
    endpoint: str,
    api_key: str,
    json_body: dict | None = None,
) -> dict:
    """Make an authenticated request to the Notion API."""
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
        "Notion-Version": NOTION_VERSION,
    }

    url = f"{BASE_URL}{endpoint}"

    try:
        with httpx.Client(timeout=30) as client:
            if method == "GET":
                response = client.get(url, headers=headers)
            else:
                response = client.request(
                    method=method,
                    url=url,
                    headers=headers,
                    json=json_body,
                )

        if response.status_code >= 400:
            return {"error": response.text, "code": response.status_code}

        return {"result": response.json()}

    except httpx.RequestError as e:
        return {"error": str(e), "code": 0}


def execute(name: str, args: dict, env: dict) -> dict:
    """Execute a Notion tool call."""
    api_key = _get_api_key(env)

    match name:
        case "notion_search":
            query = args.get("query", "")
            filter_type = args.get("filter_type")

            if not query:
                return {"error": "query is required", "code": 0}

            body: dict = {"query": query}
            if filter_type:
                body["filter"] = {"value": filter_type, "property": "object"}

            return _make_request("POST", "/v1/search", api_key, body)

        case "notion_fetch_page":
            page_id = args.get("page_id", "")
            if not page_id:
                return {"error": "page_id is required", "code": 0}

            return _make_request("GET", f"/v1/pages/{page_id}", api_key)

        case _:
            return {"error": f"Unknown tool: {name}", "code": 0}
