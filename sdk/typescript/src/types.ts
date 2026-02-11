/** A file attachment for message content. */
export interface FileContent {
  file_data: string; // base64-encoded
  filename: string;
}

/** A typed content part (text or file). */
export interface ContentPart {
  type: "text" | "file";
  text?: string;
  file?: FileContent;
}

/** Message content can be a plain string or an array of content parts. */
export type MessageContent = string | ContentPart[];

export interface Message {
  role: "system" | "user" | "assistant";
  content: MessageContent;
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
  /** File attachments as {filename, data} where data is a base64 string or Buffer. */
  files?: Array<{ filename: string; data: string | Uint8Array }>;
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
