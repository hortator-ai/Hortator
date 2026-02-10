export class HortatorError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly body?: string,
  ) {
    super(message);
    this.name = "HortatorError";
  }
}

export class AuthenticationError extends HortatorError {
  constructor(body?: string) {
    super("Authentication failed", 401, body);
    this.name = "AuthenticationError";
  }
}

export class RateLimitError extends HortatorError {
  constructor(body?: string) {
    super("Rate limit exceeded", 429, body);
    this.name = "RateLimitError";
  }
}

export function handleErrorResponse(status: number, body: string): never {
  if (status === 401) throw new AuthenticationError(body);
  if (status === 429) throw new RateLimitError(body);
  throw new HortatorError(`Request failed with status ${status}`, status, body);
}
