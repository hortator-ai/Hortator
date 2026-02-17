FROM golang:1.25-bookworm AS cli-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.Version=${VERSION} -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.GitCommit=${GIT_COMMIT} -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.BuildTime=${BUILD_TIME}" -o /hortator ./cmd/hortator

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git python3 nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -u 1000 -m -s /bin/bash hortator \
    && mkdir -p /inbox /outbox /workspace /memory \
    && chown -R hortator:hortator /inbox /outbox /workspace /memory

COPY --from=cli-builder /hortator /usr/local/bin/hortator

COPY runtime/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER 1000
ENTRYPOINT ["/entrypoint.sh"]
