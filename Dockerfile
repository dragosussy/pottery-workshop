# Builds the web app and the Go binary with the web app embedded. The build
# stages run natively and cross-compile: deploy.sh builds for linux/arm64.

FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.4-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/*.go web/
COPY --from=web /web/dist web/dist
ARG TARGETOS=linux
ARG TARGETARCH=arm64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags embedweb -trimpath -o humidity ./cmd/humidity

FROM debian:bookworm-slim
COPY --from=builder /app/humidity /usr/local/bin/humidity
EXPOSE 8080
VOLUME /data
CMD ["humidity"]
