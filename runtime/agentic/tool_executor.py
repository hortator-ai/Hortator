"""
Tool execution — translates tool calls into actual operations.

spawn_task, check_status, get_result, cancel_task → shell out to `hortator` CLI
run_shell → subprocess in /workspace/
read_file, write_file → filesystem I/O
"""

import json
import os
import subprocess


def execute_tool(name: str, args: dict, task_name: str, task_ns: str) -> dict:
    """Execute a tool call and return the result as a dict."""
    try:
        match name:
            case "spawn_task":
                return _exec_spawn_task(args, task_name, task_ns)
            case "check_status":
                return _exec_check_status(args)
            case "get_result":
                return _exec_get_result(args)
            case "cancel_task":
                return _exec_cancel_task(args)
            case "checkpoint_and_wait":
                return _exec_checkpoint_and_wait(args)
            case "list_roles":
                return _exec_list_roles(args)
            case "describe_role":
                return _exec_describe_role(args)
            case "run_shell":
                return _exec_run_shell(args)
            case "read_file":
                return _exec_read_file(args)
            case "write_file":
                return _exec_write_file(args)
            case _:
                return {"success": False, "error": f"Unknown tool: {name}"}
    except Exception as e:
        return {"success": False, "error": str(e)}


# ── Spawn / Task Management ─────────────────────────────────────────────────

def _exec_spawn_task(args: dict, parent_name: str, task_ns: str) -> dict:
    """Create a child AgentTask via `hortator spawn`."""
    prompt = args.get("prompt", "")
    if not prompt:
        return {"success": False, "error": "prompt is required"}

    cmd = ["hortator", "spawn", "--prompt", prompt, "--parent", parent_name]

    if args.get("role"):
        cmd.extend(["--role", args["role"]])
    if args.get("tier"):
        cmd.extend(["--tier", args["tier"]])
    if args.get("capabilities"):
        cmd.extend(["--capabilities", args["capabilities"]])

    wait = args.get("wait", False)
    if wait:
        cmd.append("--wait")

    cmd.extend(["-o", "json"])

    result = subprocess.run(
        cmd, capture_output=True, text=True,
        timeout=600 if wait else 30,
    )

    if result.returncode != 0:
        return {
            "success": False,
            "error": result.stderr.strip() or f"exit code {result.returncode}",
        }

    try:
        output = json.loads(result.stdout)
    except json.JSONDecodeError:
        output = {"raw": result.stdout.strip()}

    return {
        "success": True,
        "task_name": output.get("name") or output.get("task", ""),
        "phase": output.get("phase", ""),
        "output": output.get("output", ""),
        "async": not wait,
    }


def _exec_check_status(args: dict) -> dict:
    """Check child task status via `hortator status`."""
    task_name = args.get("task_name", "")
    if not task_name:
        return {"success": False, "error": "task_name is required"}

    result = subprocess.run(
        ["hortator", "status", task_name, "-o", "json"],
        capture_output=True, text=True, timeout=15,
    )

    if result.returncode != 0:
        return {"success": False, "error": result.stderr.strip()}

    try:
        output = json.loads(result.stdout)
    except json.JSONDecodeError:
        output = {"raw": result.stdout.strip()}

    return {
        "success": True,
        "name": task_name,
        "phase": output.get("phase", "Unknown"),
        "message": output.get("message", ""),
    }


def _exec_get_result(args: dict) -> dict:
    """Retrieve child task result via `hortator result`."""
    task_name = args.get("task_name", "")
    if not task_name:
        return {"success": False, "error": "task_name is required"}

    result = subprocess.run(
        ["hortator", "result", task_name, "-o", "json"],
        capture_output=True, text=True, timeout=15,
    )

    if result.returncode != 0:
        return {"success": False, "error": result.stderr.strip()}

    try:
        output = json.loads(result.stdout)
    except json.JSONDecodeError:
        return {"success": True, "output": result.stdout.strip()}

    return {
        "success": True,
        "name": task_name,
        "phase": output.get("phase", ""),
        "output": output.get("output", ""),
    }


def _exec_cancel_task(args: dict) -> dict:
    """Cancel a child task via `hortator cancel`."""
    task_name = args.get("task_name", "")
    if not task_name:
        return {"success": False, "error": "task_name is required"}

    result = subprocess.run(
        ["hortator", "cancel", task_name],
        capture_output=True, text=True, timeout=15,
    )

    if result.returncode != 0:
        return {"success": False, "error": result.stderr.strip()}

    return {"success": True, "message": f"Task {task_name} cancelled"}


# ── Checkpoint ───────────────────────────────────────────────────────────────

def _exec_checkpoint_and_wait(args: dict) -> dict:
    """Signal the loop to checkpoint and exit with 'waiting' status.

    This doesn't do the actual checkpoint saving — the loop handles that.
    We return a sentinel that the loop detects after tool execution.
    """
    summary = args.get("summary", "")
    return {
        "success": True,
        "_checkpoint_and_wait": True,
        "summary": summary,
    }


# ── Role Discovery ───────────────────────────────────────────────────────────

def _exec_list_roles(args: dict) -> dict:
    """List available roles via `hortator roles list --json`."""
    result = subprocess.run(
        ["hortator", "roles", "list", "-o", "json"],
        capture_output=True, text=True, timeout=15,
    )
    if result.returncode != 0:
        return {"success": False, "error": result.stderr.strip()}

    try:
        roles = json.loads(result.stdout)
    except json.JSONDecodeError:
        return {"success": False, "error": "Failed to parse roles output"}

    return {"success": True, "roles": roles}


def _exec_describe_role(args: dict) -> dict:
    """Describe a specific role via `hortator roles describe <name> --json`."""
    name = args.get("name", "")
    if not name:
        return {"success": False, "error": "name is required"}

    result = subprocess.run(
        ["hortator", "roles", "describe", name, "-o", "json"],
        capture_output=True, text=True, timeout=15,
    )
    if result.returncode != 0:
        return {"success": False, "error": result.stderr.strip()}

    try:
        role = json.loads(result.stdout)
    except json.JSONDecodeError:
        return {"success": False, "error": "Failed to parse role output"}

    return {"success": True, "role": role}


# ── Shell Execution ──────────────────────────────────────────────────────────

def _check_shell_command_policy(command: str) -> str | None:
    """Check if a shell command is allowed by policy.

    Returns an error message if the command is rejected, or None if allowed.
    """
    allowed_raw = os.environ.get("HORTATOR_ALLOWED_COMMANDS", "")
    denied_raw = os.environ.get("HORTATOR_DENIED_COMMANDS", "")

    if not allowed_raw and not denied_raw:
        return None

    allowed = [c.strip() for c in allowed_raw.split(",") if c.strip()] if allowed_raw else []
    denied = [c.strip() for c in denied_raw.split(",") if c.strip()] if denied_raw else []

    # Extract base commands: first word of the command and first word after each pipe
    parts = command.split("|")
    base_commands = []
    for part in parts:
        stripped = part.strip()
        if stripped:
            base_cmd = stripped.split()[0] if stripped.split() else ""
            if base_cmd:
                base_commands.append(base_cmd)

    for base_cmd in base_commands:
        # Check allowed list (if set, only these are permitted)
        if allowed and base_cmd not in allowed:
            return f"Command '{base_cmd}' is not allowed by policy"

        # Check denied list
        for denied_prefix in denied:
            if command.strip().startswith(denied_prefix) or base_cmd == denied_prefix:
                return f"Command '{base_cmd}' is denied by policy"

    return None


def _exec_run_shell(args: dict) -> dict:
    """Execute a shell command in /workspace/."""
    command = args.get("command", "")
    if not command:
        return {"success": False, "error": "command is required"}

    # Shell command filtering via AgentPolicy env vars
    base_cmd = command.strip().split()[0] if command.strip() else ""
    allowed = os.environ.get("HORTATOR_ALLOWED_COMMANDS", "").split(",")
    denied = os.environ.get("HORTATOR_DENIED_COMMANDS", "").split(",")
    if allowed and allowed != [''] and base_cmd not in allowed:
        return {"success": False, "error": f"Command '{base_cmd}' not in allowed list"}
    if denied and denied != [''] and base_cmd in denied:
        return {"success": False, "error": f"Command '{base_cmd}' is denied by policy"}

    timeout = args.get("timeout", 120)
    workspace = "/workspace"

    try:
        result = subprocess.run(
            command, shell=True,
            capture_output=True, text=True,
            timeout=timeout, cwd=workspace,
        )
    except subprocess.TimeoutExpired:
        return {
            "success": False,
            "error": f"Command timed out after {timeout}s",
            "exit_code": -1,
        }

    # Truncate very large output
    stdout = result.stdout
    stderr = result.stderr
    if len(stdout) > 10000:
        stdout = stdout[:5000] + "\n... (truncated) ...\n" + stdout[-5000:]
    if len(stderr) > 5000:
        stderr = stderr[:2500] + "\n... (truncated) ...\n" + stderr[-2500:]

    return {
        "success": result.returncode == 0,
        "exit_code": result.returncode,
        "stdout": stdout,
        "stderr": stderr,
    }


# ── File I/O ─────────────────────────────────────────────────────────────────

ALLOWED_READ_PREFIXES = ["/inbox/", "/outbox/", "/workspace/", "/memory/", "/prior/"]
ALLOWED_WRITE_PREFIXES = ["/outbox/", "/workspace/", "/memory/"]


def _exec_read_file(args: dict) -> dict:
    """Read a file from the agent's filesystem."""
    path = args.get("path", "")
    if not path:
        return {"success": False, "error": "path is required"}

    if not any(path.startswith(p) for p in ALLOWED_READ_PREFIXES):
        return {"success": False, "error": f"Read access denied: {path}. "
                f"Allowed prefixes: {ALLOWED_READ_PREFIXES}"}

    if not os.path.isfile(path):
        return {"success": False, "error": f"File not found: {path}"}

    try:
        with open(path) as f:
            content = f.read()
    except Exception as e:
        return {"success": False, "error": f"Read failed: {e}"}

    # Truncate very large files
    if len(content) > 50000:
        content = content[:25000] + "\n... (truncated) ...\n" + content[-25000:]

    return {"success": True, "path": path, "content": content}


def _exec_write_file(args: dict) -> dict:
    """Write a file to the agent's filesystem."""
    path = args.get("path", "")
    content = args.get("content", "")
    if not path:
        return {"success": False, "error": "path is required"}

    if not any(path.startswith(p) for p in ALLOWED_WRITE_PREFIXES):
        return {"success": False, "error": f"Write access denied: {path}. "
                f"Allowed prefixes: {ALLOWED_WRITE_PREFIXES}"}

    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(content)
    except Exception as e:
        return {"success": False, "error": f"Write failed: {e}"}

    return {"success": True, "path": path, "bytes_written": len(content)}
