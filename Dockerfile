FROM golang:1.26.4-bookworm AS builder
WORKDIR /app

COPY go.mod go.sum* ./

RUN go mod download

COPY . .
RUN GOOS=linux GOARCH=arm64 go build -o humidity

FROM debian:bookworm-slim
WORKDIR /app

COPY --from=builder /app/humidity .

ENV PORT=8080
EXPOSE 8080

CMD ["./humidity"]
