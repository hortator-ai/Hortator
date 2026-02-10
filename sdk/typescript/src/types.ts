export interface Message {
  role: "system" | "user" | "assistant";
  content: string;
}

export interface Budget {
  max_cost_usd?: string;
  max_tokens?: number;
}

export interface Usage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface RunResult {
  id: string;
  content: string;
  finish_reason: string;
  model: string;
  usage?: Usage;
}

export interface StreamChunk {
  id: string;
  content: string;
  finish_reason: string | null;
  model: string;
}

export interface ModelInfo {
  id: string;
  object: string;
  created: number;
  owned_by: string;
}

export interface RequestOptions {
  role?: string;
  capabilities?: string[];
  tier?: string;
  budget?: Budget;
  temperature?: number;
  max_tokens?: number;
}

export interface ClientOptions {
  baseUrl: string;
  apiKey: string;
  timeout?: number;
}

// Internal API types
export interface ChatCompletionRequest {
  model: string;
  messages: Message[];
  stream?: boolean;
  temperature?: number;
  max_tokens?: number;
  x_capabilities?: string[];
  x_tier?: string;
  x_budget?: Budget;
}

export interface ChatCompletionResponse {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: { index: number; message: Message; finish_reason: string }[];
  usage?: Usage;
}

export interface ChatCompletionChunk {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: { index: number; delta: { content?: string }; finish_reason: string | null }[];
}

export interface ModelsResponse {
  object: string;
  data: ModelInfo[];
}
