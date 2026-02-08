FROM golang:1.25-bookworm AS cli-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hortator ./cmd/hortator

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git python3 nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /inbox /outbox /workspace /memory

COPY --from=cli-builder /hortator /usr/local/bin/hortator

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
