# @hortator/sdk

TypeScript SDK for **Hortator** — Kubernetes-native AI agent orchestration.

## Install

```bash
npm install @hortator/sdk
```

## Quick Start

```typescript
import { HortatorClient } from "@hortator/sdk";

const client = new HortatorClient({
  baseUrl: "http://localhost:8080",
  apiKey: "your-api-key",
});

// Simple prompt
const result = await client.run("Summarize this document");
console.log(result.content);

// Chat with message history
const chat = await client.chat([
  { role: "system", content: "You are a helpful assistant." },
  { role: "user", content: "Hello!" },
]);

// Streaming
for await (const chunk of client.stream("Tell me a story")) {
  process.stdout.write(chunk.content);
}

// List available models
const models = await client.listModels();
```

## Options

```typescript
const result = await client.run("Hello", {
  role: "centurion",                    // default: "legionary"
  capabilities: ["code", "search"],
  tier: "premium",
  budget: { max_cost_usd: "1.00", max_tokens: 1000 },
  temperature: 0.7,
  max_tokens: 2048,
});
```

## Error Handling

```typescript
import { AuthenticationError, RateLimitError, HortatorError } from "@hortator/sdk";

try {
  await client.run("Hello");
} catch (e) {
  if (e instanceof AuthenticationError) { /* 401 */ }
  if (e instanceof RateLimitError) { /* 429 */ }
  if (e instanceof HortatorError) { /* other HTTP errors */ }
}
```

## LangChain.js Integration

```typescript
import { createHortatorTool } from "@hortator/sdk/langchain";

const tool = await createHortatorTool({ client });
// Use with LangChain agents
```

Requires `@langchain/core` as a peer dependency.

## Requirements

- Node.js 18+ (uses native `fetch`)
- Zero runtime dependencies
