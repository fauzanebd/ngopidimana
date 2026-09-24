import { useCallback, useEffect, useRef, useState } from "react";
import { fetchPlacePhoto, refreshPlacePhoto, type PlacePhoto } from "../api/recommendations";

/**
 * Loads the Google photo for one place. The bytes come straight from Google and the URL
 * expires, so nothing is persisted here: the result lives in component state and a failure
 * is indistinguishable from "this place has no photo".
 *
 * The browser is the only thing that can see a URL go dead, so the card calls `reportBroken`
 * when the <img> fails. That reports it to the API once and swaps in the fresh URL; if the
 * attempt yields nothing, or the same URL again, the card settles on its art — there is no
 * retry loop.
 */
export function usePlacePhoto(googlePlaceID: string | undefined) {
  const id = googlePlaceID?.trim() ?? "";
  const [resolved, setResolved] = useState<{ id: string; photo: PlacePhoto } | null>(null);
  const [brokenID, setBrokenID] = useState("");
  // One refresh per id per mount: "pending" while that attempt is in flight, "settled" once it
  // has answered. A second failure after that must not start another one.
  const refreshes = useRef(new Map<string, "pending" | "settled">());
  const controller = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!id) return;
    const current = new AbortController();
    controller.current = current;
    void fetchPlacePhoto(id, current.signal).then((photo) => {
      if (photo && !current.signal.aborted) setResolved({ id, photo });
    });
    return () => {
      current.abort();
      controller.current = null;
    };
  }, [id]);

  const reportBroken = useCallback(() => {
    if (!id) return;
    const attempt = refreshes.current.get(id);
    if (attempt) {
      if (attempt === "settled") setBrokenID(id);
      return;
    }
    const signal = controller.current?.signal;
    if (!signal || signal.aborted) return;
    const dead = resolved && resolved.id === id ? resolved.photo.photoURL : "";
    refreshes.current.set(id, "pending");
    void refreshPlacePhoto(id, signal).then((photo) => {
      refreshes.current.set(id, "settled");
      if (signal.aborted) return;
      // An unchanged URL would only be handed back to the same failing <img>, so treat it as
      // a miss and let the card keep its art.
      if (photo && photo.photoURL !== dead) setResolved({ id, photo });
      else setBrokenID(id);
    });
  }, [id, resolved]);

  const photo = resolved && resolved.id === id && brokenID !== id ? resolved.photo : null;
  return { photoURL: photo?.photoURL ?? null, attribution: photo?.attribution ?? null, reportBroken };
}
