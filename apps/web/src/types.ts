export type Requirement = {
  key: string;
  label: string;
  kind: "hard" | "soft";
  weight: number;
  confidence: number;
};

export type Recommendation = {
  place_id: string;
  name: string;
  area: string;
  address: string;
  match_score: number;
  result_confidence: number;
  matched_on: string[];
  caveats: string[];
  maps_url: string;
  review_links: { platform: string; url: string }[];
  price_min: number;
  price_max: number;
  distance_km: number;
  open_until: string;
  open_24_hours: boolean;
  freshness_days: number;
  description: string;
  accent: string;
  evidence_summary: string;
  score_components: Record<string, number>;
  /** Google Places id, present only for places sourced from Google. */
  google_place_id?: string;
};

export type RecommendationResponse = {
  results: Recommendation[];
  interpretation: {
    hard_constraints: Requirement[];
    soft_preferences: Requirement[];
    location: string;
    location_mode: "default" | "named" | "nearby";
    budget?: number;
    summary: string;
    confidence: number;
    provider: "jev" | "deterministic";
    model?: string;
    latency_ms: number;
    fallback_reason?: string;
  };
  request_id: string;
  elapsed_ms: number;
  catalogue_size: number;
};

export type UserLocation = { lat: number; lng: number };
