# syntax=docker/dockerfile:1.7
FROM golang:1.25-alpine3.22 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/pressly/goose/v3/cmd/goose@latest

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    go build -trimpath -ldflags="-s -w" -o /out/import ./cmd/import && \
    go build -trimpath -ldflags="-s -w" -o /out/bootadmin ./cmd/bootadmin

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 app

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /out/import /app/import
COPY --from=builder /out/bootadmin /app/bootadmin
COPY --from=builder /go/bin/goose /app/goose
COPY migrations /app/migrations

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO /dev/null --tries=1 http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/api"]
