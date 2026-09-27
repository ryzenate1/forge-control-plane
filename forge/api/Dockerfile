FROM golang:1.26-alpine AS build

WORKDIR /src
COPY forge/api/go.mod forge/api/go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    for attempt in 1 2 3; do \
      go mod download && exit 0; \
      [ "$attempt" -eq 3 ] || sleep $((attempt * 5)); \
    done; exit 1
COPY forge/api/ .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

FROM alpine:3.21

LABEL maintainer="GamePanel Team" \
      version="1.0.0" \
      description="Forge API - GamePanel control plane REST API" \
      org.opencontainers.image.source="https://github.com/gamepanel/forge" \
      org.opencontainers.image.title="Forge API" \
      org.opencontainers.image.description="GamePanel control plane REST API"

RUN apk add --no-cache ca-certificates postgresql16-client tzdata

ENV TZ=UTC

LABEL security.nonroot="true" \
      security.readonly-rootfs="true"

RUN adduser -D -H -u 10001 appuser

COPY --from=build /out/api /api
COPY forge/api/migrations /migrations
COPY lang /lang

RUN mkdir -p /batch2-migrations && \
    chown -R appuser:appuser /migrations /batch2-migrations /lang

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/api", "--healthcheck"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/api"]

VOLUME ["/tmp"]
