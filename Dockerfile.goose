# Dockerfile for StarRocks-compatible Goose
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy source
COPY . .

# Build goose binary
# We build from the root cmd/goose
RUN go build -o /bin/goose ./cmd/goose

# Runtime Stage
FROM alpine:3.19

WORKDIR /app

# Install basic tools (bash, curl) for convenience
RUN apk add --no-cache bash curl

# Copy binary
COPY --from=builder /bin/goose /usr/local/bin/goose

# Default entrypoint
ENTRYPOINT ["/usr/local/bin/goose"]
CMD ["--help"]
