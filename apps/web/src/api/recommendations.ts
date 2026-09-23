import type { RecommendationResponse, UserLocation } from "../types";

export async function fetchRecommendations(query: string, signal: AbortSignal, userLocation?: UserLocation): Promise<RecommendationResponse> {
  const response = await fetch("/v1/recommendations", {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, limit: 15, ...(userLocation ? { user_location: userLocation } : {}) }),
  });
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || "Could not refresh recommendations");
  return body;
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
  if (!("geolocation" in navigator)) {
    return Promise.reject(new Error("This browser does not support location-based searches."));
  }
  return new Promise((resolve, reject) => {
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => resolve({ lat: coords.latitude, lng: coords.longitude }),
      (cause) => {
        if (cause.code === cause.PERMISSION_DENIED) reject(new Error("Location access is needed for “near me” searches. Allow it in your browser, then try again."));
        else if (cause.code === cause.TIMEOUT) reject(new Error("Your location request timed out. Try the search again."));
        else reject(new Error("Your current location could not be determined."));
      },
      { enableHighAccuracy: false, timeout: 8000, maximumAge: 300000 },
    );
  });
}
