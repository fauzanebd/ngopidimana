import { useEffect, useMemo, useRef, useState } from "react";
import { fetchRecommendations, hasNamedLocation, needsUserLocation, requestUserLocation } from "../api/recommendations";
import type { RecommendationResponse, UserLocation } from "../types";

export type RecommendationStatus = "idle" | "loading" | "ready" | "error";

export function useRecommendations(query: string) {
  const [data, setData] = useState<RecommendationResponse | null>(null);
  const [status, setStatus] = useState<RecommendationStatus>("idle");
  const [error, setError] = useState("");
  const [warning, setWarning] = useState("");
  const sequence = useRef(0);
  const userLocation = useRef<UserLocation | null>(null);
  const locationRequest = useRef<Promise<UserLocation> | null>(null);
  const locationProblem = useRef<Error | null>(null);

  useEffect(() => {
    const clean = query.trim();
    if (clean.length < 3) {
      setStatus("idle");
      return;
    }
    const controller = new AbortController();
    const currentSequence = ++sequence.current;
    const timer = window.setTimeout(async () => {
      setStatus("loading");
      setError("");
      setWarning("");
      try {
        const explicitNearby = needsUserLocation(clean);
        const shouldLocate = explicitNearby || !hasNamedLocation(clean);
        if (shouldLocate && !userLocation.current) {
          if (!locationRequest.current && (!locationProblem.current || explicitNearby)) {
            locationRequest.current = requestUserLocation()
              .then((location) => {
                userLocation.current = location;
                locationProblem.current = null;
                return location;
              })
              .catch((cause) => {
                const problem = cause instanceof Error ? cause : new Error("Your current location could not be determined.");
                locationProblem.current = problem;
                throw problem;
              })
              .finally(() => { locationRequest.current = null; });
          }
          if (locationRequest.current) {
            try {
              await locationRequest.current;
            } catch (cause) {
              if (explicitNearby) throw cause;
            }
          }
          if (!userLocation.current && locationProblem.current) {
            if (!controller.signal.aborted && sequence.current === currentSequence) {
              setWarning(`${locationProblem.current.message} Showing Jakarta-wide results instead.`);
            }
          }
        }
        const next = await fetchRecommendations(clean, controller.signal, userLocation.current || undefined);
        if (sequence.current === currentSequence) {
          setData(next);
          setStatus("ready");
        }
      } catch (cause) {
        if (controller.signal.aborted) return;
        setError(cause instanceof Error ? cause.message : "Could not refresh recommendations");
        setStatus("error");
      }
    }, 350);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [query]);

  const requirements = useMemo(
    () => data ? [
      ...(Array.isArray(data.interpretation.hard_constraints) ? data.interpretation.hard_constraints : []),
      ...(Array.isArray(data.interpretation.soft_preferences) ? data.interpretation.soft_preferences : []),
    ] : [],
    [data],
  );
  return { data, status, error, warning, requirements };
}
