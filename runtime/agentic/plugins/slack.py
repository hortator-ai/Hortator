"""
Slack plugin for the agentic runtime.

Provides Slack messaging and channel operations.
"""

import httpx

from plugins import ToolExecutionError

CAPABILITY = "slack"

BASE_URL = "https://slack.com/api"

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "slack_send_message",
            "description": "Send a message to a Slack channel.",
            "parameters": {
                "type": "object",
                "properties": {
                    "channel": {
                        "type": "string",
                        "description": "Channel ID or name (e.g., '#general' or 'C1234567890').",
                    },
                    "text": {
                        "type": "string",
                        "description": "The message text to send.",
                    },
                    "thread_ts": {
                        "type": "string",
                        "description": "Optional thread timestamp to reply in a thread.",
                    },
                },
                "required": ["channel", "text"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "slack_read_channel",
            "description": "Read recent messages from a Slack channel.",
            "parameters": {
                "type": "object",
                "properties": {
                    "channel": {
                        "type": "string",
                        "description": "Channel ID (e.g., 'C1234567890').",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum number of messages to return (default: 20).",
                    },
                },
                "required": ["channel"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "slack_read_thread",
            "description": "Read messages from a Slack thread.",
            "parameters": {
                "type": "object",
                "properties": {
                    "channel": {
                        "type": "string",
                        "description": "Channel ID where the thread is located.",
                    },
                    "thread_ts": {
                        "type": "string",
                        "description": "Thread timestamp to read.",
                    },
                },
                "required": ["channel", "thread_ts"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "slack_search_channels",
            "description": "Search for channels by name.",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search query to filter channels by name.",
                    },
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "slack_search_users",
            "description": "Search for users by name or email.",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search query to filter users by name or email.",
                    },
                },
                "required": ["query"],
            },
        },
    },
]


def _get_token(env: dict) -> str:
    """Get bot token from environment, raising if not set."""
    token = env.get("SLACK_BOT_TOKEN")
    if not token:
        raise ToolExecutionError("SLACK_BOT_TOKEN environment variable is not set")
    return token


def _make_request(method: str, token: str, json_body: dict) -> dict:
    """Make an authenticated request to the Slack API."""
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
    }

    url = f"{BASE_URL}/{method}"

    try:
        with httpx.Client(timeout=30) as client:
            response = client.post(url, headers=headers, json=json_body)

        data = response.json()

        # Slack API returns ok: true/false in the response body
        if not data.get("ok"):
            return {"error": data.get("error", "Unknown error"), "code": 0}

        return {"result": data}

    except httpx.RequestError as e:
        return {"error": str(e), "code": 0}


def execute(name: str, args: dict, env: dict) -> dict:
    """Execute a Slack tool call."""
    token = _get_token(env)

    match name:
        case "slack_send_message":
            channel = args.get("channel", "")
            text = args.get("text", "")
            thread_ts = args.get("thread_ts")

            if not channel:
                return {"error": "channel is required", "code": 0}
            if not text:
                return {"error": "text is required", "code": 0}

            body = {"channel": channel, "text": text}
            if thread_ts:
                body["thread_ts"] = thread_ts

            return _make_request("chat.postMessage", token, body)

        case "slack_read_channel":
            channel = args.get("channel", "")
            limit = args.get("limit", 20)

            if not channel:
                return {"error": "channel is required", "code": 0}

            return _make_request(
                "conversations.history",
                token,
                {"channel": channel, "limit": limit},
            )

        case "slack_read_thread":
            channel = args.get("channel", "")
            thread_ts = args.get("thread_ts", "")

            if not channel:
                return {"error": "channel is required", "code": 0}
            if not thread_ts:
                return {"error": "thread_ts is required", "code": 0}

            return _make_request(
                "conversations.replies",
                token,
                {"channel": channel, "ts": thread_ts},
            )

        case "slack_search_channels":
            query = args.get("query", "").lower()
            if not query:
                return {"error": "query is required", "code": 0}

            # Get all channels and filter by name
            result = _make_request("conversations.list", token, {"limit": 200})
            if "error" in result:
                return result

            channels = result.get("result", {}).get("channels", [])
            filtered = [
                ch for ch in channels
                if query in ch.get("name", "").lower()
            ]
            return {"result": {"channels": filtered}}

        case "slack_search_users":
            query = args.get("query", "").lower()
            if not query:
                return {"error": "query is required", "code": 0}

            # Get all users and filter by name/email
            result = _make_request("users.list", token, {"limit": 200})
            if "error" in result:
                return result

            members = result.get("result", {}).get("members", [])
            filtered = [
                m for m in members
                if query in m.get("name", "").lower()
                or query in m.get("real_name", "").lower()
                or query in m.get("profile", {}).get("email", "").lower()
            ]
            return {"result": {"members": filtered}}

        case _:
            return {"error": f"Unknown tool: {name}", "code": 0}
