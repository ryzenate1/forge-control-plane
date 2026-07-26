FROM golang:1.26-alpine AS build

WORKDIR /src
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    for attempt in 1 2 3; do \
      go mod download && exit 0; \
      [ "$attempt" -eq 3 ] || sleep $((attempt * 5)); \
    done; exit 1
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -o /out/daemon ./cmd/daemon

FROM alpine:3.21

ARG TARGETARCH
ARG NIXPACKS_VERSION=1.41.0

LABEL maintainer="GamePanel Team" \
      version="1.0.0" \
      description="Beacon - GamePanel node agent" \
      org.opencontainers.image.source="https://github.com/gamepanel/beacon" \
      org.opencontainers.image.title="Beacon" \
      org.opencontainers.image.description="GamePanel per-node Docker, filesystem, console, backup and SFTP agent"

RUN apk add --no-cache ca-certificates curl docker-cli iptables tar tzdata && \
    case "$TARGETARCH" in \
      amd64) archive_arch=x86_64; expected=0f55de7874507b9cf7502113120bd96f2ab6979f78d10eaf2eb2ade9207b3af6 ;; \
      arm64) archive_arch=aarch64; expected=912bd02dd2bb6f9c3a9ed965fe8a68b4aa318dc7a2546e2eca6f2806a894ba39 ;; \
      *) echo "unsupported Nixpacks architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    archive="nixpacks-v${NIXPACKS_VERSION}-${archive_arch}-unknown-linux-musl.tar.gz" && \
    curl --fail --location --retry 3 --output "/tmp/$archive" \
      "https://github.com/railwayapp/nixpacks/releases/download/v${NIXPACKS_VERSION}/$archive" && \
    echo "$expected  /tmp/$archive" | sha256sum -c - && \
    tar -xzf "/tmp/$archive" -C /usr/local/bin nixpacks && \
    rm -f "/tmp/$archive" && \
    nixpacks --version

ENV TZ=UTC

LABEL security.nonroot="true" \
      security.readonly-rootfs="true"

RUN adduser -D -H -u 10001 appuser

RUN mkdir -p /srv/game-panel/servers && \
    chown -R appuser:appuser /srv/game-panel/servers

COPY --from=build /out/daemon /daemon
RUN chown appuser:appuser /daemon

USER appuser

EXPOSE 9090 2022

VOLUME ["/srv/game-panel/servers"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/daemon", "--healthcheck"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/daemon"]
