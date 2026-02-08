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
  "tier": "fast|think|deep",
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
| `HORTATOR_TIER` | task.json | Model tier (fast/think/deep) |
| `HORTATOR_BUDGET` | task.json | Token budget |
| `HORTATOR_TASK_NAME` | operator | K8s task name |
| `HORTATOR_MODEL` | operator | Override model selection |
| `OPENAI_API_KEY` | secret | Enables OpenAI backend |
| `ANTHROPIC_API_KEY` | secret | Enables Anthropic backend (preferred) |

## Tier → Model Mapping

| Tier | OpenAI | Anthropic |
|------|--------|-----------|
| fast | gpt-4o-mini | claude-sonnet-4-20250514 |
| think | gpt-4o | claude-sonnet-4-20250514 |
| deep | gpt-4o | claude-opus-4-20250514 |

## Behavior

- If `ANTHROPIC_API_KEY` is set, uses Anthropic (preferred)
- If `OPENAI_API_KEY` is set, uses OpenAI
- If neither is set, runs in **echo mode** (returns prompt as summary)
- Handles SIGTERM gracefully for operator-initiated timeouts
- Exits 0 on success, non-zero on failure
