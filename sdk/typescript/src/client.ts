import { handleErrorResponse } from "./errors.js";
import { parseSSEStream } from "./streaming.js";
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ClientOptions,
  ContentPart,
  Message,
  MessageContent,
  ModelInfo,
  ModelsResponse,
  RequestOptions,
  RunResult,
  StreamChunk,
} from "./types.js";

const DEFAULT_ROLE = "legionary";

/**
 * Build messages with optional file attachments.
 * If files are provided, the content becomes an array of ContentParts.
 */
function buildMessagesWithFiles(
  prompt: string,
  files?: RequestOptions["files"],
): Message[] {
  if (!files || files.length === 0) {
    return [{ role: "user", content: prompt }];
  }

  const parts: ContentPart[] = [{ type: "text", text: prompt }];
  for (const f of files) {
    const b64 =
      typeof f.data === "string"
        ? f.data
        : Buffer.from(f.data).toString("base64");
    parts.push({
      type: "file",
      file: { filename: f.filename, file_data: b64 },
    });
  }
  return [{ role: "user", content: parts }];
}

export class HortatorClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly timeout: number;

  constructor(opts: ClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.apiKey = opts.apiKey;
    this.timeout = opts.timeout ?? 30_000;
  }

  /**
   * Run a task with an optional list of file attachments.
   */
  async run(prompt: string, opts?: RequestOptions): Promise<RunResult> {
    const messages = buildMessagesWithFiles(prompt, opts?.files);
    return this.chat(messages, opts);
  }

  async chat(messages: Message[], opts?: RequestOptions): Promise<RunResult> {
    const body = this.buildRequest(messages, opts, false);
    const res = await this.fetch("/v1/chat/completions", body);
    const data: ChatCompletionResponse = await res.json();
    const choice = data.choices[0];
    // Extract text from content (handles both string and ContentPart array)
    const content = typeof choice.message.content === "string"
      ? choice.message.content
      : (choice.message.content as ContentPart[])
          .filter((p) => p.type === "text")
          .map((p) => p.text ?? "")
          .join("\n");
    return {
      id: data.id,
      content,
      finish_reason: choice.finish_reason,
      model: data.model,
      usage: data.usage,
    };
  }

  /**
   * Stream a task with an optional list of file attachments.
   */
  async *stream(prompt: string, opts?: RequestOptions): AsyncIterable<StreamChunk> {
    const messages = buildMessagesWithFiles(prompt, opts?.files);
    const body = this.buildRequest(messages, opts, true);
    const res = await this.fetch("/v1/chat/completions", body);
    if (!res.body) throw new Error("Response body is null");
    yield* parseSSEStream(res.body);
  }

  async listModels(): Promise<ModelInfo[]> {
    const res = await this.fetch("/v1/models", undefined, "GET");
    const data: ModelsResponse = await res.json();
    return data.data;
  }

  private buildRequest(
    messages: Message[],
    opts?: RequestOptions,
    stream?: boolean,
  ): ChatCompletionRequest {
    const role = opts?.role ?? DEFAULT_ROLE;
    const req: ChatCompletionRequest = {
      model: `hortator/${role}`,
      messages,
      stream,
    };
    if (opts?.temperature != null) req.temperature = opts.temperature;
    if (opts?.max_tokens != null) req.max_tokens = opts.max_tokens;
    if (opts?.capabilities) req.x_capabilities = opts.capabilities;
    if (opts?.tier) req.x_tier = opts.tier;
    if (opts?.budget) req.x_budget = opts.budget;
    return req;
  }

  private async fetch(path: string, body?: unknown, method = "POST"): Promise<Response> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const res = await fetch(`${this.baseUrl}${path}`, {
        method,
        headers: {
          ...(body ? { "Content-Type": "application/json" } : {}),
          Authorization: `Bearer ${this.apiKey}`,
        },
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      if (!res.ok) {
        const text = await res.text();
        handleErrorResponse(res.status, text);
      }

      return res;
    } finally {
      clearTimeout(timer);
    }
  }
}
