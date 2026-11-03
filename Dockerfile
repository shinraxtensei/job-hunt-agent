# syntax=docker/dockerfile:1
# One image for the API (serving the tracker), the collector and migrations (L-01,
# ADR-0016). Every base image is pinned by digest: bump the tag and the digest together.
# The same image runs on the Mac now and on the server later (D).

# --- the tracker page (ADR-0013)
FROM node:26.13.0-alpine@sha256:c6ad9e9c1d80f6eb607c26b4b0a132ff7a2344fad1c78ea3baa2a7d4c8de6d94 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci --ignore-scripts
COPY web/ ./
# vite writes to ../internal/webui/dist/app (vite.config.ts)
RUN mkdir -p ../internal/webui/dist && npm run build

# --- the Go binaries, static
FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS go
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db
COPY --from=web /src/internal/webui/dist/app ./internal/webui/dist/app
ARG GIT_SHA=unknown
RUN for c in api collector migrate; do \
		go build -ldflags "-s -w -X main.buildCommit=${GIT_SHA}" -o /out/$c ./cmd/$c || exit 1; \
	done

# --- runtime: no shell, no package manager, non-root
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=go /out/ /app/
COPY data/ /app/data/
ENV PRESETS_DIR=/app/data/presets
USER nonroot:nonroot
