FROM golang:1.26-alpine AS build

WORKDIR /src
COPY forge/api/go.mod forge/api/go.sum* ./
RUN go mod download
COPY forge/api/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

FROM alpine:3.21

LABEL maintainer="GamePanel Team" \
      version="1.0.0" \
      description="Forge API - GamePanel control plane REST API" \
      org.opencontainers.image.source="https://github.com/gamepanel/forge" \
      org.opencontainers.image.title="Forge API" \
      org.opencontainers.image.description="GamePanel control plane REST API"

RUN apk add --no-cache ca-certificates tzdata

ENV TZ=UTC

RUN adduser -D -H -u 10001 appuser

COPY --from=build /out/api /api
COPY forge/api/migrations /migrations
COPY forge/api/internal/store/migrations/033_node_onboarding.sql /batch2-migrations/033_node_onboarding.sql
COPY forge/api/internal/store/migrations/034_build_pipeline.sql /batch2-migrations/034_build_pipeline.sql
COPY forge/api/internal/store/migrations/035_compose_gitops.sql /batch2-migrations/035_compose_gitops.sql
COPY forge/api/internal/store/migrations/035_a_infra_endpoints.sql /batch2-migrations/035_a_infra_endpoints.sql
COPY forge/api/internal/store/migrations/035_b_observability_monitoring.sql /batch2-migrations/035_b_observability_monitoring.sql
COPY forge/api/internal/store/migrations/036_compose_concurrency.sql /batch2-migrations/036_compose_concurrency.sql
COPY forge/api/internal/store/migrations/037_build_extended_fields.sql /batch2-migrations/037_build_extended_fields.sql
COPY forge/api/internal/store/migrations/038_traffic_routing.sql /batch2-migrations/038_traffic_routing.sql
COPY forge/api/internal/store/migrations/040_git_deployment.sql /batch2-migrations/040_git_deployment.sql
COPY forge/api/internal/store/migrations/041_a_placement_intents.sql /batch2-migrations/041_a_placement_intents.sql
COPY forge/api/internal/store/migrations/041_buildpack_support.sql /batch2-migrations/041_buildpack_support.sql
COPY forge/api/internal/store/migrations/042_service_discovery_endpoints.sql /batch2-migrations/042_service_discovery_endpoints.sql
COPY forge/api/internal/store/migrations/043_webhook_idempotency.sql /batch2-migrations/043_webhook_idempotency.sql
COPY lang /lang

RUN chown -R appuser:appuser /migrations /batch2-migrations /lang

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/api", "--healthcheck"]

ENTRYPOINT ["/api"]

VOLUME ["/tmp"]

LABEL security.nonroot="true" \
      security.readonly-rootfs="true"
