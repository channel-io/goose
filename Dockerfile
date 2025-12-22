FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /goose ./cmd/goose

FROM alpine:3.21

RUN apk add --no-cache bash ca-certificates tzdata

COPY --from=builder /goose /usr/local/bin/goose
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

VOLUME /migrations
WORKDIR /migrations

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["status"]
