# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Version info
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ENV LDFLAGS="-s -w -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.Version=${VERSION} -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.GitCommit=${GIT_COMMIT} -X github.com/hortator-ai/Hortator/cmd/hortator/cmd.BuildTime=${BUILD_TIME}"

# Build operator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go

# Build CLI
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags "${LDFLAGS}" -o hortator ./cmd/hortator

# Build gateway
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o gateway ./cmd/gateway

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/hortator /usr/local/bin/
COPY --from=builder /workspace/gateway /usr/local/bin/

USER 65532:65532

ENTRYPOINT ["/manager"]
