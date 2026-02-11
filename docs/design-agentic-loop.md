# Design: Agentic Loop & Autonomous Task Execution

**Status:** Implemented  
**Date:** 2026-02-11  
**Author:** Daemon + M

## Problem

Hortator's elevator pitch is "OpenClaw for Enterprise" — autonomous AI agents with infrastructure-grade controls. Today, that promise is broken in two places:

1. **Tribunes cannot orchestrate autonomously.** The default runtime (`entrypoint.sh`) makes a single LLM call and exits. The stress test (`examples/e2e/stress-test.yaml`) demonstrates 22 tasks across Tribune, Centurion, and Legionary tiers — but every task is manually authored in YAML by a human. The Tribune does not decompose, delegate, or consolidate. It just answers its own prompt. This is the P0 bug in the backlog.

2. **There is no mechanism for multi-week task lifecycle or artifact retrieval.** The gateway's blocking and SSE modes assume HTTP-lifetime tasks. `status.output` is capped at 16,000 characters. A task that produces thousands of files (a codebase, a legal review, a research corpus) has no extraction path. There is no async job submission, no artifact download, and no notification on completion.

These two gaps mean the system cannot accept a prompt like "write a C compiler including all tests" and autonomously handle design, planning, building, testing, and documenting — even though the CRD hierarchy, policy enforcement, and storage model were designed for exactly that.

This document defines the architecture to close both gaps.

## Architecture: Tribune as Autonomous Orchestrator

The core insight: **the Tribune is the orchestrator, the operator is the enforcer**. The Tribune's LLM decides what to decompose and how. The operator's AgentPolicy validation decides what is permitted.

### End-to-End Flow

```
User submits: "Write a C compiler including all tests"
  │
  ▼
AgentTask CR created (tier: tribune, role: tech-lead)
  │
  ▼
Operator validates against AgentPolicy, creates Tribune Pod
  │
  ▼
Python agentic runtime starts:
  │
  ├─ LLM call #1: "Plan this project. You have tools: spawn_task, ..."
  │   └─ LLM responds with tool calls
  │
  ├─ Runtime executes: hortator spawn --prompt "Design the lexer" --role architect --tier centurion
  │   └─ Operator receives AgentTask CR
  │       ├─ Validates against AgentPolicy (capabilities, budget, images)
  │       ├─ Checks capability inheritance (child ≤ parent)
  │       └─ Rejects if policy violated → Tribune gets error, adapts
  │
  ├─ Valid children become Pods → work → complete
  │   └─ Results flow back to Tribune
  │
  ├─ LLM call #N: "All subtasks complete. Consolidate."
  │   └─ Tribune writes final result
  │
  ▼
Task completed. Result in status.output + /outbox/artifacts/.
```

### Why This Works

The separation is clean:

- **Intelligence lives in the LLM.** The Tribune's model decides how to decompose "write a C compiler" — it knows about lexers, parsers, code generation, test suites. Hortator does not. The same Tribune can handle "review this contract" or "analyse this dataset" without Hortator containing domain knowledge.

- **Governance lives in the operator.** Every `hortator spawn` call creates an AgentTask CR that goes through the full reconciliation pipeline: AgentPolicy checks, capability inheritance validation, namespace budget enforcement, image allowlisting. The Tribune can *try* to spawn 100 GPU agents with `shell` capability — the operator will reject what the policy forbids.

- **No new CRDs required.** The existing AgentTask, AgentRole, and AgentPolicy CRDs are sufficient. The Tribune creates standard AgentTasks via the CLI. The hierarchy is expressed through `parentTaskId`, exactly as designed.

## Python Agentic Runtime

The current `entrypoint.sh` is adequate for Legionaries — leaf tasks that make a single LLM call and produce a result. Tribunes and Centurions need an agentic loop.

### Runtime Selection

The operator selects the runtime based on the task's tier:

| Tier | Runtime | Behaviour |
|------|---------|-----------|
| **Tribune** | Python agentic (`runtime/agentic/`) | Tool-calling loop with checkpoint/restore |
| **Centurion** | Python agentic (`runtime/agentic/`) | Tool-calling loop with checkpoint/restore |
| **Legionary** | Bash single-shot (`runtime/entrypoint.sh`) | Single LLM call, write result, exit |

The operator sets the container image and entrypoint based on tier. Centurions use the agentic runtime because they may also need to spawn Legionaries and consolidate their results.

### Tool-Calling Loop

The Python runtime implements a standard tool-calling loop:

```python
# Simplified — actual implementation uses litellm for provider-agnostic LLM calls
def agentic_loop(task: Task, tools: list[Tool]) -> Result:
    messages = [
        {"role": "system", "content": build_system_prompt(task)},
        {"role": "user", "content": task.prompt},
    ]

    while True:
        response = llm_call(messages, tools=tools)

        if response.stop_reason == "end_turn":
            return write_result(response.content)

        if response.stop_reason == "tool_use":
            for tool_call in response.tool_calls:
                result = execute_tool(tool_call)
                messages.append(tool_result(tool_call.id, result))

            messages.append(response)  # include assistant turn with tool calls
            continue

        # Budget / context overflow handling
        if should_checkpoint(messages, task.budget):
            checkpoint_state(messages, task)
            return Result(status="checkpoint", state_ref="/memory/state.json")
```

### Available Tools

Tools are gated by the agent's capabilities (from the AgentTask spec or inherited from the AgentRole):

| Tool | Capability | What It Does |
|------|-----------|--------------|
| `spawn_task` | `spawn` | Creates a child AgentTask via `hortator spawn`. Supports `--wait` for synchronous execution or fire-and-forget. |
| `check_status` | `spawn` | Reads status of a child task via `hortator status`. |
| `get_result` | `spawn` | Retrieves the output of a completed child via `hortator result`. |
| `cancel_task` | `spawn` | Cancels a running child via `hortator cancel`. |
| `run_shell` | `shell` | Executes a shell command in `/workspace/`. |
| `read_file` | (always) | Reads a file from the agent's filesystem. |
| `write_file` | (always) | Writes a file to `/outbox/artifacts/` or `/workspace/`. |

The Tribune's system prompt includes instructions about available tools and the expected workflow: plan, delegate, collect results, consolidate.

### Provider-Agnostic LLM Integration

The runtime uses `litellm` for LLM calls. This means the same runtime works with Anthropic, OpenAI, local models via Ollama/vLLM, or any OpenAI-compatible endpoint. The LLM endpoint and model name come from the task's model configuration (injected by the operator from the AgentRole or AgentTask spec).

## Reincarnation Model

A Tribune orchestrating a 2-week project cannot run as a single continuous pod. The pod would be idle 95% of the time (waiting for children), vulnerable to node eviction, and would accumulate unbounded conversation context.

Instead, the Tribune uses an **event-driven reincarnation model**: run, spawn children, checkpoint state, exit. Wake up when children complete. Resume from checkpoint.

### Lifecycle

```
Tribune Run 1
  ├─ Read /inbox/task.json (original prompt)
  ├─ LLM: plan decomposition
  ├─ Spawn 3 centurions (design, architecture, test strategy)
  ├─ Checkpoint state to /memory/state.json
  └─ Exit with status "Waiting" (new phase)

    ... centurions work for hours ...

Tribune Run 2
  ├─ Read /memory/state.json (restore checkpoint)
  ├─ Read /inbox/child-results/ (injected by operator)
  ├─ LLM: review results, plan next phase
  ├─ Spawn 5 legionaries (implement modules)
  ├─ Checkpoint state
  └─ Exit with status "Waiting"

    ... legionaries work ...

Tribune Run N (final)
  ├─ Read state + all child results
  ├─ LLM: consolidate, run final checks
  ├─ Write /outbox/result.json (summary)
  ├─ Push to git / write artifacts
  └─ Exit with status "Completed"
```

### Checkpoint Schema

The runtime writes `/memory/state.json` before exiting:

```json
{
  "version": 1,
  "taskId": "build-c-compiler",
  "phase": "implementation",
  "plan": {
    "phases": ["design", "implementation", "testing", "documentation"],
    "currentPhase": 1
  },
  "completedChildren": [
    {
      "name": "design-lexer",
      "status": "Completed",
      "summaryRef": "/inbox/child-results/design-lexer.json"
    }
  ],
  "pendingChildren": [
    {"name": "impl-parser", "status": "Running"}
  ],
  "decisions": [
    "Using recursive descent parser (simpler, sufficient for C89)",
    "Targeting x86-64 only for MVP"
  ],
  "accumulatedContext": "Key findings from design phase: ..."
}
```

The `accumulatedContext` field is critical — it carries forward a compressed summary of prior work so the Tribune does not need to re-read every child result on every reincarnation. This is the structured extraction strategy from the context compression design.

### Operator Changes: Parent Wake-Up

The operator needs a new reconciliation path for the reincarnation model:

**New phase: `Waiting`.** When the agentic runtime exits with a checkpoint and pending children, the task enters the `Waiting` phase. The pod terminates, but the PVC persists.

**Child completion trigger.** When a child task reaches a terminal phase, `notifyParentTask()` is extended:

1. Append child name to `parent.Status.ChildTasks` (existing behaviour).
2. Copy child's `status.output` to a file in the parent's PVC at `/inbox/child-results/<child-name>.json` (new behaviour — via a short-lived exec into a utility pod, or by mounting the parent PVC into a job).
3. Check if all pending children for the parent are now terminal. If yes, set the parent's phase back to `Pending` — this triggers the operator to recreate the Tribune pod, which reads its checkpoint and the new results.

**Idempotency.** The operator already handles re-reconciliation safely. The Tribune pod's PVC persists across reincarnations, so `/memory/state.json` survives. The checkpoint includes enough information for the runtime to determine what has changed since the last run.

### Benefits

- **Resilience.** Node failure, pod eviction, or OOM during a child's execution does not affect the Tribune — it is not running. The checkpoint on the PVC survives.
- **No context overflow.** Each Tribune run starts with a fresh context window. Only the checkpoint and new child results are loaded, not the full history of every prior LLM turn.
- **No idle resources.** The Tribune pod only runs when it has work to do: planning, spawning, or consolidating. It does not sit idle for hours waiting for children.
- **Natural budget tracking.** Each Tribune run is a separate pod with measurable token usage. Total cost is the sum of all runs plus all children.

## Result Delivery Model

Three access patterns, zero delivery-specific CRD fields. Hortator provides the tools; the agent (or the caller) decides how to deliver.

### Access Pattern 1: Synchronous (IDE Integration)

For interactive tasks (fix a bug, refactor a file, review code), the existing OpenAI-compatible gateway streams the response. The IDE sends `POST /v1/chat/completions`, receives SSE chunks, and applies changes to the workspace.

This already works. No changes needed for tasks that complete within HTTP timeout and produce text-sized output.

### Access Pattern 2: Asynchronous (Mega-Tasks)

For tasks that run for hours, days, or weeks, the caller submits the task and retrieves the result later.

**Submission:** `kubectl apply -f task.yaml`, or `POST /v1/chat/completions` (gateway creates the task and returns immediately with a task ID if the request includes a header like `X-Hortator-Async: true`), or SDK `client.tasks.create(...)`.

**Polling:** `hortator status <task>` or `GET /api/v1/tasks/{id}` returns the current phase, child task tree, and budget usage.

**Artifact retrieval:** New gateway endpoint `GET /api/v1/tasks/{id}/artifacts` serves files from the completed task's PVC. The CLI equivalent: `hortator result <task> --artifacts` downloads `/outbox/artifacts/` contents to a local directory.

Implementation: the gateway mounts (or exec-reads) the task's PVC and serves files. For completed tasks with retained PVCs, this is straightforward. For tasks whose PVCs have been cleaned up, the endpoint returns 410 Gone.

### Access Pattern 3: Agent-Driven Push

For code projects, the natural delivery is a git push. For reports, it might be an upload to S3 or a shared drive. This is configured per AgentRole, not per task:

```yaml
apiVersion: core.hortator.ai/v1alpha1
kind: ClusterAgentRole
metadata:
  name: software-engineer
spec:
  systemPrompt: |
    When your work produces code, commit and push to the git
    repository configured in your environment. Always write a
    summary to /outbox/result.json.
  capabilities: [shell, spawn, git]
  env:
    - name: GIT_REMOTE
      valueFrom:
        configMapKeyRef:
          name: project-config
          key: git-remote
  secretRefs:
    - name: git-credentials
```

The agent pushes as its final act. Hortator's role: ensure the agent has the `git` capability (network access via NetworkPolicy) and the credentials (mounted secrets). Hortator does not know or care about git — it provides the plumbing.

### PVC as Universal Fallback

Regardless of delivery method, the PVC always contains the authoritative result:

| Path | Content | Always Present |
|------|---------|----------------|
| `/outbox/result.json` | Structured summary (taskId, status, summary, artifacts list) | Yes (enforced by `runtime.filesystem.enforceRequired`) |
| `/outbox/artifacts/` | Produced files (code, documents, patches, reports) | If the task produces files |
| `/outbox/usage.json` | Token usage for budget tracking | Yes |

If the agent fails to push to git, the result is still on the PVC. If the PVC has been cleaned up, the text summary is still in `status.output`.

## Integration Patterns

### Cursor / IDE

The OpenAI-compatible API is the integration point. No plugin or special protocol needed.

**Quick tasks:** Cursor sends prompt, Hortator streams response, Cursor applies changes. Same as using any LLM backend.

**Longer tasks:** A Cursor extension could submit a task via the API, poll for completion, and download artifacts via `GET /api/v1/tasks/{id}/artifacts` to apply them to the workspace. This is future work and out of scope for Hortator itself — Hortator provides the endpoints, the IDE extension is someone else's problem.

### CI/CD

```bash
# Submit
kubectl apply -f build-compiler.yaml

# Monitor
hortator watch -n ai-team --task build-compiler

# Retrieve when done
hortator result build-compiler --artifacts --output-dir ./build-output/
```

### SDK

The Python and TypeScript SDKs wrap the gateway. Add one method:

```python
# Python SDK
artifacts = client.tasks.get_artifacts("build-compiler")
for artifact in artifacts:
    artifact.save_to("./output/")
```

### Webhook (Future)

An optional callback URL on task completion. Deferred — polling + artifact download is sufficient for v1. When implemented, it would be a field on the AgentTask spec: `spec.webhook.url` and `spec.webhook.secretRef`.

## What Changes in Each Component

| Component | Change | Scope |
|-----------|--------|-------|
| **Runtime** | New Python agentic runtime at `runtime/agentic/` with tool-calling loop, checkpoint/restore, litellm integration | New directory, new container image |
| **Operator** | `Waiting` phase; child-completion → parent wake-up reconciliation; child result injection into parent PVC | Modify `agenttask_controller.go`, `helpers.go` |
| **Gateway** | `GET /api/v1/tasks/{id}/artifacts` endpoint; optional `X-Hortator-Async` header for fire-and-forget submission | Modify `handler.go`, add artifact handler |
| **CLI** | `hortator result --artifacts --output-dir` flag for PVC content download | Modify `result.go` |
| **CRDs** | No new CRDs. Add `Waiting` to the phase enum in AgentTaskStatus | Modify `agenttask_types.go` |
| **Helm** | Config for agentic runtime image, checkpoint settings | Modify `values.yaml` |

## Scope Boundary

| Concern | In Scope? | Why |
|---------|-----------|-----|
| Agentic tool-calling loop for Tribunes/Centurions | **Yes** | Core gap — without this, autonomous orchestration is impossible |
| Reincarnation (event-driven Tribune lifecycle) | **Yes** | Required for multi-day/week tasks; K8s-native resilience |
| Artifact download endpoint | **Yes** | Minimal surface area to enable async result retrieval |
| `Waiting` phase and parent wake-up | **Yes** | Operator support for reincarnation model |
| Git push / S3 upload / delivery format | **No** | Agent's responsibility via capabilities — Hortator provides tools, not delivery logic |
| IDE-specific plugins | **No** | The OpenAI API is the integration; plugins are someone else's problem |
| Webhook/callback on completion | **No** (deferred) | Nice to have, not required for v1 — polling works |
| Orchestration CRD / plan CRD | **No** | The Tribune IS the orchestrator — no separate plan object needed |

This follows the established principle: **orchestrate agents, don't be the agent**. Hortator manages pods, lifecycles, budgets, and security. It does not decide how to decompose tasks, what delivery format to use, or how to structure a C compiler.
