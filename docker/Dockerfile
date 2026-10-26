# syntax=docker/dockerfile:1

# This file uses multi-stage builds to build the application from source, including the front-end

# Tags passed to "go build"
ARG BUILD_TAGS=""

# Stage 1: Build Frontend
FROM --platform=$BUILDPLATFORM ghcr.io/voidzero-dev/vite-plus:1.4.0 AS frontend-builder
ENV CI=true
WORKDIR /app

COPY --chown=vp:vp package.json pnpm-workspace.yaml pnpm-lock.yaml .node-version ./
COPY --chown=vp:vp --parents frontend/package.json tests/package.json email-templates/package.json ./
RUN --mount=type=cache,id=quad4-id-pnpm,target=/home/vp/.local/share/pnpm/store,sharing=locked,uid=1000,gid=1000 \
  vp -C frontend install --frozen-lockfile --prefer-offline

COPY --chown=vp:vp ./frontend ./frontend/

RUN --mount=type=cache,target=/app/frontend/node_modules/.vite,uid=1000,gid=1000 \
  --mount=type=cache,target=/app/frontend/.svelte-kit,uid=1000,gid=1000 \
  BUILD_OUTPUT_PATH=dist vp -C frontend run build

# Stage 2: Build Backend
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS backend-builder
ARG BUILD_TAGS
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY ./backend/go.mod ./backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  go mod download

COPY ./backend ./
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY .version .version

# The CycloneDX SBOM is shipped as documentation in the final image instead of being embedded in the binary
RUN rm -rf ./frontend/dist/cyclonedx

WORKDIR /build/cmd
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  VERSION=$(cat /build/.version) && \
  CGO_ENABLED=0 \
  GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  go build \
    -tags "${BUILD_TAGS}" \
    -ldflags="-w -s -X github.com/pocket-id/pocket-id/backend/internal/common.Version=${VERSION} -buildid=${VERSION}" \
    -trimpath \
    -o /build/quad4-id \
    .

# Stage 3: Data directory
# Distroless has no shell, so the data directory is created and chowned in a builder stage
FROM alpine:3.24.3 AS data-dir
RUN install -d -o 65532 -g 65532 /app/data

# Stage 4: Production Image
FROM gcr.io/distroless/static-debian13:nonroot

ARG VERSION
ARG REVISION

LABEL org.opencontainers.image.title="Quad4 ID" \
  org.opencontainers.image.description="A passkey-only OpenID Connect and OAuth 2.0 provider" \
  org.opencontainers.image.source="https://github.com/Quad4-Software/Quad4-ID" \
  org.opencontainers.image.url="https://github.com/Quad4-Software/Quad4-ID" \
  org.opencontainers.image.documentation="https://github.com/Quad4-Software/Quad4-ID/blob/master/README.md" \
  org.opencontainers.image.licenses="BSD-2-Clause" \
  org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${REVISION}"

WORKDIR /app

COPY --from=backend-builder /build/quad4-id /app/quad4-id
COPY --from=frontend-builder /app/frontend/dist/cyclonedx/frontend.cdx.json /usr/share/doc/quad4-id/frontend.cdx.json
COPY --from=data-dir --chown=65532:65532 /app/data /app/data

USER 65532:65532

EXPOSE 1411
ENV APP_ENV=production

HEALTHCHECK --interval=90s --timeout=5s --start-period=10s --retries=3 CMD [ "/app/quad4-id", "healthcheck" ]

ENTRYPOINT ["/app/quad4-id"]
