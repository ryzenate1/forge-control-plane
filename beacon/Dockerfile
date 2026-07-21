FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/daemon ./cmd/daemon

FROM alpine:3.21

LABEL maintainer="GamePanel Team" \
      version="1.0.0" \
      description="Beacon - GamePanel node agent" \
      org.opencontainers.image.source="https://github.com/gamepanel/beacon" \
      org.opencontainers.image.title="Beacon" \
      org.opencontainers.image.description="GamePanel per-node Docker, filesystem, console, backup and SFTP agent" \
      security.nonroot="true" \
      security.readonly-rootfs="true"

RUN apk add --no-cache ca-certificates tzdata

ENV TZ=UTC

RUN adduser -D -H -u 10001 appuser

RUN mkdir -p /srv/game-panel/servers && \
    chown -R appuser:appuser /srv/game-panel/servers

COPY --from=build /out/daemon /daemon
RUN chown appuser:appuser /daemon

USER appuser

EXPOSE 9090 2022

VOLUME ["/srv/game-panel/servers"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/daemon", "--healthcheck"]

ENTRYPOINT ["/daemon"]
