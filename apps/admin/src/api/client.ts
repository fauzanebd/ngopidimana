// Every admin request goes through here: one place that knows the API base URL and
// that the session cookie is HttpOnly, so it must be sent with credentials.

const configured = import.meta.env.VITE_API_BASE_URL?.trim() || "";

// Empty in development, where Vite proxies /v1 to the local API.
export const apiBase = configured.replace(/\/+$/, "");

const requestTimeoutMs = 15_000;
const healthTimeoutMs = 5_000;

export type ApiRequestInit = RequestInit & { errorMessage?: string };

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

function errorMessage(body: unknown, fallback: string) {
  if (body && typeof body === "object" && "error" in body) {
    const { error } = body;
    if (typeof error === "string" && error) return error;
  }
  return fallback;
}

// A connection that blackholes — a DNS answer pointing at an unroutable address, a
// dropped route — never rejects on its own, so without a deadline the queue sits on a
// spinner forever and never reports anything. The timeout turns that into a failure the
// UI can show and retry. status 0 marks "never reached the API" as distinct from an
// HTTP error.
function unreachable(cause: unknown) {
  if (cause instanceof DOMException && cause.name === "TimeoutError") return `The API did not respond within ${requestTimeoutMs / 1000} seconds`;
  return "Could not reach the API";
}

export async function request<T>(path: string, init: ApiRequestInit = {}): Promise<T> {
  const { errorMessage: fallback = "The request failed", signal, ...rest } = init;
  let response: Response;
  try {
    response = await fetch(`${apiBase}${path}`, { ...rest, credentials: "include", signal: signal ?? AbortSignal.timeout(requestTimeoutMs) });
  } catch (cause) {
    throw new ApiError(0, unreachable(cause));
  }
  // Endpoints with no response body are called as request<void>.
  if (response.status === 204) return undefined as T;
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    // A body that cannot be read is a failure the caller has to hear about. Swallowing
    // it into null used to surface as "Cannot read properties of null (reading 'runs')"
    // — an opaque crash instead of the retry banner — when a response was cut short in
    // transit (a proxy or connection dropping the body mid-flight).
    throw new ApiError(response.status, response.ok ? "The API response was cut short" : fallback);
  }
  if (!response.ok) throw new ApiError(response.status, errorMessage(body, fallback));
  return body as T;
}

// The header's connectivity claim has to come from the API itself: "the last request did
// not throw" is not the same thing, because a hung request throws nothing at all.
export async function checkHealth(): Promise<boolean> {
  try {
    const response = await fetch(`${apiBase}/healthz`, { signal: AbortSignal.timeout(healthTimeoutMs), cache: "no-store" });
    return response.ok;
  } catch {
    return false;
  }
}
