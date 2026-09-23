-- Email magic-link authentication for the catalogue admin app.
--
-- Only addresses listed in allowed_contributors may request a sign-in link.
-- Tokens are stored as SHA-256 hashes: the raw token exists only in the email
-- that carries it and in the session cookie the browser holds.

CREATE TABLE allowed_contributors (
  email         text PRIMARY KEY CHECK (email = lower(email) AND position('@' IN email) > 1),
  display_name  text NOT NULL DEFAULT '',
  role          text NOT NULL DEFAULT 'contributor' CHECK (role IN ('contributor', 'owner')),
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz
);

CREATE TABLE admin_login_tokens (
  token_hash  bytea PRIMARY KEY,
  email       text NOT NULL REFERENCES allowed_contributors(email) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz
);

CREATE INDEX admin_login_tokens_email_created_at_idx ON admin_login_tokens (email, created_at DESC);

CREATE TABLE admin_sessions (
  token_hash   bytea PRIMARY KEY,
  email        text NOT NULL REFERENCES allowed_contributors(email) ON DELETE CASCADE,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  user_agent   text NOT NULL DEFAULT ''
);

CREATE INDEX admin_sessions_email_idx ON admin_sessions (email);

-- First owner. Further addresses are managed with the contributors CLI
-- (see README: Admin access).
INSERT INTO allowed_contributors (email, role) VALUES ('fauzanebd@gmail.com', 'owner')
ON CONFLICT (email) DO NOTHING;
