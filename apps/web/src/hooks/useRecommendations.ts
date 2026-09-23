import { useEffect, useMemo, useRef, useState } from "react";
import { LocationError, fetchRecommendations, hasNamedLocation, needsUserLocation, requestUserLocation } from "../api/recommendations";
import { useI18n, type LocationFailure } from "../i18n";
import type { RecommendationResponse, UserLocation } from "../types";

export type RecommendationStatus = "idle" | "loading" | "ready" | "error";

export function useRecommendations(query: string) {
  const { locale, messages } = useI18n();
  const [data, setData] = useState<RecommendationResponse | null>(null);
  const [status, setStatus] = useState<RecommendationStatus>("idle");
  const [error, setError] = useState("");
  const [warning, setWarning] = useState("");
  const sequence = useRef(0);
  const userLocation = useRef<UserLocation | null>(null);
  const locationRequest = useRef<Promise<UserLocation> | null>(null);
  const locationProblem = useRef<LocationFailure | null>(null);

  useEffect(() => {
    const clean = query.trim();
    if (clean.length < 3) {
      setStatus("idle");
      setData(null);
      setError("");
      setWarning("");
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
              .catch((cause: unknown) => {
                locationProblem.current = cause instanceof LocationError ? cause.code : "unknown";
                throw cause;
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
              setWarning(messages.locationFallback(messages.location[locationProblem.current]));
            }
          }
        }
        const next = await fetchRecommendations(clean, locale, controller.signal, userLocation.current || undefined);
        if (sequence.current === currentSequence) {
          setData(next);
          setStatus("ready");
        }
      } catch (cause) {
        if (controller.signal.aborted) return;
        setError(cause instanceof LocationError ? messages.location[cause.code] : messages.refreshFailed);
        setStatus("error");
      }
    }, 350);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [query, locale, messages]);

  const requirements = useMemo(
    () => data ? [
      ...(Array.isArray(data.interpretation.hard_constraints) ? data.interpretation.hard_constraints : []),
      ...(Array.isArray(data.interpretation.soft_preferences) ? data.interpretation.soft_preferences : []),
    ] : [],
    [data],
  );
  return { data, status, error, warning, requirements };
}
