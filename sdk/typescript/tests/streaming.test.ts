import { describe, it, expect } from "vitest";
import { parseSSEStream } from "../src/streaming.js";

function createStream(text: string): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(text));
      controller.close();
    },
  });
}

const chunk = (content: string, finish: string | null = null) =>
  JSON.stringify({
    id: "c1",
    object: "chat.completion.chunk",
    created: 1700000000,
    model: "hortator/legionary",
    choices: [{ index: 0, delta: { content }, finish_reason: finish }],
  });

describe("parseSSEStream", () => {
  it("parses multiple data lines", async () => {
    const stream = createStream(
      `data: ${chunk("Hello")}\ndata: ${chunk(" world", "stop")}\ndata: [DONE]\n`,
    );

    const chunks = [];
    for await (const c of parseSSEStream(stream)) {
      chunks.push(c);
    }

    expect(chunks).toHaveLength(2);
    expect(chunks[0].content).toBe("Hello");
    expect(chunks[1].content).toBe(" world");
    expect(chunks[1].finish_reason).toBe("stop");
  });

  it("handles empty content in delta", async () => {
    const stream = createStream(
      `data: ${JSON.stringify({ id: "c1", model: "m", choices: [{ index: 0, delta: {}, finish_reason: null }] })}\ndata: [DONE]\n`,
    );

    const chunks = [];
    for await (const c of parseSSEStream(stream)) {
      chunks.push(c);
    }

    expect(chunks).toHaveLength(1);
    expect(chunks[0].content).toBe("");
  });

  it("ignores comment lines", async () => {
    const stream = createStream(
      `: this is a comment\ndata: ${chunk("Hi")}\ndata: [DONE]\n`,
    );

    const chunks = [];
    for await (const c of parseSSEStream(stream)) {
      chunks.push(c);
    }

    expect(chunks).toHaveLength(1);
  });

  it("stops at [DONE]", async () => {
    const stream = createStream(
      `data: ${chunk("A")}\ndata: [DONE]\ndata: ${chunk("B")}\n`,
    );

    const chunks = [];
    for await (const c of parseSSEStream(stream)) {
      chunks.push(c);
    }

    expect(chunks).toHaveLength(1);
    expect(chunks[0].content).toBe("A");
  });
});
