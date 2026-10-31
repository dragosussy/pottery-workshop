# Builds the web app and the Go binary (with the web app embedded). Build
# stages run on the build machine's native platform and cross-compile for
# the target: deploy.sh builds with --platform linux/arm64 (Raspberry Pi 5).

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
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT_AT=
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags embedweb -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.builtAt=${BUILT_AT}" \
    -o humidity ./cmd/humidity

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/humidity .

ENV PORT=8080
EXPOSE 8080
VOLUME /data

CMD ["./humidity"]
