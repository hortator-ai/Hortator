import { describe, it, expect, vi, beforeEach } from "vitest";
import { HortatorClient } from "../src/client.js";
import { AuthenticationError, RateLimitError, HortatorError } from "../src/errors.js";

const BASE_URL = "http://localhost:8080";
const API_KEY = "test-key";

function mockFetch(data: unknown, status = 200) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
    text: () => Promise.resolve(JSON.stringify(data)),
    body: null,
  });
}

function sseResponse(chunks: string[]) {
  const text = chunks.join("\n") + "\n";
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(text));
      controller.close();
    },
  });
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    body: stream,
  });
}

const COMPLETION_RESPONSE = {
  id: "chatcmpl-123",
  object: "chat.completion",
  created: 1700000000,
  model: "hortator/legionary",
  choices: [{ index: 0, message: { role: "assistant", content: "Hello!" }, finish_reason: "stop" }],
  usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
};

describe("HortatorClient", () => {
  let client: HortatorClient;

  beforeEach(() => {
    client = new HortatorClient({ baseUrl: BASE_URL, apiKey: API_KEY });
  });

  it("run() sends prompt and returns RunResult", async () => {
    const fetchMock = mockFetch(COMPLETION_RESPONSE);
    vi.stubGlobal("fetch", fetchMock);

    const result = await client.run("Hi");

    expect(result.content).toBe("Hello!");
    expect(result.finish_reason).toBe("stop");
    expect(result.usage?.total_tokens).toBe(15);

    const call = fetchMock.mock.calls[0];
    expect(call[0]).toBe(`${BASE_URL}/v1/chat/completions`);
    const body = JSON.parse(call[1].body);
    expect(body.model).toBe("hortator/legionary");
    expect(body.messages[0].content).toBe("Hi");
  });

  it("chat() sends messages array", async () => {
    const fetchMock = mockFetch(COMPLETION_RESPONSE);
    vi.stubGlobal("fetch", fetchMock);

    const result = await client.chat([
      { role: "system", content: "You are helpful" },
      { role: "user", content: "Hi" },
    ]);

    expect(result.content).toBe("Hello!");
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body.messages).toHaveLength(2);
  });

  it("run() with custom role", async () => {
    const fetchMock = mockFetch(COMPLETION_RESPONSE);
    vi.stubGlobal("fetch", fetchMock);

    await client.run("Hi", { role: "centurion" });
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body.model).toBe("hortator/centurion");
  });

  it("run() passes capabilities, tier, budget", async () => {
    const fetchMock = mockFetch(COMPLETION_RESPONSE);
    vi.stubGlobal("fetch", fetchMock);

    await client.run("Hi", {
      capabilities: ["code", "search"],
      tier: "premium",
      budget: { max_cost_usd: "1.00", max_tokens: 1000 },
    });

    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body.x_capabilities).toEqual(["code", "search"]);
    expect(body.x_tier).toBe("premium");
    expect(body.x_budget).toEqual({ max_cost_usd: "1.00", max_tokens: 1000 });
  });

  it("listModels() returns model list", async () => {
    const fetchMock = mockFetch({
      object: "list",
      data: [{ id: "hortator/legionary", object: "model", created: 1700000000, owned_by: "hortator" }],
    });
    vi.stubGlobal("fetch", fetchMock);

    const models = await client.listModels();
    expect(models).toHaveLength(1);
    expect(models[0].id).toBe("hortator/legionary");

    const call = fetchMock.mock.calls[0];
    expect(call[1].method).toBe("GET");
  });

  it("stream() yields chunks", async () => {
    const fetchMock = sseResponse([
      `data: ${JSON.stringify({ id: "c1", model: "hortator/legionary", choices: [{ index: 0, delta: { content: "Hel" }, finish_reason: null }] })}`,
      `data: ${JSON.stringify({ id: "c1", model: "hortator/legionary", choices: [{ index: 0, delta: { content: "lo!" }, finish_reason: "stop" }] })}`,
      "data: [DONE]",
    ]);
    vi.stubGlobal("fetch", fetchMock);

    const chunks: any[] = [];
    for await (const chunk of client.stream("Hi")) {
      chunks.push(chunk);
    }

    expect(chunks).toHaveLength(2);
    expect(chunks[0].content).toBe("Hel");
    expect(chunks[1].content).toBe("lo!");
    expect(chunks[1].finish_reason).toBe("stop");
  });

  it("throws AuthenticationError on 401", async () => {
    vi.stubGlobal("fetch", mockFetch({ error: "unauthorized" }, 401));
    await expect(client.run("Hi")).rejects.toThrow(AuthenticationError);
  });

  it("throws RateLimitError on 429", async () => {
    vi.stubGlobal("fetch", mockFetch({ error: "rate limited" }, 429));
    await expect(client.run("Hi")).rejects.toThrow(RateLimitError);
  });

  it("throws HortatorError on 500", async () => {
    vi.stubGlobal("fetch", mockFetch({ error: "internal" }, 500));
    await expect(client.run("Hi")).rejects.toThrow(HortatorError);
  });

  it("sends Authorization header", async () => {
    const fetchMock = mockFetch(COMPLETION_RESPONSE);
    vi.stubGlobal("fetch", fetchMock);

    await client.run("Hi");
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe("Bearer test-key");
  });
});
