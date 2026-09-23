import { useCallback, useEffect, useRef, useState } from "react";
import { fetchSession, requestLink, signOut as endSession, verifyToken } from "../api/auth";
import type { Contributor } from "../types";

export type SessionStatus = "checking" | "signed-out" | "signed-in";

const INVALID_LINK = "This sign-in link is invalid or has expired";

// A null token means "no magic link in the URL": the ordinary session lookup runs instead.
function bootstrap(callbackToken: string | null): Promise<Contributor | null> {
  if (callbackToken === null) return fetchSession();
  return callbackToken ? verifyToken(callbackToken) : Promise.reject(new Error("The sign-in link carried no token"));
}

export function useSession(callbackToken: string | null = null) {
  const [contributor, setContributor] = useState<Contributor | null>(null);
  const [status, setStatus] = useState<SessionStatus>("checking");
  const [signingIn, setSigningIn] = useState(false);
  const [signInError, setSignInError] = useState("");
  const [sentTo, setSentTo] = useState("");
  // Held in a ref so React's development StrictMode remount reuses the one request rather
  // than spending the single-use token twice.
  const bootstrapRequest = useRef<Promise<Contributor | null> | null>(null);

  const adopt = useCallback((next: Contributor | null) => {
    setContributor(next);
    setStatus(next ? "signed-in" : "signed-out");
  }, []);

  useEffect(() => {
    bootstrapRequest.current ??= bootstrap(callbackToken);
    bootstrapRequest.current.then(adopt, () => {
      if (callbackToken !== null) setSignInError(INVALID_LINK);
      adopt(null);
    });
  }, [adopt, callbackToken]);

  const signIn = useCallback(async (email: string) => {
    setSigningIn(true); setSignInError("");
    try {
      await requestLink(email);
      setSentTo(email);
    } catch (cause) {
      setSignInError(cause instanceof Error ? cause.message : "We could not send that sign-in link");
    } finally { setSigningIn(false); }
  }, []);

  const signOut = useCallback(async () => {
    try { await endSession(); } catch { /* the local session is dropped even if the request fails */ }
    setContributor(null); setStatus("signed-out"); setSentTo(""); setSignInError("");
  }, []);

  const clearSentTo = useCallback(() => setSentTo(""), []);

  return { contributor, status, signIn, signingIn, signInError, sentTo, clearSentTo, signOut };
}
