FROM golang:1.25-bookworm AS cli-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.Version=${VERSION} -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.GitCommit=${GIT_COMMIT} -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.BuildTime=${BUILD_TIME}" -o /hortator ./cmd/hortator

FROM python:3.12-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /inbox /outbox /outbox/artifacts /workspace /memory

COPY --from=cli-builder /hortator /usr/local/bin/hortator

COPY runtime/agentic/requirements.txt /opt/hortator/requirements.txt
RUN pip install --no-cache-dir -r /opt/hortator/requirements.txt

COPY runtime/agentic/*.py /opt/hortator/

WORKDIR /opt/hortator

ENTRYPOINT ["python3", "/opt/hortator/main.py"]
