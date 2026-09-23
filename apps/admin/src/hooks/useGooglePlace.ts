import { useEffect, useRef, useState } from "react";
import { getGooglePlace } from "../api/ingestion";
import type { GooglePlace } from "../types";

export function useGooglePlace(runID: string) {
  const [place, setPlace] = useState<GooglePlace | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const activeRunID = useRef(runID);

  useEffect(() => {
    activeRunID.current = runID;
    setPlace(null);
    setError("");
    setLoading(false);
  }, [runID]);

  async function load() {
    const requestedRunID = runID;
    setLoading(true);
    setError("");
    try {
      const next = await getGooglePlace(requestedRunID);
      if (activeRunID.current === requestedRunID) setPlace(next);
    } catch (cause) {
      if (activeRunID.current === requestedRunID) {
        setError(cause instanceof Error ? cause.message : "Could not load Google Maps reviews");
      }
    } finally {
      if (activeRunID.current === requestedRunID) setLoading(false);
    }
  }

  return { place, loading, error, load };
}
