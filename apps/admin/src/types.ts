export type ContributorRole = "contributor" | "owner";

export type Contributor = {
  email: string;
  role: ContributorRole;
  display_name: string;
};

export type EvidenceField = {
  id?: string;
  key: string;
  value: string;
  source: string;
  source_id?: string;
  source_url?: string;
  excerpt?: string;
  captured_at?: string;
  published_at?: string;
  method?: string;
  extractor?: string;
  author_name?: string;
  author_url?: string;
  author_photo?: string;
  confidence: number;
  conflict: boolean;
  excluded?: boolean;
};

export type RunState = "draft" | "enriching" | "needs_review" | "published" | "stale" | "archived" | "failed";

// An image the project owns or has licensed, served in place of the Google photo. Absent
// means "leave whatever the published place already has"; an empty url means "explicitly none".
export type PhotoOverride = { url: string; attribution: string };

export type Run = {
  id: string;
  url: string;
  source: string;
  state: RunState;
  stage: string;
  progress: number;
  name: string;
  area?: string;
  confidence: number;
  issues: string[];
  warnings?: string[];
  fields: EvidenceField[];
  google_place_id?: string;
  photo_override?: PhotoOverride;
  created_at: string;
  updated_at: string;
};

export type RunAction = "publish" | "refresh" | "archive" | "review";
export type ManualEvidenceInput = { key: string; value: string };
export type FilterKey = "all" | RunState;

export type LocalizedText = { text: string; languageCode?: string };

export type GoogleReview = {
  name: string;
  relativePublishTimeDescription?: string;
  text: LocalizedText;
  originalText: LocalizedText;
  rating: number;
  authorAttribution: { displayName: string; uri?: string; photoUri?: string };
  publishTime?: string;
  flagContentUri?: string;
  googleMapsUri: string;
};

export type GooglePlace = {
  id: string;
  displayName: LocalizedText;
  formattedAddress?: string;
  googleMapsUri: string;
  rating?: number;
  userRatingCount?: number;
  reviews: GoogleReview[];
  attributions?: { provider: string; providerUri?: string }[];
};
