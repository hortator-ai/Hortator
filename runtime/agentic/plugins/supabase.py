"""
Supabase plugin for the agentic runtime.

Provides database operations via the Supabase REST API.
"""

import httpx

from plugins import ToolExecutionError

CAPABILITY = "supabase"

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "supabase_execute_sql",
            "description": "Execute a SQL query against the Supabase database.",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "The SQL query to execute.",
                    },
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "supabase_list_tables",
            "description": "List all tables in the Supabase database.",
            "parameters": {
                "type": "object",
                "properties": {},
            },
        },
    },
]


def _get_config(env: dict) -> tuple[str, str]:
    """Get Supabase config from environment, raising if not set."""
    url = env.get("SUPABASE_URL")
    key = env.get("SUPABASE_SERVICE_KEY")

    if not url:
        raise ToolExecutionError("SUPABASE_URL environment variable is not set")
    if not key:
        raise ToolExecutionError("SUPABASE_SERVICE_KEY environment variable is not set")

    return url, key


def _make_request(base_url: str, service_key: str, query: str) -> dict:
    """Execute a SQL query via the Supabase REST SQL endpoint."""
    headers = {
        "apikey": service_key,
        "Authorization": f"Bearer {service_key}",
        "Content-Type": "application/json",
    }

    # Use the REST SQL endpoint
    url = f"{base_url}/rest/v1/rpc/exec_sql"

    try:
        with httpx.Client(timeout=30) as client:
            response = client.post(
                url,
                headers=headers,
                json={"query": query},
            )

        if response.status_code >= 400:
            return {"error": response.text, "code": response.status_code}

        return {"result": response.json()}

    except httpx.RequestError as e:
        return {"error": str(e), "code": 0}


def execute(name: str, args: dict, env: dict) -> dict:
    """Execute a Supabase tool call."""
    base_url, service_key = _get_config(env)

    match name:
        case "supabase_execute_sql":
            query = args.get("query", "")
            if not query:
                return {"error": "query is required", "code": 0}
            return _make_request(base_url, service_key, query)

        case "supabase_list_tables":
            query = """
                SELECT table_name
                FROM information_schema.tables
                WHERE table_schema = 'public'
                ORDER BY table_name
            """
            return _make_request(base_url, service_key, query)

        case _:
            return {"error": f"Unknown tool: {name}", "code": 0}
