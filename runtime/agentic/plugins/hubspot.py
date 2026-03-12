"""
HubSpot plugin for the agentic runtime.

Provides CRM operations via the HubSpot API.
"""

import httpx

from plugins import ToolExecutionError

CAPABILITY = "hubspot"

BASE_URL = "https://api.hubapi.com"

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "hubspot_get_contact",
            "description": "Get a contact by ID from HubSpot CRM.",
            "parameters": {
                "type": "object",
                "properties": {
                    "contact_id": {
                        "type": "string",
                        "description": "The HubSpot contact ID.",
                    },
                },
                "required": ["contact_id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "hubspot_update_contact",
            "description": "Update a contact's properties in HubSpot CRM.",
            "parameters": {
                "type": "object",
                "properties": {
                    "contact_id": {
                        "type": "string",
                        "description": "The HubSpot contact ID.",
                    },
                    "properties": {
                        "type": "object",
                        "description": "Properties to update as key-value pairs.",
                    },
                },
                "required": ["contact_id", "properties"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "hubspot_get_company",
            "description": "Get a company by ID from HubSpot CRM.",
            "parameters": {
                "type": "object",
                "properties": {
                    "company_id": {
                        "type": "string",
                        "description": "The HubSpot company ID.",
                    },
                },
                "required": ["company_id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "hubspot_update_company",
            "description": "Update a company's properties in HubSpot CRM.",
            "parameters": {
                "type": "object",
                "properties": {
                    "company_id": {
                        "type": "string",
                        "description": "The HubSpot company ID.",
                    },
                    "properties": {
                        "type": "object",
                        "description": "Properties to update as key-value pairs.",
                    },
                },
                "required": ["company_id", "properties"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "hubspot_get_deal",
            "description": "Get a deal by ID from HubSpot CRM.",
            "parameters": {
                "type": "object",
                "properties": {
                    "deal_id": {
                        "type": "string",
                        "description": "The HubSpot deal ID.",
                    },
                },
                "required": ["deal_id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "hubspot_search",
            "description": "Search for objects in HubSpot CRM.",
            "parameters": {
                "type": "object",
                "properties": {
                    "object_type": {
                        "type": "string",
                        "enum": ["contacts", "companies", "deals", "tickets"],
                        "description": "The object type to search.",
                    },
                    "filter_groups": {
                        "type": "array",
                        "description": "Filter groups for the search query.",
                        "items": {"type": "object"},
                    },
                    "properties": {
                        "type": "array",
                        "description": "Properties to return in results.",
                        "items": {"type": "string"},
                    },
                },
                "required": ["object_type", "filter_groups", "properties"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "hubspot_get_properties",
            "description": "Get available properties for an object type.",
            "parameters": {
                "type": "object",
                "properties": {
                    "object_type": {
                        "type": "string",
                        "enum": ["contacts", "companies", "deals", "tickets"],
                        "description": "The object type to get properties for.",
                    },
                },
                "required": ["object_type"],
            },
        },
    },
]


def _get_api_key(env: dict) -> str:
    """Get API key from environment, raising if not set."""
    api_key = env.get("HUBSPOT_API_KEY")
    if not api_key:
        raise ToolExecutionError("HUBSPOT_API_KEY environment variable is not set")
    return api_key


def _make_request(
    method: str,
    endpoint: str,
    api_key: str,
    json_body: dict | None = None,
) -> dict:
    """Make an authenticated request to the HubSpot API."""
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    url = f"{BASE_URL}{endpoint}"

    try:
        with httpx.Client(timeout=30) as client:
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
    """Execute a HubSpot tool call."""
    api_key = _get_api_key(env)

    match name:
        case "hubspot_get_contact":
            contact_id = args.get("contact_id", "")
            if not contact_id:
                return {"error": "contact_id is required", "code": 0}
            return _make_request("GET", f"/crm/v3/objects/contacts/{contact_id}", api_key)

        case "hubspot_update_contact":
            contact_id = args.get("contact_id", "")
            properties = args.get("properties", {})
            if not contact_id:
                return {"error": "contact_id is required", "code": 0}
            return _make_request(
                "PATCH",
                f"/crm/v3/objects/contacts/{contact_id}",
                api_key,
                {"properties": properties},
            )

        case "hubspot_get_company":
            company_id = args.get("company_id", "")
            if not company_id:
                return {"error": "company_id is required", "code": 0}
            return _make_request("GET", f"/crm/v3/objects/companies/{company_id}", api_key)

        case "hubspot_update_company":
            company_id = args.get("company_id", "")
            properties = args.get("properties", {})
            if not company_id:
                return {"error": "company_id is required", "code": 0}
            return _make_request(
                "PATCH",
                f"/crm/v3/objects/companies/{company_id}",
                api_key,
                {"properties": properties},
            )

        case "hubspot_get_deal":
            deal_id = args.get("deal_id", "")
            if not deal_id:
                return {"error": "deal_id is required", "code": 0}
            return _make_request("GET", f"/crm/v3/objects/deals/{deal_id}", api_key)

        case "hubspot_search":
            object_type = args.get("object_type", "")
            filter_groups = args.get("filter_groups", [])
            properties = args.get("properties", [])
            if not object_type:
                return {"error": "object_type is required", "code": 0}
            return _make_request(
                "POST",
                f"/crm/v3/objects/{object_type}/search",
                api_key,
                {"filterGroups": filter_groups, "properties": properties},
            )

        case "hubspot_get_properties":
            object_type = args.get("object_type", "")
            if not object_type:
                return {"error": "object_type is required", "code": 0}
            return _make_request("GET", f"/crm/v3/properties/{object_type}", api_key)

        case _:
            return {"error": f"Unknown tool: {name}", "code": 0}
