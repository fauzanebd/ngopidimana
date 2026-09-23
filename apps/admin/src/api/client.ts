// Every admin request goes through here: one place that knows the API base URL and
// that the session cookie is HttpOnly, so it must be sent with credentials.

const configured = import.meta.env.VITE_API_BASE_URL?.trim() || "";

// Empty in development, where Vite proxies /v1 to the local API.
export const apiBase = configured.replace(/\/+$/, "");

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
    const error = (body as { error?: unknown }).error;
    if (typeof error === "string" && error) return error;
  }
  return fallback;
}

export async function request<T>(path: string, init: ApiRequestInit = {}): Promise<T> {
  const { errorMessage: fallback = "The request failed", ...rest } = init;
  const response = await fetch(`${apiBase}${path}`, { ...rest, credentials: "include" });
  const body = response.status === 204 ? null : await response.json().catch(() => null);
  if (!response.ok) throw new ApiError(response.status, errorMessage(body, fallback));
  return body as T;
}
