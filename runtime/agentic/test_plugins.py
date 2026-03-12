"""Comprehensive tests for the plugin system."""

import os
import unittest
from unittest.mock import MagicMock, patch

from plugins import ToolExecutionError


class TestPluginLoader(unittest.TestCase):
    """Tests for plugin_loader.py functions."""

    def test_load_plugins_only_loads_matching_capabilities(self):
        """load_plugins only loads plugins whose CAPABILITY is in capabilities list."""
        from plugin_loader import load_plugins

        # Load with web-fetch capability only
        plugins = load_plugins(["web-fetch"])
        self.assertIn("web-fetch", plugins)
        self.assertNotIn("hubspot", plugins)
        self.assertNotIn("slack", plugins)

    def test_load_plugins_returns_empty_for_no_matching_capabilities(self):
        """load_plugins returns empty dict if no matching capabilities."""
        from plugin_loader import load_plugins

        plugins = load_plugins(["nonexistent-capability"])
        self.assertEqual(plugins, {})

    def test_load_plugins_loads_multiple_capabilities(self):
        """load_plugins loads all matching plugins."""
        from plugin_loader import load_plugins

        plugins = load_plugins(["web-fetch", "hubspot", "slack"])
        self.assertIn("web-fetch", plugins)
        self.assertIn("hubspot", plugins)
        self.assertIn("slack", plugins)

    def test_get_plugin_tools_returns_correct_schemas(self):
        """get_plugin_tools returns correct tool schemas for active plugins."""
        from plugin_loader import get_plugin_tools

        tools = get_plugin_tools(["web-fetch"])
        tool_names = [t["function"]["name"] for t in tools]
        self.assertIn("web_fetch", tool_names)

    def test_get_plugin_tools_combines_multiple_plugins(self):
        """get_plugin_tools combines tools from multiple plugins."""
        from plugin_loader import get_plugin_tools

        tools = get_plugin_tools(["web-fetch", "notion"])
        tool_names = [t["function"]["name"] for t in tools]
        self.assertIn("web_fetch", tool_names)
        self.assertIn("notion_search", tool_names)
        self.assertIn("notion_fetch_page", tool_names)

    def test_dispatch_plugin_tool_returns_none_for_unknown_tool(self):
        """dispatch_plugin_tool returns None for unknown tool names."""
        from plugin_loader import dispatch_plugin_tool

        result = dispatch_plugin_tool(
            "nonexistent_tool",
            {},
            ["web-fetch"],
            {},
        )
        self.assertIsNone(result)

    def test_dispatch_plugin_tool_routes_to_correct_plugin(self):
        """dispatch_plugin_tool routes to correct plugin."""
        from plugin_loader import dispatch_plugin_tool

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.text = "Hello"
            mock_response.status_code = 200
            mock_response.headers = {"content-type": "text/plain"}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = dispatch_plugin_tool(
                "web_fetch",
                {"url": "https://example.com"},
                ["web-fetch"],
                {},
            )

            self.assertIsNotNone(result)
            self.assertIn("result", result)


class TestWebFetchPlugin(unittest.TestCase):
    """Tests for web_fetch.py plugin."""

    def test_web_fetch_returns_correct_structure(self):
        """GET request returns {status_code, content_type, body, truncated: False}."""
        from plugins.web_fetch import execute

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.text = "Hello World"
            mock_response.status_code = 200
            mock_response.headers = {"content-type": "text/plain"}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute("web_fetch", {"url": "https://example.com"}, {})

            self.assertIn("result", result)
            self.assertEqual(result["result"]["status_code"], 200)
            self.assertEqual(result["result"]["content_type"], "text/plain")
            self.assertEqual(result["result"]["body"], "Hello World")
            self.assertFalse(result["result"]["truncated"])

    def test_web_fetch_html_extracts_text(self):
        """HTML response strips tags, extracts text."""
        from plugins.web_fetch import execute

        html = """
        <html>
        <head><script>var x = 1;</script><style>.foo{}</style></head>
        <body>
            <nav>Nav content</nav>
            <main>Main content here</main>
            <footer>Footer</footer>
        </body>
        </html>
        """

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.text = html
            mock_response.status_code = 200
            mock_response.headers = {"content-type": "text/html"}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute("web_fetch", {"url": "https://example.com"}, {})

            self.assertIn("result", result)
            # Nav, footer, script, style should be removed
            self.assertNotIn("Nav content", result["result"]["body"])
            self.assertNotIn("Footer", result["result"]["body"])
            self.assertNotIn("var x", result["result"]["body"])
            self.assertIn("Main content here", result["result"]["body"])

    def test_web_fetch_truncates_large_responses(self):
        """Response over 50,000 chars is truncated (truncated: True)."""
        from plugins.web_fetch import execute

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.text = "x" * 60000
            mock_response.status_code = 200
            mock_response.headers = {"content-type": "text/plain"}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute("web_fetch", {"url": "https://example.com"}, {})

            self.assertIn("result", result)
            self.assertTrue(result["result"]["truncated"])
            self.assertEqual(len(result["result"]["body"]), 50000)

    def test_web_fetch_timeout_returns_error(self):
        """Timeout returns {"error": ..., "code": 408}."""
        from plugins.web_fetch import execute
        import httpx

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_client.return_value.__enter__.return_value.request.side_effect = (
                httpx.TimeoutException("timeout")
            )

            result = execute("web_fetch", {"url": "https://example.com"}, {})

            self.assertIn("error", result)
            self.assertEqual(result["code"], 408)

    def test_web_fetch_request_error_returns_error(self):
        """Request error returns {"error": ..., "code": 0}."""
        from plugins.web_fetch import execute
        import httpx

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_client.return_value.__enter__.return_value.request.side_effect = (
                httpx.RequestError("connection failed")
            )

            result = execute("web_fetch", {"url": "https://example.com"}, {})

            self.assertIn("error", result)
            self.assertEqual(result["code"], 0)


class TestHubSpotPlugin(unittest.TestCase):
    """Tests for hubspot.py plugin."""

    def test_hubspot_get_contact_calls_correct_url(self):
        """hubspot_get_contact calls correct URL with auth header."""
        from plugins.hubspot import execute

        with patch("plugins.hubspot.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = {"id": "123", "properties": {}}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute(
                "hubspot_get_contact",
                {"contact_id": "123"},
                {"HUBSPOT_API_KEY": "test-key"},
            )

            mock_client.return_value.__enter__.return_value.request.assert_called_once()
            call_args = mock_client.return_value.__enter__.return_value.request.call_args
            self.assertEqual(call_args.kwargs["method"], "GET")
            self.assertIn("/crm/v3/objects/contacts/123", call_args.kwargs["url"])
            self.assertEqual(
                call_args.kwargs["headers"]["Authorization"],
                "Bearer test-key",
            )
            self.assertIn("result", result)

    def test_hubspot_search_sends_correct_post_body(self):
        """hubspot_search sends correct POST body."""
        from plugins.hubspot import execute

        with patch("plugins.hubspot.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = {"results": []}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            filter_groups = [{"filters": [{"propertyName": "email", "value": "test@example.com"}]}]
            properties = ["email", "firstname"]

            result = execute(
                "hubspot_search",
                {
                    "object_type": "contacts",
                    "filter_groups": filter_groups,
                    "properties": properties,
                },
                {"HUBSPOT_API_KEY": "test-key"},
            )

            call_args = mock_client.return_value.__enter__.return_value.request.call_args
            self.assertEqual(call_args.kwargs["method"], "POST")
            self.assertIn("/crm/v3/objects/contacts/search", call_args.kwargs["url"])
            self.assertEqual(
                call_args.kwargs["json"],
                {"filterGroups": filter_groups, "properties": properties},
            )

    def test_hubspot_missing_api_key_raises_error(self):
        """Missing HUBSPOT_API_KEY raises ToolExecutionError."""
        from plugins.hubspot import execute

        with self.assertRaises(ToolExecutionError) as ctx:
            execute("hubspot_get_contact", {"contact_id": "123"}, {})

        self.assertIn("HUBSPOT_API_KEY", str(ctx.exception))

    def test_hubspot_http_4xx_returns_error_dict(self):
        """HTTP 4xx returns error dict with code."""
        from plugins.hubspot import execute

        with patch("plugins.hubspot.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.status_code = 404
            mock_response.text = "Not found"
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute(
                "hubspot_get_contact",
                {"contact_id": "nonexistent"},
                {"HUBSPOT_API_KEY": "test-key"},
            )

            self.assertIn("error", result)
            self.assertEqual(result["code"], 404)


class TestSupabasePlugin(unittest.TestCase):
    """Tests for supabase.py plugin."""

    def test_supabase_execute_sql_calls_correct_endpoint(self):
        """supabase_execute_sql calls correct endpoint."""
        from plugins.supabase import execute

        with patch("plugins.supabase.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = [{"id": 1}]
            mock_client.return_value.__enter__.return_value.post.return_value = mock_response

            result = execute(
                "supabase_execute_sql",
                {"query": "SELECT * FROM users"},
                {
                    "SUPABASE_URL": "https://myproject.supabase.co",
                    "SUPABASE_SERVICE_KEY": "test-key",
                },
            )

            mock_client.return_value.__enter__.return_value.post.assert_called_once()
            call_args = mock_client.return_value.__enter__.return_value.post.call_args
            self.assertIn("/rest/v1/rpc/exec_sql", call_args[0][0])
            self.assertEqual(call_args.kwargs["json"]["query"], "SELECT * FROM users")
            self.assertIn("result", result)

    def test_supabase_missing_url_raises_error(self):
        """Missing SUPABASE_URL raises ToolExecutionError."""
        from plugins.supabase import execute

        with self.assertRaises(ToolExecutionError) as ctx:
            execute(
                "supabase_execute_sql",
                {"query": "SELECT 1"},
                {"SUPABASE_SERVICE_KEY": "test-key"},
            )

        self.assertIn("SUPABASE_URL", str(ctx.exception))

    def test_supabase_missing_key_raises_error(self):
        """Missing SUPABASE_SERVICE_KEY raises ToolExecutionError."""
        from plugins.supabase import execute

        with self.assertRaises(ToolExecutionError) as ctx:
            execute(
                "supabase_execute_sql",
                {"query": "SELECT 1"},
                {"SUPABASE_URL": "https://myproject.supabase.co"},
            )

        self.assertIn("SUPABASE_SERVICE_KEY", str(ctx.exception))


class TestSlackPlugin(unittest.TestCase):
    """Tests for slack.py plugin."""

    def test_slack_send_message_sends_correct_body(self):
        """slack_send_message sends correct JSON body."""
        from plugins.slack import execute

        with patch("plugins.slack.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.json.return_value = {"ok": True, "ts": "123.456"}
            mock_client.return_value.__enter__.return_value.post.return_value = mock_response

            result = execute(
                "slack_send_message",
                {"channel": "#general", "text": "Hello!"},
                {"SLACK_BOT_TOKEN": "xoxb-test-token"},
            )

            call_args = mock_client.return_value.__enter__.return_value.post.call_args
            self.assertIn("chat.postMessage", call_args[0][0])
            self.assertEqual(call_args.kwargs["json"]["channel"], "#general")
            self.assertEqual(call_args.kwargs["json"]["text"], "Hello!")
            self.assertIn("result", result)

    def test_slack_api_ok_false_returns_error(self):
        """Slack API ok: false response returns error dict."""
        from plugins.slack import execute

        with patch("plugins.slack.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.json.return_value = {"ok": False, "error": "channel_not_found"}
            mock_client.return_value.__enter__.return_value.post.return_value = mock_response

            result = execute(
                "slack_send_message",
                {"channel": "#nonexistent", "text": "Hello!"},
                {"SLACK_BOT_TOKEN": "xoxb-test-token"},
            )

            self.assertIn("error", result)
            self.assertEqual(result["error"], "channel_not_found")

    def test_slack_missing_token_raises_error(self):
        """Missing SLACK_BOT_TOKEN raises ToolExecutionError."""
        from plugins.slack import execute

        with self.assertRaises(ToolExecutionError) as ctx:
            execute(
                "slack_send_message",
                {"channel": "#general", "text": "Hello!"},
                {},
            )

        self.assertIn("SLACK_BOT_TOKEN", str(ctx.exception))


class TestNotionPlugin(unittest.TestCase):
    """Tests for notion.py plugin."""

    def test_notion_search_sends_correct_request(self):
        """notion_search sends correct request with Notion-Version header."""
        from plugins.notion import execute

        with patch("plugins.notion.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = {"results": []}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute(
                "notion_search",
                {"query": "test"},
                {"NOTION_API_KEY": "secret_test"},
            )

            call_args = mock_client.return_value.__enter__.return_value.request.call_args
            self.assertEqual(call_args.kwargs["method"], "POST")
            self.assertIn("/v1/search", call_args.kwargs["url"])
            self.assertEqual(
                call_args.kwargs["headers"]["Notion-Version"],
                "2022-06-28",
            )
            self.assertEqual(
                call_args.kwargs["headers"]["Authorization"],
                "Bearer secret_test",
            )
            self.assertIn("result", result)

    def test_notion_search_with_filter_type(self):
        """notion_search includes filter when filter_type is provided."""
        from plugins.notion import execute

        with patch("plugins.notion.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = {"results": []}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute(
                "notion_search",
                {"query": "test", "filter_type": "page"},
                {"NOTION_API_KEY": "secret_test"},
            )

            call_args = mock_client.return_value.__enter__.return_value.request.call_args
            self.assertEqual(
                call_args.kwargs["json"]["filter"],
                {"value": "page", "property": "object"},
            )

    def test_notion_missing_api_key_raises_error(self):
        """Missing NOTION_API_KEY raises ToolExecutionError."""
        from plugins.notion import execute

        with self.assertRaises(ToolExecutionError) as ctx:
            execute("notion_search", {"query": "test"}, {})

        self.assertIn("NOTION_API_KEY", str(ctx.exception))


class TestToolExecutorIntegration(unittest.TestCase):
    """Integration tests for tool_executor with plugins."""

    def test_execute_tool_dispatches_to_plugin(self):
        """execute_tool dispatches plugin tools correctly."""
        from tool_executor import execute_tool

        with patch("plugins.web_fetch.httpx.Client") as mock_client:
            mock_response = MagicMock()
            mock_response.text = "Hello"
            mock_response.status_code = 200
            mock_response.headers = {"content-type": "text/plain"}
            mock_client.return_value.__enter__.return_value.request.return_value = mock_response

            result = execute_tool(
                "web_fetch",
                {"url": "https://example.com"},
                "test-task",
                "default",
                capabilities=["web-fetch"],
            )

            self.assertIn("result", result)
            self.assertEqual(result["result"]["status_code"], 200)

    def test_execute_tool_unknown_tool_without_capabilities(self):
        """execute_tool returns error for unknown tool without matching capabilities."""
        from tool_executor import execute_tool

        result = execute_tool(
            "web_fetch",
            {"url": "https://example.com"},
            "test-task",
            "default",
            capabilities=[],
        )

        self.assertFalse(result.get("success", True))
        self.assertIn("Unknown tool", result.get("error", ""))

    def test_execute_tool_handles_plugin_execution_error(self):
        """execute_tool handles ToolExecutionError from plugins."""
        from tool_executor import execute_tool

        result = execute_tool(
            "hubspot_get_contact",
            {"contact_id": "123"},
            "test-task",
            "default",
            capabilities=["hubspot"],
        )

        # Should get error about missing API key
        self.assertFalse(result.get("success", True))
        self.assertIn("HUBSPOT_API_KEY", result.get("error", ""))


if __name__ == "__main__":
    unittest.main()
