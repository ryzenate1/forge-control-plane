FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/daemon ./cmd/daemon

FROM alpine:3.21
RUN mkdir -p /srv/game-panel/servers
COPY --from=build /out/daemon /daemon
EXPOSE 9090 2022
ENTRYPOINT ["/daemon"]
