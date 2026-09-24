-- The photo reference for a published place, captured while ingestion is already
-- making a Place Details call (so it costs nothing extra there).
--
-- Only the *reference* and its attribution are kept — never pixels. Google's
-- Places policies forbid caching or storing place content, and the reference is
-- treated as a hint that may have expired: the API uses it for the cheap media
-- call and falls back to a fresh Place Details lookup when Google rejects it.

ALTER TABLE places
  ADD COLUMN photo_name            text NOT NULL DEFAULT '',
  ADD COLUMN photo_attribution_name text NOT NULL DEFAULT '',
  ADD COLUMN photo_attribution_uri  text NOT NULL DEFAULT '',
  ADD COLUMN photo_refreshed_at    timestamptz;
