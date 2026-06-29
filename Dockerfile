# ─── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS build

# git is required by `go mod download` for some modules that resolve to git refs.
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Copy source (explicit paths avoid pulling secrets from the build context).
COPY cmd/ cmd/
COPY internal/ internal/

# Static build with stripped symbol table for smaller binary.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/chat2api ./cmd/chat2api

# ─── Runtime stage ───────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S chat2api && adduser -S chat2api -G chat2api
USER chat2api

WORKDIR /home/chat2api

COPY --from=build /out/chat2api /usr/local/bin/chat2api

# Default data + storage directory; mount a volume here to persist.
RUN mkdir -p /home/chat2api/data
VOLUME ["/home/chat2api/data"]

ENV HOST=0.0.0.0 \
    PORT=8080 \
    STORAGE_DIR=/home/chat2api/data \
    ACCOUNTS_FILE=/home/chat2api/data/accounts.json

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/chat2api"]