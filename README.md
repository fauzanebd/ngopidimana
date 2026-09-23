# Where to WFC

A coffee-place recommendation MVP with a public discovery app, a separate catalogue-admin app, a Go API, and an asynchronous URL-ingestion worker.

## Architecture

```text
apps/
  web/                       public recommendations UI
    src/api                  HTTP client
    src/hooks                recommendation state and debounce lifecycle
    src/components           focused UI components
  admin/                     ingestion and evidence-review UI
    src/api                  ingestion HTTP client
    src/hooks                queue polling and mutations
    src/components           queue, pipeline, and review components
backend/
  cmd/api                    HTTP API binary
  cmd/worker                 Asynq worker binary
  internal/config            environment configuration
  internal/httpapi           routes, validation, JSON, middleware
  internal/recommendation    Jev interpretation and ranking domain
  internal/ingestion         Redis store, Asynq tasks, extraction worker
  internal/catalogue         reviewed-run publication and Postgres search catalogue
  migrations                 PostgreSQL/PostGIS schema
```

The API validates a submitted URL, persists an ingestion run in Redis, and enqueues an Asynq task. The worker fetches public HTTP(S) sources with private-network/SSRF protection, extracts page metadata and JSON-LD evidence, asks Exa for a bounded set of corroborating sources, and optionally sends the bounded source bundle to OpenRouter for strict structured extraction. Exa discovery prioritizes indexed Google Maps review context, then TikTok text context, with other venue-specific web reviews as backfill. When Google Places is configured, the worker resolves and persists the stable Place ID, fetches up to five official reviews, stores each attributed excerpt with its direct Google Maps link, and includes those review documents in cited OpenRouter extraction. The admin can also request the current Place Details response on demand for comparison. Local validation rejects uncited claims, normalizes exact values, detects material cross-source conflicts, and then moves the run to review.

Redis is queue infrastructure and ingestion-run storage. PostgreSQL/PostGIS stores the reviewed, published catalogue used by recommendation ranking. It is not used as a Jev response cache; every recommendation query makes a fresh Jev request when Jev is configured.

## Run locally

Requirements: Go 1.26.5 or newer, Node.js 20 or newer, pnpm 9, Docker Desktop.

```bash
pnpm install
cp .env.example .env

# PostGIS and Redis
pnpm infra:up

# terminal 1 — API on :8080
pnpm dev:api

# terminal 2 — Asynq ingestion worker
pnpm dev:worker

# terminal 3 — public app :5173 and admin app :5174
pnpm dev
```

`DATABASE_URL` defaults to the local Compose Postgres instance. Add `TYPESAFE_API_KEY` to `.env` to enable Jev. Add `EXA_API_KEY` to enable ingestion source discovery; `EXA_MAX_RESULTS` defaults to five. Add both `OPENROUTER_API_KEY` and an `OPENROUTER_MODEL` that supports structured outputs to enable evidence extraction. Add `GOOGLE_PLACES_API_KEY` to resolve Place IDs, addresses, coordinates, hours, price ranges, and official review evidence during ingestion. Enable **Places API (New)** and billing for that Google Cloud key. Without those optional providers, deterministic HTML and JSON-LD ingestion continues normally, but publication requires a stable Google Place ID and searchable location evidence. Both Go binaries automatically load the root `.env` when launched through the root pnpm scripts or from `backend/`.

Open:

- Public app: http://127.0.0.1:5173
- Admin app: http://127.0.0.1:5174
- API health: http://127.0.0.1:8080/healthz
- Redis: `127.0.0.1:6380`
- PostgreSQL/PostGIS: `127.0.0.1:5432`

Stop infrastructure without deleting its data:

```bash
pnpm infra:down
```

## Checks

```bash
go -C backend test ./...
pnpm typecheck
pnpm build
docker compose config --quiet
```

## Recommendation API

`POST /v1/recommendations`

```json
{
  "query": "quiet WFC in Blok M, must have outlets, under 60k, harus 24 jam",
  "user_location": { "lat": -6.244, "lng": 106.8 },
  "limit": 15
}
```

The online path uses the PRD-specified `github.com/atharvamhaske/typesafe-sdk-go`, pinned to commit `1edab9b13089b2d45558e580e7b0ed546be4f02f`. It sends one bounded, batched `SystemOne` request. Budget, location, and exact deterministic constraints such as `24 jam` are normalized locally. Jev failures fall back to deterministic interpretation.

## Ingestion API

- `GET /v1/admin/ingestion-runs`
- `POST /v1/admin/ingestion-runs` with `{ "url": "https://…" }`
- `PATCH /v1/admin/ingestion-runs/{id}` with `review`, `publish`, `refresh`, or `archive`
- `DELETE /v1/admin/ingestion-runs/{id}`
- `GET /v1/admin/ingestion-runs/{id}/google-place` — uncached live Google Place Details for admin display
- `POST /v1/admin/ingestion-runs/{id}/manual-evidence` with `{ "key": "Seating comfort", "value": "…" }`
- `DELETE /v1/admin/ingestion-runs/{id}/manual-evidence/{evidenceID}`
- `PATCH /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}` — replace with an attributed manual correction
- `DELETE /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}` — exclude sourced evidence from catalogue facts and matching
- `POST /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}/restore`
- `POST /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}/choose` — choose the winning value for a conflict group

The current extractor supports ordinary server-rendered HTML metadata, JSON-LD, bounded visible page text, and identity decoding from resolved Google Maps place URLs. Exa runs separate Google Maps, TikTok, and general-web review searches in priority order; only venue-matching results with an extractable excerpt enter the evidence bundle. Indexed excerpts are not treated as a guarantee that every source review was available. The official OpenRouter Go SDK is pinned and requests strict JSON Schema with provider routing restricted to endpoints that support the supplied parameters. Every model claim must cite a known source ID and a verbatim excerpt that is checked locally before acceptance. Boolean values, prices, coordinates, and URLs are normalized deterministically; independent conflicting claims for material fields block publication. Provider failures are non-blocking and appear as warnings.

Clicking **Publish** transactionally projects the reviewed record, provenance, evidence, facts, scores, links, menu items, and location into Postgres. The recommendation service reads only those published rows; it does not serve the fictional development seed catalogue. Publishing is rejected when identity, stable Google Place ID, area, address, or coordinates are missing. Archiving or deleting the admin record also removes a previously published catalogue place from search. Manual evidence is clearly marked `Manually added`; changing it returns a published record to review so you explicitly republish the revision. Sourced evidence is never silently rewritten: an admin can select a winning value, exclude/restore an item, or create a manual correction while retaining the original in the audit trail.

JavaScript-only pages, PDFs, images/OCR, and TikTok audio/video analysis remain separate provider milestones.

## Data and security boundaries

- Source fetching rejects localhost, private, link-local, multicast, and credential-bearing URLs; redirects are checked again.
- Source responses are capped at 2 MiB, Exa responses at 1 MiB, model input is character-bounded, and worker tasks have a 150-second timeout with three retries.
- Model output is never accepted as evidence unless its source ID exists and its excerpt occurs in the supplied source document.
- Admin authentication and role-based access are still required before exposing the admin app publicly.
- Google review evidence retains the author name/profile/photo when returned, the official review URL, publication time, verbatim excerpt, and Google Maps attribution. OpenRouter-derived claims must cite one of those stored excerpts exactly.
- The fictional seed catalogue remains test-only; the running API uses published Postgres records.
