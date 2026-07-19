FROM golang:1.26-alpine AS build

WORKDIR /src
COPY forge/api/go.mod forge/api/go.sum* ./
RUN go mod download
COPY forge/api/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -H -u 10001 appuser
USER appuser
COPY --from=build /out/api /api
COPY forge/api/migrations /migrations
COPY lang /lang
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/api", "--healthcheck"]
ENTRYPOINT ["/api"]
