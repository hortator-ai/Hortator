import type { ChatCompletionChunk, StreamChunk } from "./types.js";

export async function* parseSSEStream(
  body: ReadableStream<Uint8Array>,
): AsyncIterable<StreamChunk> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith(":")) continue;
        if (!trimmed.startsWith("data: ")) continue;

        const data = trimmed.slice(6);
        if (data === "[DONE]") return;

        const chunk: ChatCompletionChunk = JSON.parse(data);
        const choice = chunk.choices[0];
        if (!choice) continue;

        yield {
          id: chunk.id,
          content: choice.delta.content ?? "",
          finish_reason: choice.finish_reason,
          model: chunk.model,
        };
      }
    }
  } finally {
    reader.releaseLock();
  }
}
