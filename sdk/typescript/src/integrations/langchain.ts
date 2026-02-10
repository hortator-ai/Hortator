/**
 * LangChain.js integration for Hortator.
 * Requires `@langchain/core` as a peer dependency.
 *
 * Usage:
 *   import { HortatorTool } from "@hortator/sdk/langchain";
 */
import type { HortatorClient } from "../client.js";
import type { RequestOptions } from "../types.js";

// Dynamic import to avoid hard dependency
let StructuredToolBase: any;

async function getStructuredTool() {
  if (!StructuredToolBase) {
    try {
      const mod = await import("@langchain/core/tools");
      StructuredToolBase = mod.StructuredTool;
    } catch {
      throw new Error(
        "@langchain/core is required for LangChain integration. Install it with: npm install @langchain/core",
      );
    }
  }
  return StructuredToolBase;
}

export interface HortatorToolOptions {
  client: HortatorClient;
  name?: string;
  description?: string;
  requestOptions?: RequestOptions;
}

export async function createHortatorTool(opts: HortatorToolOptions) {
  const Base = await getStructuredTool();
  const { z } = await import("zod").catch(() => {
    throw new Error("zod is required for LangChain integration");
  });

  const schema = z.object({
    prompt: z.string().describe("The prompt to send to Hortator"),
  });

  return new (class HortatorTool extends Base {
    name = opts.name ?? "hortator";
    description = opts.description ?? "Send a prompt to Hortator AI agent orchestration";
    schema = schema;

    async _call(input: { prompt: string }) {
      const result = await opts.client.run(input.prompt, opts.requestOptions);
      return result.content;
    }
  })();
}
