# Hortator Agent Runtime

Generic agent executor container for Hortator-managed AI agent pods.

## Filesystem Layout

| Path | Purpose |
|------|---------|
| `/inbox/task.json` | Task definition (read-only, mounted by operator) |
| `/outbox/result.json` | Task result (written by runtime) |
| `/outbox/usage.json` | Token usage for budget tracking |
| `/workspace` | Scratch space for the agent |
| `/memory` | Persistent memory (optional PVC mount) |

## Schemas

### task.json (input)

```json
{
  "taskId": "string",
  "prompt": "string",
  "role": "worker|planner|reviewer",
  "flavor": "string",
  "parentTaskId": "string|null",
  "tier": "tribune|centurion|legionary",
  "budget": 0,
  "capabilities": ["string"],
  "prior_work": "string"
}
```

### result.json (output)

```json
{
  "taskId": "string",
  "status": "completed|failed",
  "summary": "string",
  "artifacts": [],
  "decisions": 0,
  "tokensUsed": { "input": 0, "output": 0 },
  "duration": 0
}
```

### usage.json (output)

```json
{
  "input": 0,
  "output": 0,
  "total": 0
}
```

## Environment Variables

| Variable | Source | Description |
|----------|--------|-------------|
| `HORTATOR_TASK_ID` | task.json | Set by entrypoint |
| `HORTATOR_PROMPT` | task.json | The task prompt |
| `HORTATOR_ROLE` | task.json | Agent role |
| `HORTATOR_FLAVOR` | task.json | Task flavor |
| `HORTATOR_TIER` | task.json | Hierarchy tier (tribune/centurion/legionary) |
| `HORTATOR_BUDGET` | task.json | Token budget |
| `HORTATOR_TASK_NAME` | operator | K8s task name |
| `HORTATOR_MODEL` | operator | Override model selection |
| `OPENAI_API_KEY` | secret | Enables OpenAI backend |
| `ANTHROPIC_API_KEY` | secret | Enables Anthropic backend (preferred) |

## Model Selection

Model selection is determined by the operator, not the runtime tier. The
operator injects the `HORTATOR_MODEL` environment variable based on the
task's `spec.model.name` or the AgentRole's `defaultModel` field. The
`HORTATOR_TIER` value (tribune/centurion/legionary) controls which
**runtime** is used (agentic vs bash), not which model.

## Behavior

- Model comes from `HORTATOR_MODEL` env var (set by operator from the AgentRole or AgentTask spec)
- If `ANTHROPIC_API_KEY` is set and no model is configured, uses Anthropic (preferred)
- If `OPENAI_API_KEY` is set and no model is configured, uses OpenAI
- If neither is set, runs in **echo mode** (returns prompt as summary)
- Handles SIGTERM gracefully for operator-initiated timeouts
- Exits 0 on success, non-zero on failure
