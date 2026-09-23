import { ApiError, request } from "./client";
import type { Contributor } from "../types";

type SessionBody = { contributor: Contributor };

export async function requestLink(email: string): Promise<void> {
  await request<{ status: string }>("/v1/auth/request-link", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
    errorMessage: "We could not send that sign-in link",
  });
}

export async function verifyToken(token: string): Promise<Contributor> {
  const body = await request<SessionBody>("/v1/auth/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
    errorMessage: "This sign-in link is invalid or has expired",
  });
  return body.contributor;
}

export async function fetchSession(): Promise<Contributor | null> {
  try {
    const body = await request<SessionBody>("/v1/auth/session");
    return body.contributor;
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 401) return null;
    throw cause;
  }
}

export async function signOut(): Promise<void> {
  await request<void>("/v1/auth/logout", { method: "POST", errorMessage: "We could not end the session" });
}
