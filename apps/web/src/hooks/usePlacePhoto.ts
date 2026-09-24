import { useCallback, useEffect, useState } from "react";
import { fetchPlacePhoto, type PlacePhoto } from "../api/recommendations";

/**
 * Loads the Google photo for one place. The bytes come straight from Google and the
 * URL expires, so nothing is persisted here: the result lives in component state,
 * a failure is indistinguishable from "this place has no photo", and the card is
 * expected to call `reportBroken` when the <img> fails so it can fall back to art.
 */
export function usePlacePhoto(googlePlaceID: string | undefined) {
  const id = googlePlaceID?.trim() ?? "";
  const [resolved, setResolved] = useState<{ id: string; photo: PlacePhoto } | null>(null);
  const [brokenID, setBrokenID] = useState("");

  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    void fetchPlacePhoto(id, controller.signal).then((photo) => {
      if (photo && !controller.signal.aborted) setResolved({ id, photo });
    });
    return () => controller.abort();
  }, [id]);

  const reportBroken = useCallback(() => setBrokenID(id), [id]);
  const photo = resolved && resolved.id === id && brokenID !== id ? resolved.photo : null;
  return { photoURL: photo?.photoURL ?? null, attribution: photo?.attribution ?? null, reportBroken };
}
