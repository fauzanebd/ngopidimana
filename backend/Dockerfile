# syntax=docker/dockerfile:1
#
# Build (from the repository root, so the build context contains backend/):
#   docker build -f backend/Dockerfile -t wheretowfc-api:local .
#
# One image, several commands (all built from backend/cmd/*):
#   /app/api           HTTP API (listens on $PORT, default 8080)   <- ENTRYPOINT
#   /app/worker        asynq ingestion worker
#   /app/contributors  admin CLI (allowed-email list)
# /app is on PATH, so inside a container they can be run by bare name:
#   docker compose exec api contributors list
#   docker run --rm --entrypoint worker wheretowfc-api:prod

# ---------- builder ----------
FROM golang:1.26-alpine AS builder

# git is only needed if a module ever resolves over git; harmless when unused.
RUN apk add --no-cache ca-certificates git

WORKDIR /src

# Dependency layer: cached until go.mod / go.sum change.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Source layer.
COPY backend/ ./

# CGO is disabled so the binaries are fully static and the runtime stage can be
# a minimal musl/alpine image. -o /out/ writes one binary per ./cmd/* package,
# named after its directory (api, worker, contributors, ...).
RUN mkdir -p /out \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# Fail the build early if the two required entrypoints were not produced.
RUN test -x /out/api && test -x /out/worker

# ---------- runtime ----------
FROM alpine:3.21

# ca-certificates: outbound TLS to Exa / OpenRouter / Google Places / TypeSafe.
# tzdata: correct timestamps when TZ is set.
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app -g 10001 \
 && adduser -S -D -H -u 10001 -G app app

COPY --from=builder /out/ /app/

USER app
WORKDIR /app
# /app is on PATH so the admin CLI can be run by name inside a running
# container, e.g. `docker compose exec api contributors list`.
ENV PATH=/app:$PATH

# The API container listens on this inside the compose network; the worker does
# not listen at all. Nothing here is published to the host.
ENV PORT=8080
EXPOSE 8080

# Default command is the API; other binaries are selected with --entrypoint
# (see the `worker` service in deploy/compose.prod.yaml).
ENTRYPOINT ["/app/api"]
