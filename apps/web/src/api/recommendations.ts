import type { Locale, LocationFailure } from "../i18n";
import type { RecommendationResponse, UserLocation } from "../types";

export class RefreshError extends Error {
  readonly detail: string;
  constructor(detail: string) {
    super("recommendation request failed");
    this.name = "RefreshError";
    this.detail = detail;
  }
}

export class LocationError extends Error {
  readonly code: LocationFailure;
  constructor(code: LocationFailure) {
    super(`location failure: ${code}`);
    this.name = "LocationError";
    this.code = code;
  }
}

export async function fetchRecommendations(query: string, locale: Locale, signal: AbortSignal, userLocation?: UserLocation): Promise<RecommendationResponse> {
  const response = await fetch(`${import.meta.env.VITE_API_BASE_URL ?? ""}/v1/recommendations`, {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, limit: 15, locale, ...(userLocation ? { user_location: userLocation } : {}) }),
  });
  const body = await response.json();
  if (!response.ok) throw new RefreshError(typeof body.error === "string" ? body.error : "");
  return body;
}

export type PlacePhoto = {
  /** Short-lived Google URL. Loaded by the browser straight from Google, never proxied or stored. */
  photoURL: string;
  attribution: { name: string; uri: string } | null;
};

/** Both photo endpoints answer with this same body, so the shape is validated in one place. */
function readPlacePhotoBody(body: unknown): PlacePhoto | null {
  if (typeof body !== "object" || body === null || !("photo_url" in body)) return null;
  const photoURL = body.photo_url;
  if (typeof photoURL !== "string" || photoURL.length === 0) return null;
  const raw = "attribution" in body ? body.attribution : null;
  const credit = raw && typeof raw === "object" ? raw : null;
  return {
    photoURL,
    attribution: credit
      ? { name: "name" in credit && typeof credit.name === "string" ? credit.name : "", uri: "uri" in credit && typeof credit.uri === "string" ? credit.uri : "" }
      : null,
  };
}

/**
 * Resolves to null — never throws, never logs — for every expected miss: a place
 * without a photo, an unknown id, an unconfigured Places key, an expired URL or a
 * dropped connection. A missing photo is normal, so the card simply keeps its art.
 */
export async function fetchPlacePhoto(googlePlaceID: string, signal: AbortSignal): Promise<PlacePhoto | null> {
  try {
    const response = await fetch(`${import.meta.env.VITE_API_BASE_URL ?? ""}/v1/places/${encodeURIComponent(googlePlaceID)}/photo`, { signal });
    if (!response.ok) return null;
    return readPlacePhotoBody(await response.json());
  } catch {
    return null;
  }
}

/**
 * Asks the API to mint a fresh Google URL after the browser found the cached one dead — the
 * counterpart to the long server-side cache. Same shape and the same silent rule as
 * `fetchPlacePhoto`: a place with no photo (404), a rate-limited refresh (429), a dropped
 * connection or an unparseable body all resolve to null, so the card keeps its art.
 */
export async function refreshPlacePhoto(googlePlaceID: string, signal: AbortSignal): Promise<PlacePhoto | null> {
  try {
    const response = await fetch(`${import.meta.env.VITE_API_BASE_URL ?? ""}/v1/places/${encodeURIComponent(googlePlaceID)}/photo/refresh`, { method: "POST", signal });
    if (!response.ok) return null;
    return readPlacePhotoBody(await response.json());
  } catch {
    return null;
  }
}

const nearbyPattern = /\b(?:near me|nearby|around me|sekitar saya|dekat saya|di sekitar sini|dekat sini)\b/i;
const namedLocationPattern = /\b(?:blok m|melawai|kemang|cipete|senopati|scbd|tebet|cilandak|lebak bulus|pondok labu|rawamangun|duren sawit|jakarta selatan|jaksel|south jakarta|jakarta timur|jaktim|east jakarta|jakarta pusat|jakpus|central jakarta|jakarta barat|jakbar|west jakarta|jakarta utara|jakut|north jakarta)\b/i;

export function needsUserLocation(query: string) {
  return nearbyPattern.test(query);
}

export function hasNamedLocation(query: string) {
  return namedLocationPattern.test(query);
}

export function requestUserLocation(): Promise<UserLocation> {
  if (!("geolocation" in navigator)) return Promise.reject(new LocationError("unsupported"));
  const { promise, resolve, reject } = Promise.withResolvers<UserLocation>();
  navigator.geolocation.getCurrentPosition(
    ({ coords }) => resolve({ lat: coords.latitude, lng: coords.longitude }),
    (cause) => {
      if (cause.code === cause.PERMISSION_DENIED) reject(new LocationError("denied"));
      else if (cause.code === cause.TIMEOUT) reject(new LocationError("timeout"));
      else reject(new LocationError("unknown"));
    },
    { enableHighAccuracy: false, timeout: 8000, maximumAge: 300000 },
  );
  return promise;
}
