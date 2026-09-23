# Production deployment runbook

Everything below is meant to be copy-pasted. Commands are run **from the
repository root** on the VPS unless stated otherwise, and always name the
compose file explicitly:

```bash
docker compose -f deploy/compose.prod.yaml <command>
```

## 1. What runs where

```
                                your VPS - one compose project ("wheretowfc")
                              ┌──────────────────────────────────────────────┐
   Cloudflare Pages           │  wfc-caddy :80 :443   ← the only host ports  │
   ┌─────────────────────┐    │        │                                     │
   │ wfc-web             │    │        ▼        network "wfc-prod"           │
   │   apps/web/dist     │    │   wfc-api :8080 ─────▶ wfc-redis :6379       │
   ├─────────────────────┤    │        │                                     │
   │ wfc-admin           │    │        └──────────────▶ wfc-db :5432 (PostGIS)│
   │   apps/admin/dist   │    │                                              │
   └──────────┬──────────┘    │   wfc-worker ─────────▶ wfc-redis, wfc-db    │
              │               └──────────────────────────────────────────────┘
              │
   browser ───┼──▶ https://api.<domain> ──▶ wfc-caddy ──▶ wfc-api
              └──  VITE_API_BASE_URL, baked in at build time
```

* One compose project (`wheretowfc`) on the VPS: `db` (PostGIS), `redis`, `api`,
  `worker`, `caddy`. Every container is named `wfc-*`.
* `db` and `redis` publish **no host ports**: they are reachable only from the
  compose network. Only Caddy binds `80`/`443`.
* The two frontends are static Vite builds hosted on Cloudflare Pages. They call
  the API at the absolute `VITE_API_BASE_URL` baked in at build time.
* Migrations are applied **manually** (section 4). The compose file deliberately
  does not mount `backend/migrations` into `/docker-entrypoint-initdb.d`.

Paths referenced below: `deploy/compose.prod.yaml`, `deploy/Caddyfile`,
`deploy/.env.production.example` → copied to `.env`, `backend/Dockerfile`.

## 2. VPS preparation

Ubuntu 22.04/24.04, root or a sudo user.

```bash
apt-get update
apt-get install -y ca-certificates curl git ufw
curl -fsSL https://get.docker.com | sh
docker --version
docker compose version          # needs the Compose v2 plugin (docker compose, not docker-compose)
```

Firewall: only SSH, HTTP and HTTPS are reachable. The datastores are never
exposed, so they need no rule.

```bash
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp               # HTTP/3 (QUIC)
ufw --force enable
ufw status
```

Optional but recommended for small VPSes (the Go builds are the memory spike):

```bash
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

### DNS records (create these before the first start)

| Record | Name | Value | Proxy |
| --- | --- | --- | --- |
| `A` (and `AAAA` if you have IPv6) | `api.<domain>` | the VPS IPv4/IPv6 | **DNS only** (grey cloud) |
| `CNAME` | the hostnames you attach to the two Pages projects | `<project>.pages.dev` - the Pages dashboard creates this record for you when the zone is in the same Cloudflare account, otherwise create it by hand | proxied (orange) |

`api.<domain>` must resolve to this VPS **before** `docker compose up`: Caddy
requests its Let's Encrypt certificate during the first start (HTTP-01/TLS-ALPN)
and will retry with backoff until DNS is correct. Leaving `api.<domain>`
grey-clouded is the simplest setup (Caddy terminates TLS itself). If you proxy it
through Cloudflare instead, set the SSL/TLS mode to **Full (strict)** so the
origin certificate is still validated.

## 3. First deploy

```bash
git clone <your-repo-url> /srv/wheretowfc
cd /srv/wheretowfc

# 1. Production environment (contains real secrets - keep it out of git).
cp deploy/.env.production.example .env
chmod 600 .env
$EDITOR .env
```

In `.env` you must at minimum replace:

* `POSTGRES_PASSWORD` **and** the same password inside `DATABASE_URL`
  (generate one: `openssl rand -base64 32`);
* `CORS_ORIGIN` (section 6);
* `ADMIN_APP_URL`, `ADMIN_COOKIE_DOMAIN`, `ADMIN_COOKIE_SAMESITE` (section 7);
* `SMTP_*` (section 7) - without `SMTP_HOST` no magic-link mail is sent;
* `OPENROUTER_SITE_URL` and the API keys you actually use.

Then set your API hostname in the Caddyfile:

```bash
sed -i 's/api\.example\.com/api.YOURDOMAIN.com/g' deploy/Caddyfile
grep -n 'YOURDOMAIN' deploy/Caddyfile    # sanity check the replacement
```

Note that the `db` service reads `POSTGRES_DB`/`POSTGRES_USER`/`POSTGRES_PASSWORD`
from this same `.env` (the root file, via `env_file: ../.env`), which keeps the
database password in exactly one place: rotating it means editing
`POSTGRES_PASSWORD` and the password inside `DATABASE_URL` in the same file. The
password is *not* duplicated in `deploy/compose.prod.yaml`.

Validate both files before starting anything:

```bash
docker compose -f deploy/compose.prod.yaml config -q
docker run --rm -v "$PWD/deploy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
```

Start the datastores first, apply the schema, then bring up the rest:

```bash
# 1. Postgres + Redis (first start initialises the cluster from .env).
docker compose -f deploy/compose.prod.yaml up -d db redis
docker compose -f deploy/compose.prod.yaml ps

# 2. Apply migrations - see section 4 (fresh database).
#    The API is not running yet: it must never see an empty schema.

# 3. Build the Go image and start the API, worker and Caddy.
docker compose -f deploy/compose.prod.yaml up -d --build

# 4. Verify.
docker compose -f deploy/compose.prod.yaml ps
docker compose -f deploy/compose.prod.yaml logs --tail=50 api
curl -fsS https://api.YOURDOMAIN.com/healthz
```

`/healthz` answers `{"status":"ok","redis":"ok","catalogue_size":<n>}`.
`catalogue_size: 0` is expected on a brand-new database; it grows as ingestion
publishes places. A `503` with `{"status":"degraded","redis":"unavailable"}`
means Redis is unreachable.

The worker logs which ingestion integrations are enabled on startup - check it:

```bash
docker compose -f deploy/compose.prod.yaml logs --tail=50 worker
```

## 4. Migrations

There is **no migration runner**. `backend/migrations/*.sql` are plain SQL files
applied in lexical order (`001_…`, `002_…`, …) and recorded nowhere, so the
operator tracks what has been applied. The files are piped from the host into
`psql` inside the `db` container, so nothing needs to be mounted.

> **Never re-apply `001_init.sql` to an existing database.** It runs plain
> `CREATE TABLE`/`CREATE TYPE` statements, so a second run aborts with
> `type "place_status" already exists`. With `ON_ERROR_STOP=1` it stops at the
> first error instead of half-migrating, but you still lose time.
> **Always take a backup (section 9) before applying migrations.**

### 4a. Fresh database

Apply every file, in order, and stop at the first failure:

```bash
for f in backend/migrations/*.sql; do
  echo "==> applying $f"
  docker compose -f deploy/compose.prod.yaml exec -T db \
    sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -f -' < "$f" || break
done
```

Single-file variant (same mechanism, useful for one new migration):

```bash
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -f -' \
  < backend/migrations/001_init.sql
```

`sh -lc` inside the container means `$POSTGRES_USER`/`$POSTGRES_DB` come from the
same `.env` the cluster was initialised with - no names to retype.

### 4b. Existing database

List what is on the server, then compare with the files in `backend/migrations/`:

```bash
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\dt"'

ls backend/migrations/
```

Record the applied filenames somewhere durable (a ticket, or
`deploy/applied-migrations.txt` on the VPS) so the next operator does not have to
guess. Then apply **only** the new files, one at a time, in order:

```bash
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -f -' \
  < backend/migrations/002_admin_auth.sql      # ← the new file only
```

Then rebuild/restart the application containers so they run against the new
schema:

```bash
docker compose -f deploy/compose.prod.yaml up -d --build
```

### 4c. Checking a migration ran

```bash
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\d places"'
```

## 5. Cloudflare Pages (both frontends)

Create **two separate Pages projects** in the same Cloudflare account, both
connected to this repository (Workers & Pages → Create → Pages → Connect to Git).
Build settings are identical except for the build command and output directory:

| Setting | Public app | Admin app |
| --- | --- | --- |
| Project name | `wfc-web` | `wfc-admin` |
| Production branch | `main` (or your default branch) | same |
| Framework preset | **None** | **None** |
| Root directory | repository root (leave empty / `/`) | same |
| Build command | `pnpm install --frozen-lockfile && pnpm --filter @wfc/web build` | `pnpm install --frozen-lockfile && pnpm --filter @wfc/admin build` |
| Build output directory | `apps/web/dist` | `apps/admin/dist` |
| Environment variable | `VITE_API_BASE_URL=https://api.YOURDOMAIN.com` | `VITE_API_BASE_URL=https://api.YOURDOMAIN.com` |
| Environment variable | `PNPM_VERSION=9.15.4` | `PNPM_VERSION=9.15.4` |

Notes:

* `PNPM_VERSION` must match `packageManager` in `package.json`; the build runs
  from the workspace root so the `@wfc/*` filters resolve.
* The framework preset stays **None**: a Vite preset would force the output
  directory to `dist`, but here it is `apps/*/dist`.
* `VITE_API_BASE_URL` is inlined by Vite at build time - it is **not** a runtime
  variable. Changing it requires a redeploy. Set it for the Production
  environment; if you want preview deployments to work, set it there too and add
  the preview origins to `CORS_ORIGIN` (see section 6).
* Leave `VITE_API_BASE_URL` unset/empty for local development: the SPAs then call
  `/v1/...` relative, which the Vite dev server proxies to `127.0.0.1:8080`.
* **SPA fallback:** Cloudflare Pages only serves files that exist. Both apps need
  a `_redirects` file in their Vite `public/` directory, which Vite copies to the
  build output root:
  `apps/web/public/_redirects` (public app) and `apps/admin/public/_redirects`
  (admin app). Both contain exactly:

  ```
  /*    /index.html   200
  ```

  The admin SPA needs it for every deep link (`/sessions`, `/runs/…`); the public
  app has no routes today but is served the same way so a future route does not
  404.
* Custom domains (Pages → project → Custom domains) are the recommended setup:
  put the admin app and the API under the same registrable domain
  (`admin.<domain>` + `api.<domain>`) so the admin session cookie is first-party.

## 6. `CORS_ORIGIN` - exact value

`CORS_ORIGIN` is a comma-separated allowlist of **exact browser origins**
(scheme + host + optional port, no trailing slash). Whitespace is trimmed, so
spaces after the commas are harmless. It is an equality match: a subdomain,
a different scheme or a Pages preview URL is *not* covered. For a matching
origin the API echoes that origin back (never `*`) together with
`Access-Control-Allow-Credentials: true`, which is what lets the admin SPA send
its session cookie; an origin that is not listed receives no CORS headers at all,
so the browser blocks both the request and the cookie.

List every origin the admin app (and the public app) is actually served from:

```dotenv
CORS_ORIGIN=https://admin.example.com,https://example.com,https://wfc-admin.pages.dev,https://wfc-web.pages.dev
```

* `https://admin.example.com` - admin custom domain (primary).
* `https://wfc-admin.pages.dev` - the Pages project's own hostname.
* Add custom domains for the public app, and the `www` variant if you serve it.
* Preview deployments get a unique origin per deployment
  (`https://<hash>.wfc-admin.pages.dev`) and cannot be matched unless you add
  each one. Either add them explicitly or turn preview deployments off.
* After editing, `docker compose -f deploy/compose.prod.yaml up -d api` to
  recreate the container with the new value (it is an environment variable, read
  at startup - no rebuild needed).

## 7. Admin auth and outbound email

Magic-link sign-in needs the `ADMIN_*` and `SMTP_*` variables in `.env`:

* `ADMIN_APP_URL` - the admin origin (`https://admin.example.com`). It is the
  base of the emailed sign-in link, so it must be the real HTTPS origin.
* `ADMIN_SESSION_TTL_HOURS` - session lifetime in hours; default `720` (30 days),
  minimum `1`. `12` is a reasonable hardened value.
* `ADMIN_COOKIE_DOMAIN` - `.example.com` to share the session cookie between the
  admin app and `api.<domain>` (a leading dot is optional). Leave it **empty**
  when the admin app and the API are on unrelated domains (e.g. `*.pages.dev`).
* `ADMIN_COOKIE_SAMESITE` - `lax` when both sides share a registrable domain
  (recommended, and the default for any unrecognised value). `none` only for
  genuinely cross-site setups, where the cookie is forced `Secure` and may still
  be dropped by browsers that block third-party cookies.
* `SMTP_HOST`/`SMTP_PORT`/`SMTP_USERNAME`/`SMTP_PASSWORD`/`SMTP_FROM`/`SMTP_TLS` -
  the outbound relay used for the sign-in mail. `SMTP_PORT=587` with the default
  `SMTP_TLS=starttls` is the usual combination; set `SMTP_TLS=implicit` (or just
  use port `465`, which implies it) for implicit TLS. `SMTP_TLS=none` sends
  unencrypted and exists only for a local relay (Mailpit, an internal smarthost
  on a private network); never use it against a public host. `SMTP_FROM` is
  required whenever `SMTP_HOST` is set and falls back to `SMTP_USERNAME`; it must
  be an address your provider allows you to send as, or the links are rejected or
  spam-filed.

> **`SMTP_HOST` empty is a development convenience only.** The API logs
> `SMTP_HOST is not set: catalogue admin sign-in links will be written to this
> log instead of emailed` and prints each sign-in link to its own log instead of
> sending it. Production must set `SMTP_HOST`.

The startup line then reports the relay it will use:

```bash
docker compose -f deploy/compose.prod.yaml logs --tail=50 api | grep -i -e smtp -e 'sign-in'
```

Expect `catalogue admin sign-in email via smtp.<provider>:587 (STARTTLS)`.

Then prove the credentials and the sender validation actually work, before anyone
depends on a sign-in link. `mailcheck` sends one message through the same mailer
the API uses and prints the relay's own error text if it fails — which is where
`sender not validated` and authentication rejections surface:

```bash
docker compose -f deploy/compose.prod.yaml exec api mailcheck you@example.com
```

Success prints `sent to you@example.com via <host>:<port> (<mode>) as <from>`.
Failures are also logged by the API itself (`sign-in email failed: …`) while the
browser still gets a neutral response.

## 8. Managing the allowed-email list (`cmd/contributors`)

The image contains every entrypoint built from `backend/cmd/*`, including
`contributors`, and `/app` is on `PATH`. Run it inside the running api container
so it inherits the same `DATABASE_URL` and admin configuration:

```bash
docker compose -f deploy/compose.prod.yaml exec api contributors list
docker compose -f deploy/compose.prod.yaml exec api contributors add someone@example.com
docker compose -f deploy/compose.prod.yaml exec api contributors add boss@example.com owner
docker compose -f deploy/compose.prod.yaml exec api contributors remove someone@example.com
```

* `add <email>` defaults to the `contributor` role; pass `owner` explicitly for
  an owner.
* Add `-T` (`docker compose ... exec -T api contributors list`) when running from
  a script or CI, where there is no TTY.
* With the stack scaled down you can still run it as a one-off container:

  ```bash
  docker compose -f deploy/compose.prod.yaml run --rm --entrypoint contributors api list
  ```

* `docker compose -f deploy/compose.prod.yaml exec api contributors --help`
  prints the authoritative flag list.

## 9. Day-two operations

```bash
# Status and health
docker compose -f deploy/compose.prod.yaml ps
docker compose -f deploy/compose.prod.yaml exec api wget -qO- http://127.0.0.1:8080/healthz

# Logs (follow / filter)
docker compose -f deploy/compose.prod.yaml logs -f --tail=200 api
docker compose -f deploy/compose.prod.yaml logs -f --tail=200 worker
docker compose -f deploy/compose.prod.yaml logs --tail=200 caddy

# Restart one service
docker compose -f deploy/compose.prod.yaml restart api

# Disk usage of the volumes and images
docker system df

# Backup Postgres (custom format, restorable with pg_restore)
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > wheretowfc-$(date +%F).dump

# Plain-SQL backup, if you prefer readable output
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  > wheretowfc-$(date +%F).sql

# Restore (destructive: drops and recreates the objects in the dump)
docker compose -f deploy/compose.prod.yaml exec -T db \
  sh -lc 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner' \
  < wheretowfc-2026-09-23.dump

# Redis is appendonly; force a snapshot before maintenance
docker compose -f deploy/compose.prod.yaml exec redis redis-cli BGSAVE

# Copy a dump off the box (dumps are written on the host, so just scp them)
scp root@<vps-ip>:/srv/wheretowfc/wheretowfc-*.dump ./
```

Upgrade procedure:

```bash
cd /srv/wheretowfc
git pull --ff-only

# 1. Back up first (see above), then apply any new migration - section 4b.

# 2. Rebuild the Go image and recreate the containers it is used by.
docker compose -f deploy/compose.prod.yaml build api
docker compose -f deploy/compose.prod.yaml up -d

# 3. Verify.
docker compose -f deploy/compose.prod.yaml ps
curl -fsS https://api.YOURDOMAIN.com/healthz

# 4. Reclaim disk from the previous image.
docker image prune -f
```

`up -d` recreates only the containers whose image or configuration changed, and
`restart: unless-stopped` keeps everything else running across reboots. Caddy's
certificates live in the `wfc-caddy-data` volume, so upgrades do not re-request
certificates. Frontend changes are not deployed here: pushing to the repository
triggers a new Cloudflare Pages build.

## 10. Hosting other services on the same VPS

Rules that keep projects from tripping over each other:

1. **One directory and one compose project per service**
   (`/srv/wheretowfc`, `/srv/otherproject`) with its own compose file, its own
   containers and its own volumes. Never add unrelated services to
   `deploy/compose.prod.yaml`.
2. **Never publish `80`, `443`, `5432` or `6379`** from another project. This
   Caddy is the single ingress; a second Caddy/reverse proxy cannot bind the same
   ports. Datastores stay private to their own compose network.
3. **Join the `wfc-prod` network as an external network** so this Caddy can reach
   the new service by container name:

   ```yaml
   # /srv/otherproject/compose.yaml
   services:
     other-api:
       image: other-api:prod
       container_name: other-api
       expose: ["3000"]
       networks: [edge]
   networks:
     edge:
       external: true
       name: wfc-prod
   ```

4. **Add a site block to `deploy/Caddyfile`** (see the commented example at the
   bottom of that file), then reload Caddy without dropping connections:

   ```bash
   docker compose -f deploy/compose.prod.yaml exec caddy \
     caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
   ```

5. Give the other service its own DNS record; Caddy issues the certificate on the
   next reload and needs the record to resolve first.
6. Keep an eye on the host: `docker stats`, `df -h`, `docker system prune -f`.
