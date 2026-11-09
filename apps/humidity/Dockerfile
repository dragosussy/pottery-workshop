# Builds the web app and the Go binary with the web app embedded. Build it from
# the repo root, because go.mod is there. The build stages run natively and
# cross-compile: deploy.sh builds for linux/arm64.
# The cache mounts keep npm's downloads and Go's compiled packages between
# builds, so a code change recompiles only what changed.

FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /repo/apps/humidity/web
COPY apps/humidity/web/package.json apps/humidity/web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY apps/humidity/web/ ./
COPY shared/ui/ /repo/shared/ui/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.4-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY shared/kit/ shared/kit/
COPY apps/humidity/cmd/ apps/humidity/cmd/
COPY apps/humidity/internal/ apps/humidity/internal/
COPY apps/humidity/web/*.go apps/humidity/web/
COPY --from=web /repo/apps/humidity/web/dist apps/humidity/web/dist
ARG TARGETOS=linux
ARG TARGETARCH=arm64
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags embedweb -trimpath -ldflags="-s -w" -o humidity ./apps/humidity/cmd/humidity

FROM debian:bookworm-slim
COPY --from=builder /app/humidity /usr/local/bin/humidity
EXPOSE 8080
VOLUME /data
CMD ["humidity"]
