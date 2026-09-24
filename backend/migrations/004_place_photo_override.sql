-- A photo the project owns or has rights to, attached per place by an admin. It
-- takes precedence over the Google photo: it costs no API call, never expires,
-- and is served straight from wherever it is hosted.
--
-- Empty means "not set" — the render path falls back to Google's photo.

ALTER TABLE places
  ADD COLUMN photo_override_url         text NOT NULL DEFAULT '',
  ADD COLUMN photo_override_attribution text NOT NULL DEFAULT '';
