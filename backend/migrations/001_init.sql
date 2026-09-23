CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE place_status AS ENUM ('draft', 'enriching', 'needs_review', 'published', 'stale', 'archived', 'failed');

CREATE TABLE places (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  status place_status NOT NULL DEFAULT 'draft',
  google_place_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE place_locations (
  place_id uuid PRIMARY KEY REFERENCES places(id) ON DELETE CASCADE,
  address text NOT NULL,
  area text NOT NULL,
  district text,
  geog geography(Point, 4326) NOT NULL
);

CREATE TABLE place_facts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  key text NOT NULL,
  bool_value boolean,
  numeric_value numeric,
  text_value text,
  confidence numeric(4,3) NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  observed_at timestamptz NOT NULL,
  verification_method text NOT NULL,
  UNIQUE (place_id, key)
);

CREATE TABLE place_scores (
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  dimension text NOT NULL,
  score numeric(4,3) NOT NULL CHECK (score BETWEEN 0 AND 1),
  confidence numeric(4,3) NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  model_version text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (place_id, dimension)
);

CREATE TABLE place_sources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  source_type text NOT NULL,
  url text NOT NULL,
  captured_at timestamptz NOT NULL,
  payload_json jsonb NOT NULL DEFAULT '{}'
);

CREATE TABLE evidence_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  source_id uuid NOT NULL REFERENCES place_sources(id) ON DELETE CASCADE,
  field_key text NOT NULL,
  excerpt_or_region text,
  confidence numeric(4,3) NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  conflict boolean NOT NULL DEFAULT false
);

CREATE TABLE place_media (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  storage_key text NOT NULL,
  rights_basis text NOT NULL,
  attribution text
);

CREATE TABLE external_links (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  platform text NOT NULL,
  url text NOT NULL,
  verified_at timestamptz,
  UNIQUE (place_id, platform, url)
);

CREATE TABLE menu_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  source_label text NOT NULL,
  normalized_name text NOT NULL,
  category text NOT NULL,
  attributes_json jsonb NOT NULL DEFAULT '{}',
  price numeric,
  currency char(3) NOT NULL DEFAULT 'IDR'
);

CREATE TABLE ingestion_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  dedupe_key text NOT NULL UNIQUE,
  source_url text NOT NULL,
  state place_status NOT NULL DEFAULT 'draft',
  errors jsonb NOT NULL DEFAULT '[]',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX places_google_place_uidx ON places (google_place_id) WHERE google_place_id IS NOT NULL;
CREATE INDEX place_locations_geo_gix ON place_locations USING GIST (geog);
CREATE INDEX place_facts_lookup_idx ON place_facts (key, bool_value, numeric_value);
CREATE INDEX place_scores_lookup_idx ON place_scores (dimension, score DESC);
CREATE INDEX place_sources_payload_gin ON place_sources USING GIN (payload_json);
